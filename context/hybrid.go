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
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HybridManager implements ModeHybrid with full L0-L4 compression.
type HybridManager struct {
	cfg   Config
	store StepStoreLike

	mu            sync.RWMutex
	currentStep   int32 // atomic counter for step IDs
	headBuffer    []RenderedBlock
	headBufferVer string
	taskGroupID   int32
	idleCount     int32

	// --- Phase 0: sweet-spot passthrough ---
	sweetSpotTokens   int     // effective value (may be adjusted by tolerance)
	sweetSpotOriginal int     // original config value (immutable after init)
	sweetSpotTolerance float64 // current multiplier (1.0 = original)
	toleranceMu       sync.Mutex

	// sliding window for citation tracking
	recentRefCounts []int
	recentRefIdx    int

	// --- Phase 0: warmup pre-compression ---
	warmupPending atomic.Bool

	// --- Phase 0: Zone 0.5 constitutional entries ---
	constitutionalEntries []string
	constitutionalMu      sync.RWMutex

	// --- Phase 0: head buffer archive ---
	archivedHeads []ArchivedHead
}

// ArchivedHead is a previous head buffer kept for audit/rollback.
type ArchivedHead struct {
	Content    []RenderedBlock
	Version    string
	ArchivedAt time.Time
}

// NewHybridManager creates a hybrid context manager.
func NewHybridManager(cfg Config, store StepStoreLike) (ContextManager, error) {
	h := &HybridManager{
		cfg:                cfg,
		store:              store,
		headBufferVer:      "h_v1",
		sweetSpotTokens:    cfg.SweetSpotTokens,
		sweetSpotOriginal:  cfg.SweetSpotTokens,
		sweetSpotTolerance: 1.0,
		recentRefCounts:    make([]int, 10), // sliding window of 10
	}
	if cfg.WarmupRatio <= 0 {
		h.cfg.WarmupRatio = 0.8
	}
	if cfg.ToleranceDecayRate <= 0 {
		h.cfg.ToleranceDecayRate = 0.98
	}
	return h, nil
}

func (h *HybridManager) Mode() Mode { return ModeHybrid }

// StepStore returns the underlying step store for direct access (e.g., causal tracing).
func (h *HybridManager) StepStore() StepStoreLike { return h.store }

// SetHeadBuffer sets the immutable head buffer (zone 1). Called once at session init.
func (h *HybridManager) SetHeadBuffer(blocks []RenderedBlock, version string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.headBuffer = blocks
	h.headBufferVer = version
}

// Ingest processes a new step: stores L0, creates ref, checks per-step decay.
func (h *HybridManager) Ingest(step StepRecord) error {
	sid := int(atomic.AddInt32(&h.currentStep, 1))
	step.StepID = sid

	// Store L0
	if err := h.store.AppendStep(step); err != nil {
		return fmt.Errorf("hybrid ingest: append step: %w", err)
	}

	// Create initial ref (level=0, strength=1.0)
	ref := RefRecord{
		StepID:      sid,
		Level:       0,
		RefCount:    0,
		Strength:    1.0,
		TaskGroupID: int(h.taskGroupID),
	}
	if err := h.store.UpsertRef(ref); err != nil {
		return fmt.Errorf("hybrid ingest: upsert ref: %w", err)
	}

	// Reset idle counter
	atomic.StoreInt32(&h.idleCount, 0)

	// Per-step decay check (layer 1 of 5)
	if err := h.perStepDecay(sid); err != nil {
		return err
	}

	// Warmup pre-compression: async pre-compress old steps when approaching sweet spot
	h.maybeWarmup()

	// Decay tolerance back toward 1.0
	h.decayTolerance()

	return nil
}

