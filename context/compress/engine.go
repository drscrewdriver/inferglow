// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// Package compress implements the compression engine for contextmgr.
//
// It provides:
//   - CompressModelChain: small model → main model → mechanical fallback (§3.1)
//   - Prompt templates for L1/L2/L3 compression (§4.1-4.3)
//   - Quality validation (§3.3)
//   - Idle consolidation (§6.1 layer 2)
//   - Engine: five-layer defense + 8-step batch compression (§6.1, §6.4)
package compress

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/inferglow/context"
)

// maskHeaderRegex validates the L2/L3 mask header format.
// Expected: [掩码 step_N|原X t|tool|params]
var maskHeaderRegex = regexp.MustCompile(`^\[掩码 step_\d+\|原\d+t\|.+\|.+\]`)

// CompressModelClient is the interface for compression model calls.
type CompressModelClient interface {
	// Compress executes compression at the given level.
	Compress(ctx context.Context, level int, prompt string) (string, error)
	// Available checks if the model is reachable.
	Available() bool
}

// CompressModelChain implements the small→main→mechanical fallback chain (§3.1).
type CompressModelChain struct {
	small   CompressModelClient
	main    CompressModelClient
	timeout time.Duration
	retries int
}

// NewCompressModelChain creates a new compression model chain.
func NewCompressModelChain(small, main CompressModelClient, timeout time.Duration, retries int) *CompressModelChain {
	return &CompressModelChain{
		small:   small,
		main:    main,
		timeout: timeout,
		retries: retries,
	}
}

// Compress executes compression with the full fallback chain.
func (c *CompressModelChain) Compress(ctx context.Context, level int, prompt string) (string, error) {
	// 1. Try small model
	if c.small != nil && c.small.Available() {
		result, err := c.tryWithRetry(ctx, c.small, level, prompt)
		if err == nil && c.validate(level, prompt, result) {
			return result, nil
		}
	}

	// 2. Fallback to main model
	if c.main != nil && c.main.Available() {
		result, err := c.main.Compress(ctx, level, prompt)
		if err == nil && c.validate(level, prompt, result) {
			return result, nil
		}
	}

	// 3. Mechanical fallback (no LLM)
	return MechanicalCompress(level, prompt)
}

