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

package contextmgr

import (
	"context"
	"fmt"
	"sync/atomic"
)

// LongMemPromoter implements the L2 fact → long-term memory promotion pipeline (§8C).
type LongMemPromoter struct {
	store     StepStoreLike
	cfg       LongMemConfig
	sessionID string
	memCounter int32
	// GraphHook is an optional callback invoked after each successful promotion (OT-12).
	// It receives the promoted record for entity/relation extraction.
	GraphHook func(mem LongMemRecord)
}

// NewLongMemPromoter creates a long-term memory promoter.
func NewLongMemPromoter(store StepStoreLike, cfg LongMemConfig, sessionID string) *LongMemPromoter {
	return &LongMemPromoter{store: store, cfg: cfg, sessionID: sessionID}
}

// EvaluateAndPromote checks all L2 facts and promotes qualifying ones to long-term memory (§8C.2).
func (p *LongMemPromoter) EvaluateAndPromote(ctx context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	// Get all L2 records that meet the minimum ref_count threshold
	l2Records, err := p.store.HotFacts(p.cfg.MinRefCount, 0)
	if err != nil {
		return err
	}

	for _, l2 := range l2Records {
		ref, err := p.store.GetRef(l2.StepID)
		if err != nil {
			continue
		}

		// Check all promotion conditions (§8C.2)
		if !p.shouldPromote(l2, *ref) {
			continue
		}

		// Create long-term memory record
		memID := fmt.Sprintf("m_%s_%d", p.sessionID[:6], atomic.AddInt32(&p.memCounter, 1))
		mem := LongMemRecord{
			MemID:             memID,
			Facts:             l2.Facts,
			SourceSteps:       []int{l2.StepID},
			SourceSessions:    []string{p.sessionID},
			Category:          p.categorizeFacts(l2.Facts),
			CreatedAtStep:     l2.CompressedAtStep,
			LastValidatedStep: l2.CompressedAtStep,
			Confidence:        p.cfg.InitialConfidence,
		}

		if err := p.store.UpsertLongMem(mem); err != nil {
			continue
		}

		// OT-12: invoke graph extraction hook if configured.
		if p.GraphHook != nil {
			p.GraphHook(mem)
		}
	}

	return nil
}

// shouldPromote checks if an L2 record qualifies for promotion (§8C.2).
func (p *LongMemPromoter) shouldPromote(l2 L2Record, ref RefRecord) bool {
	// Condition 1: ref_count ≥ MinRefCount (already filtered by HotFacts)
	if ref.RefCount < p.cfg.MinRefCount {
		return false
	}

	// Condition 2: fact type is config/decision/constraint (not ephemeral)
	category := p.categorizeFacts(l2.Facts)
	if category == "" {
		return false
	}

	// Condition 3: cross ≥ 2 different task_groups (§8C.2)
	// Check via related refs: if this fact's step has been referenced from
	// steps belonging to multiple task groups.
	taskGroups := p.countTaskGroupsForStep(l2.StepID)
	if taskGroups < p.cfg.MinTaskGroups {
		return false
	}

	return true
}

// countTaskGroupsForStep estimates the number of distinct task groups
// that reference a given step by scanning refs.
func (p *LongMemPromoter) countTaskGroupsForStep(stepID int) int {
	ids, err := p.store.AllActiveStepIDs()
	if err != nil {
		return 1
	}
	groups := make(map[int]bool)
	for _, id := range ids {
		ref, err := p.store.GetRef(id)
		if err != nil {
			continue
		}
		// If this step references our target or is the target itself
		if id == stepID || (ref.LastRefAtStep != nil && *ref.LastRefAtStep == stepID) {
			groups[ref.TaskGroupID] = true
		}
	}
	if len(groups) == 0 {
		return 1
	}
	return len(groups)
}

// categorizeFacts determines the category of facts (§8C.3).
func (p *LongMemPromoter) categorizeFacts(facts []string) string {
	for _, f := range facts {
		// Config patterns: KEY=VALUE, paths, ports
		if containsPattern(f, "=", ":", "config", "host", "port", "path") {
			return "config"
		}
		// Decision patterns
		if containsPattern(f, "decision", "decided", "chosen", "selected", "migrated") {
			return "decision"
		}
		// Constraint patterns
		if containsPattern(f, "must", "required", "limit", "constraint", "max", "min") {
			return "constraint"
		}
	}
	return "" // not promotable
}

func containsPattern(text string, patterns ...string) bool {
	lower := toLower(text)
	for _, p := range patterns {
		if contains(lower, p) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ValidateMemory updates confidence when a long-term memory is cited (§8C.5).
func (p *LongMemPromoter) ValidateMemory(memID string, currentStep int) error {
	mem, err := p.store.GetLongMem(memID)
	if err != nil {
		return err
	}
	mem.LastValidatedStep = currentStep
	mem.Confidence += 0.04
	if mem.Confidence > 1.0 {
		mem.Confidence = 1.0
	}
	return p.store.UpsertLongMem(*mem)
}

// NegateMemory zeros confidence when a memory is contradicted (§8C.5).
func (p *LongMemPromoter) NegateMemory(memID string) error {
	mem, err := p.store.GetLongMem(memID)
	if err != nil {
		return err
	}
	mem.Confidence = 0
	return p.store.UpsertLongMem(*mem)
}

// OnSessionEnd promotes qualifying facts when session closes (§8C.5).
func (p *LongMemPromoter) OnSessionEnd(ctx context.Context) error {
	return p.EvaluateAndPromote(ctx)
}