// perStepDecay checks if the new step triggers compression for older steps.
// When sweet-spot is enabled and total tokens are below the threshold,
// decay is suppressed to maximise prefix cache hit rate.
func (h *HybridManager) perStepDecay(currentStep int) error {
	// Sweet-spot passthrough: skip decay if below threshold
	if h.sweetSpotTokens > 0 && h.estimateTotalTokens() < h.sweetSpotTokens {
		return nil
	}

	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	// Compute raw_decay for each step: sum of tokens from last_ref+1 to current
	for _, id := range ids {
		if id == currentStep {
			continue
		}
		ref, err := h.store.GetRef(id)
		if err != nil || ref.Level >= 3 {
			continue
		}

		// Compute raw_decay
		lastRef := currentStep - 1
		if ref.LastRefAtStep != nil {
			lastRef = *ref.LastRefAtStep
		}
		rawDecay := h.sumTokens(lastRef+1, currentStep)

		decay := EffectiveDecay(*ref, rawDecay, false, false)
		target := TargetLevel(decay, h.stepType(id), h.cfg.Thresholds)
		maxLvl := MaxLevelForType(h.stepType(id))
		if target > maxLvl {
			target = maxLvl
		}

		if target > ref.Level {
			// Level upgrade needed — actual compression is done by compress engine
			ref.Level = target
			if err := h.store.UpsertRef(*ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *HybridManager) stepType(stepID int) string {
	step, err := h.store.GetStep(stepID)
	if err != nil {
		return "reasoning"
	}
	return step.Type
}

func (h *HybridManager) sumTokens(from, to int) int {
	total := 0
	steps, err := h.store.RangeSteps(from, to)
	if err != nil {
		return 0
	}
	for _, s := range steps {
		total += s.TokenCount
	}
	return total
}

// BuildContext assembles the 5-zone context window (§4.5).
func (h *HybridManager) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var blocks []RenderedBlock

	// Zone 0.5: constitutional entries (dynamic, before head buffer)
	h.constitutionalMu.RLock()
	if len(h.constitutionalEntries) > 0 {
		content := "<constitutional>\n"
		for _, e := range h.constitutionalEntries {
			content += "- " + e + "\n"
		}
		content += "</constitutional>"
		blocks = append(blocks, RenderedBlock{
			StepID:  -3, // constitutional pseudo-step
			Level:   0,
			Content: content,
		})
	}
	h.constitutionalMu.RUnlock()

	// Zone 1: head_buffer (never compressed)
	blocks = append(blocks, h.headBuffer...)

	// Zone 2: hot facts injection (from .l2.jsonl with ref_count≥3, strength≥1.3)
	hotFacts, err := h.store.HotFacts(h.cfg.HotFacts.MinRefCount, h.cfg.HotFacts.MinStrength)
	if err == nil {
		for _, f := range hotFacts {
			content := "[facts | session | sources: step_" + fmt.Sprint(f.StepID) + "]\n"
			for _, fact := range f.Facts {
				content += "  • " + fact + "\n"
			}
			content += "[/facts]"
			blocks = append(blocks, RenderedBlock{
				StepID:  f.StepID,
				Level:   2,
				Content: content,
			})
		}
	}

	// Zone 2 extended: long-term memory facts (confidence ≥ 0.7) (§8C.4)
	if h.cfg.LongMem.Enabled {
		longMems, err := h.store.SearchLongMem("", "", 20)
		if err == nil {
			for _, mem := range longMems {
				if mem.Confidence < 0.7 {
					continue
				}
				content := fmt.Sprintf("[facts | longterm | mem:%s | conf:%.2f]\n", mem.MemID, mem.Confidence)
				for _, fact := range mem.Facts {
					content += "  • " + fact + " (跨 session 验证)\n"
				}
				content += "[/facts]"
				blocks = append(blocks, RenderedBlock{
					StepID:  -2, // longmem pseudo-step
					Level:   2,
					Content: content,
				})
			}
		}
	}

	// Zone 3 + 4: compressed history + tail original
	allIDs, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	tailStart := len(allIDs) - h.cfg.TailKeepSteps
	if tailStart < 0 {
		tailStart = 0
	}

	for i, id := range allIDs {
		if i >= tailStart {
			// Zone 4: tail original (L0)
			step, err := h.store.GetStep(id)
			if err != nil {
				continue
			}
			blocks = append(blocks, renderBlock(id, 0, step.Content, step.Type))
		} else {
			// Zone 3: compressed history
			ref, err := h.store.GetRef(id)
			if err != nil {
				continue
			}
			content, err := h.renderStepContent(id, ref.Level)
			if err != nil {
				continue
			}
			step, _ := h.store.GetStep(id)
			typ := "reasoning"
			if step != nil {
				typ = step.Type
			}
			blocks = append(blocks, renderBlock(id, ref.Level, content, typ))
		}
	}

	// Zone 5: HintBlock (dynamic)
	hint := h.buildHintBlock(len(allIDs))
	blocks = append(blocks, hint)

	// Fit to window
	return h.fitToWindow(blocks, windowTokens), nil
}

// renderStepContent reads the appropriate .lN content for a step.
func (h *HybridManager) renderStepContent(stepID, level int) (string, error) {
	switch level {
	case 0:
		step, err := h.store.GetStep(stepID)
		if err != nil {
			return "", err
		}
		return step.Content, nil
	case 1:
		rec, err := h.store.GetL1(stepID)
		if err != nil {
			// Fallback to L0
			step, err2 := h.store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Content, nil
	case 2:
		rec, err := h.store.GetL2(stepID)
		if err != nil {
			step, err2 := h.store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		return strings.Join(rec.Facts, "\n"), nil
	case 3:
		rec, err := h.store.GetL3(stepID)
		if err != nil {
			step, err2 := h.store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Mask, nil
	default:
		return "", fmt.Errorf("unknown level %d", level)
	}
}

// renderBlock creates a RenderedBlock with the ⟨§N·type·Lx⟩ marker (§4.7).
func renderBlock(stepID, level int, content, stepType string) RenderedBlock {
	marker := fmt.Sprintf("⟨§%d·%s·L%d⟩", stepID, stepType, level)
	return RenderedBlock{
		StepID:  stepID,
		Level:   level,
		Content: marker + " " + content,
	}
}

// buildHintBlock creates zone 5 dynamic hint.
func (h *HybridManager) buildHintBlock(activeSteps int) RenderedBlock {
	pressure := float64(activeSteps) / 100.0 // simplified
	if pressure > 1.0 {
		pressure = 1.0
	}
	content := fmt.Sprintf("[hint] 上下文压力: %.0f%% | 当前任务组: #%d | 活跃 steps: %d | tail: %d steps",
		pressure*100, h.taskGroupID, activeSteps, h.cfg.TailKeepSteps)
	return RenderedBlock{
		StepID:  -1,
		Level:   0,
		Content: content,
	}
}

// fitToWindow trims blocks to fit within windowTokens by upgrading compression
// from the oldest steps first (zone 3 head).
func (h *HybridManager) fitToWindow(blocks []RenderedBlock, windowTokens int) []RenderedBlock {
	// Simple estimation: 1 token ≈ 4 chars
	totalTokens := 0
	for _, b := range blocks {
		totalTokens += len(b.Content) / 4
	}

	if totalTokens <= windowTokens {
		return blocks
	}

	// Try upgrading zone 3 blocks from head
	for i, b := range blocks {
		if b.StepID > 0 && b.Level < 3 {
			blocks[i].Level = b.Level + 1
			// Content would be re-rendered at higher compression
			// For now, truncate as approximation
			blocks[i].Content = blocks[i].Content[:len(blocks[i].Content)/2] + "...[compressed]"
		}

		// Recount
		totalTokens = 0
		for _, bl := range blocks {
			totalTokens += len(bl.Content) / 4
		}
		if totalTokens <= windowTokens {
			break
		}
	}

	return blocks
}

// TriggerCompression performs batch compression (§6.4 8-step process).
func (h *HybridManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	result := &CompressResult{NewLevels: make(map[int]int)}

	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}

		// Skip if already at max level for type
		maxLvl := MaxLevelForType(h.stepType(id))
		if ref.Level >= maxLvl {
			continue
		}

		// Compute decay
		rawDecay := h.sumTokens(0, int(atomic.LoadInt32(&h.currentStep)))
		decay := EffectiveDecay(*ref, rawDecay, false, false)
		target := TargetLevel(decay, h.stepType(id), h.cfg.Thresholds)
		if target > maxLvl {
			target = maxLvl
		}

		if target > ref.Level {
			ref.Level = target
			if err := h.store.UpsertRef(*ref); err != nil {
				continue
			}
			result.StepsCompressed++
			result.NewLevels[id] = target
		}
	}

	return result, nil
}

// Search performs RAG search across context history.
func (h *HybridManager) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	// Simple keyword search (full RAG in retrieval/ package)
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var hits []SearchHit
	q := strings.ToLower(query.Query)
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}

	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}
		if query.LevelMax > 0 && ref.Level > query.LevelMax {
			continue
		}

		content, err := h.renderStepContent(id, ref.Level)
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(content), q) {
			snippet := content
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			hits = append(hits, SearchHit{
				StepID:  id,
				Level:   ref.Level,
				Score:   1.0,
				Snippet: snippet,
				Type:    h.stepType(id),
			})
			if len(hits) >= limit {
				break
			}
		}
	}
	return hits, nil
}