func (c *CompressModelChain) tryWithRetry(ctx context.Context, client CompressModelClient, level int, prompt string) (string, error) {
	var lastErr error
	for i := 0; i <= c.retries; i++ {
		ctxTimeout, cancel := context.WithTimeout(ctx, c.timeout)
		result, err := client.Compress(ctxTimeout, level, prompt)
		cancel()
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// validate checks compression quality (§3.3).
func (c *CompressModelChain) validate(level int, original, compressed string) bool {
	// Rule 1: compressed must not be larger than original
	if len(compressed) > len(original) {
		return false
	}
	// Rule 2: L2 must match mask header regex (§4.2)
	if level == 2 && !maskHeaderRegex.MatchString(compressed) {
		return false
	}
	// Rule 3: L3 must match mask header regex (§4.3)
	if level == 3 && !maskHeaderRegex.MatchString(compressed) {
		return false
	}
	// Rule 4: must not be empty
	if strings.TrimSpace(compressed) == "" {
		return false
	}
	return true
}

// Engine orchestrates the five-layer compression defense (§6.1).
type Engine struct {
	chain *CompressModelChain
	store contextmgr.StepStoreLike
	cfg   contextmgr.Config
}

// NewEngine creates a compression engine.
func NewEngine(chain *CompressModelChain, store contextmgr.StepStoreLike, cfg contextmgr.Config) *Engine {
	return &Engine{chain: chain, store: store, cfg: cfg}
}

// CompressStep compresses a single step to the target level (§6.4 8-step process).
// L3 is derived from L2's first line (mask header).
func (e *Engine) CompressStep(ctx context.Context, stepID int, targetLevel int) error {
	// Step 1: Read current ref
	ref, err := e.store.GetRef(stepID)
	if err != nil {
		return err
	}

	// Step 2: Get original content
	step, err := e.store.GetStep(stepID)
	if err != nil {
		return err
	}

	// Step 3: Get previous level content as input
	input := step.Content
	if ref.Level > 0 {
		input, err = e.getContentAtLevel(stepID, ref.Level)
		if err != nil {
			input = step.Content
		}
	}

	// Step 4: Build prompt and execute compression
	// L3 uses L2 prompt (mask header + facts), then extracts first line as mask
	promptLevel := targetLevel
	if targetLevel == 3 {
		promptLevel = 2 // L3 is derived from L2 output
	}
	prompt := BuildPrompt(promptLevel, stepID, step.ToolName, step.KeyParams, input)
	compressed, err := e.chain.Compress(ctx, promptLevel, prompt)
	if err != nil {
		return fmt.Errorf("compress step %d to L%d: %w", stepID, targetLevel, err)
	}

	// For L3 target, ensure valid mask header. If LLM chain failed and
	// mechanical L2 fallback produced output without [掩码 header,
	// override with MechanicalL3 using step metadata (§4.4).
	if targetLevel == 3 && !maskHeaderRegex.MatchString(compressed) {
		compressed = MechanicalL3(stepID, step.ToolName, step.KeyParams, input)
	}

	// Step 5: Write to store
	tokenEst := len(compressed) / 4
	switch targetLevel {
	case 1:
		err = e.store.AppendL1(contextmgr.L1Record{
			StepID:           stepID,
			Content:          compressed,
			TokenCount:       tokenEst,
			CompressedAtStep: stepID,
		})
	case 2:
		// L2 stores mask header + facts
		facts := strings.Split(compressed, "\n")
		err = e.store.AppendL2(contextmgr.L2Record{
			StepID:           stepID,
			Facts:            facts,
			TokenCount:       tokenEst,
			CompressedAtStep: stepID,
		})
	case 3:
		// L3 is derived from L2: first line is the mask header
		lines := strings.SplitN(compressed, "\n", 2)
		mask := strings.TrimSpace(lines[0])
		// Also store L2 (full content)
		facts := strings.Split(compressed, "\n")
		_ = e.store.AppendL2(contextmgr.L2Record{
			StepID:           stepID,
			Facts:            facts,
			TokenCount:       tokenEst,
			CompressedAtStep: stepID,
		})
		err = e.store.AppendL3(contextmgr.L3Record{
			StepID:           stepID,
			Mask:             mask,
			TokenCount:       tokenEst,
			CompressedAtStep: stepID,
		})
	}
	if err != nil {
		return err
	}

	// Step 6: Update ref level
	ref.Level = targetLevel
	return e.store.UpsertRef(*ref)
}

func (e *Engine) getContentAtLevel(stepID, level int) (string, error) {
	switch level {
	case 1:
		rec, err := e.store.GetL1(stepID)
		if err != nil {
			return "", err
		}
		return rec.Content, nil
	case 2:
		rec, err := e.store.GetL2(stepID)
		if err != nil {
			return "", err
		}
		return strings.Join(rec.Facts, "\n"), nil
	case 3:
		rec, err := e.store.GetL3(stepID)
		if err != nil {
			return "", err
		}
		return rec.Mask, nil
	default:
		step, err := e.store.GetStep(stepID)
		if err != nil {
			return "", err
		}
		return step.Content, nil
	}
}

// BatchCompress runs the full 8-step batch compression process (§6.4).
func (e *Engine) BatchCompress(ctx context.Context) (*contextmgr.CompressResult, error) {
	result := &contextmgr.CompressResult{NewLevels: make(map[int]int)}

	ids, err := e.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		ref, err := e.store.GetRef(id)
		if err != nil {
			continue
		}

		step, err := e.store.GetStep(id)
		if err != nil {
			continue
		}

		maxLvl := contextmgr.MaxLevelForType(step.Type)
		if ref.Level >= maxLvl || ref.LockL0 {
			continue
		}

		// Estimate raw_decay
		rawDecay := step.TokenCount * len(ids) // simplified
		decay := contextmgr.EffectiveDecay(*ref, rawDecay, false, false)
		target := contextmgr.TargetLevel(decay, step.Type, e.cfg.Thresholds)
		if target > maxLvl {
			target = maxLvl
		}
		if target <= ref.Level {
			continue
		}

		// Step 3: Estimate savings — skip if < 2K tokens (§6.4 step 3)
		originalTokens := step.TokenCount
		estimatedSaved := originalTokens / 2 // rough estimate: 50% reduction
		if estimatedSaved < 2000 {
			continue
		}

		// Step 4: Execute compression
		if err := e.CompressStep(ctx, id, target); err != nil {
			continue
		}

		result.StepsCompressed++
		result.NewLevels[id] = target
	}

	return result, nil
}