// SearchLongMem searches long-term memory.
func (h *HybridManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return h.store.SearchLongMem(query, category, limit)
}

// Expand retrieves original content for a step, updating refs (§8B.3).
func (h *HybridManager) Expand(stepID int) (*ExpandResult, error) {
	step, err := h.store.GetStep(stepID)
	if err != nil {
		return nil, err
	}

	// Side effect: update refs (same as §N citation)
	ref, err := h.store.GetRef(stepID)
	if err == nil {
		ref.RefCount++
		ref.Strength += 0.1
		cur := int(atomic.LoadInt32(&h.currentStep))
		ref.LastRefAtStep = &cur
		_ = h.store.UpsertRef(*ref)
	}

	warning := ""
	if step.TokenCount > 4000 {
		warning = fmt.Sprintf("原文 %d tokens，注意窗口压力", step.TokenCount)
	}

	return &ExpandResult{
		StepID:  stepID,
		Level:   0,
		Content: step.Content,
		Tokens:  step.TokenCount,
		Warning: warning,
	}, nil
}

// Surround retrieves context around a step.
func (h *HybridManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var blocks []RenderedBlock
	for _, id := range ids {
		if id >= stepID-before && id <= stepID+after {
			ref, err := h.store.GetRef(id)
			if err != nil {
				continue
			}
			content, err := h.renderStepContent(id, ref.Level)
			if err != nil {
				continue
			}
			step, _ := h.store.GetStep(id)
			typ := "reasoning"
			if step != nil {
				typ = step.Type
			}
			blocks = append(blocks, renderBlock(id, ref.Level, content, typ))
		}
	}
	return blocks, nil
}

// Stats returns current context statistics.
func (h *HybridManager) Stats() ContextStats {
	ids, _ := h.store.AllActiveStepIDs()
	levels := make(map[int]int)
	totalTokens := 0
	for _, id := range ids {
		ref, err := h.store.GetRef(id)
		if err != nil {
			continue
		}
		levels[ref.Level]++
		step, err := h.store.GetStep(id)
		if err != nil {
			continue
		}
		totalTokens += step.TokenCount
	}

	hotFacts, _ := h.store.HotFacts(h.cfg.HotFacts.MinRefCount, h.cfg.HotFacts.MinStrength)

	return ContextStats{
		TotalSteps:     int(atomic.LoadInt32(&h.currentStep)),
		ActiveSteps:    len(ids),
		TotalTokens:    totalTokens,
		LevelCounts:    levels,
		HotFacts:       len(hotFacts),
		WindowPressure: float64(totalTokens) / float64(h.cfg.WindowTokens),
	}
}

// estimateTotalTokens returns the approximate total token count across
// all active steps. Uses the sum of TokenCount from the store.
func (h *HybridManager) estimateTotalTokens() int {
	ids, err := h.store.AllActiveStepIDs()
	if err != nil {
		return 0
	}
	total := 0
	for _, id := range ids {
		step, err := h.store.GetStep(id)
		if err != nil {
			continue
		}
		total += step.TokenCount
	}
	return total
}

// maybeWarmup triggers async pre-compression when total tokens
// approach the sweet-spot threshold.
func (h *HybridManager) maybeWarmup() {
	if h.sweetSpotTokens <= 0 {
		return
	}
	if h.warmupPending.Load() {
		return
	}
	ratio := float64(h.estimateTotalTokens()) / float64(h.sweetSpotTokens)
	if ratio < h.cfg.WarmupRatio {
		return
	}
	h.warmupPending.Store(true)
	go h.warmupCompress()
}

// warmupCompress pre-compresses old steps (outside the tail zone) to L1.
func (h *HybridManager) warmupCompress() {
	defer h.warmupPending.Store(false)

	ids, err := h.store.AllActiveStepIDs()
	if err != nil || len(ids) <= h.cfg.TailKeepSteps {
		return
	}
	tailStart := len(ids) - h.cfg.TailKeepSteps
	for _, id := range ids[:tailStart] {
		ref, err := h.store.GetRef(id)
		if err != nil || ref.Level >= 1 {
			continue
		}
		ref.Level = 1
		_ = h.store.UpsertRef(*ref)
	}
}

// decayTolerance gradually reduces sweet-spot tolerance back to 1.0.
func (h *HybridManager) decayTolerance() {
	h.toleranceMu.Lock()
	defer h.toleranceMu.Unlock()

	if h.sweetSpotTolerance > 1.05 {
		h.sweetSpotTolerance *= h.cfg.ToleranceDecayRate
		h.sweetSpotTokens = int(float64(h.sweetSpotOriginal) * h.sweetSpotTolerance)
	} else if h.sweetSpotTolerance > 1.0 {
		h.sweetSpotTolerance = 1.0
		h.sweetSpotTokens = h.sweetSpotOriginal
	}
}

// UpdateCacheBudget adjusts the sweet-spot threshold based on actual
// cached_tokens reported by the LLM provider. When cachedTokens > 0,
// the sweet spot is raised to protect the cached prefix from decay.
// The increase is capped at 1.5× the original sweet-spot value to
// prevent window pressure. This is an additive-only method; the
// ContextManager interface is not changed.
func (h *HybridManager) UpdateCacheBudget(cachedTokens int) {
	if cachedTokens <= 0 {
		return
	}
	h.toleranceMu.Lock()
	defer h.toleranceMu.Unlock()

	effective := cachedTokens + h.estimateTotalTokens()
	cap := int(float64(h.sweetSpotOriginal) * 1.5)
	if effective > cap {
		effective = cap
	}
	if effective > h.sweetSpotTokens {
		h.sweetSpotTokens = effective
		// Also raise tolerance so decayTolerance starts from the new level.
		if h.sweetSpotOriginal > 0 {
			h.sweetSpotTolerance = float64(effective) / float64(h.sweetSpotOriginal)
		}
	}
}

// Close releases resources.
func (h *HybridManager) Close() error {
	return nil
}

// --- Citation processing (§4.7.3) ---

var refCiteRe = regexp.MustCompile(`§(\d+)`)

// ProcessCitations parses §N references from LLM output and updates refs.
func (h *HybridManager) ProcessCitations(output string) {
	matches := refCiteRe.FindAllStringSubmatch(output, -1)
	currentStep := int(atomic.LoadInt32(&h.currentStep))

	for _, m := range matches {
		stepID := 0
		fmt.Sscanf(m[1], "%d", &stepID)
		if stepID <= 0 {
			continue
		}

		ref, err := h.store.GetRef(stepID)
		if err != nil {
			continue
		}
		ref.RefCount++
		ref.LastRefAtStep = &currentStep
		ref.Strength += 0.1
		_ = h.store.UpsertRef(*ref)
	}
}

// SanitizeOutput replaces accidental ⟨§ patterns in LLM output (§4.7.1).
func SanitizeOutput(text string) string {
	return strings.ReplaceAll(text, "\u27E8\u00A7", "[\u00A7]")
}

// NextStepID returns the next step ID (for testing).
func (h *HybridManager) NextStepID() int {
	return int(atomic.LoadInt32(&h.currentStep)) + 1
}
