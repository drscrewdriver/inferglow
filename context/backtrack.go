package contextmgr

import (
	"fmt"
	"sort"
)

// buildBacktrackBlock constructs the Layer-8 same-group backtrack block (A-9).
// Assumes caller holds h.mu.RLock. Returns false if not triggered.
func (h *HybridManager) buildBacktrackBlock(allIDs []int) (RenderedBlock, bool) {
	cfg := h.cfg.Backtrack
	if len(allIDs) < 2 {
		return RenderedBlock{}, false
	}

	// Trigger: last two active steps share the same TaskGroupID.
	prevRef, err1 := h.store.GetRef(allIDs[len(allIDs)-2])
	curRef, err2 := h.store.GetRef(allIDs[len(allIDs)-1])
	if err1 != nil || err2 != nil {
		return RenderedBlock{}, false
	}
	if prevRef.TaskGroupID != curRef.TaskGroupID {
		return RenderedBlock{}, false
	}
	group := curRef.TaskGroupID

	// Collect same-group non-transient steps with scoring.
	type scored struct {
		id      int
		score   float64
		content string
	}
	var candidates []scored
	maxStep := allIDs[len(allIDs)-1]
	for _, id := range allIDs {
		ref, err := h.store.GetRef(id)
		if err != nil || ref.TaskGroupID != group {
			continue
		}
		step, err := h.store.GetStep(id)
		if err != nil || step.Transient {
			continue
		}
		// Recency+Strength scoring (inline, same formula as retrieval/recency.go)
		recency := 0.0
		if ref.LastRefAtStep != nil && maxStep > 0 {
			recency = float64(*ref.LastRefAtStep) / float64(maxStep)
		}
		score := cfg.RecencyW*recency + cfg.StrengthW*ref.Strength
		content := truncateRunes(step.Content, cfg.MaxCharsPerStep)
		candidates = append(candidates, scored{id, score, content})
	}
	if len(candidates) == 0 {
		return RenderedBlock{}, false
	}

	// Sort descending by score, take top-K.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > cfg.TopK {
		candidates = candidates[:cfg.TopK]
	}

	// Render block.
	content := fmt.Sprintf("[backtrack | group #%d | top-%d]\n", group, len(candidates))
	for i, c := range candidates {
		content += fmt.Sprintf("  #%d (§%d) %s\n", i+1, c.id, c.content)
	}
	content += "[/backtrack]"
	return RenderedBlock{StepID: -5, Level: 0, Content: content}, true
}

// truncateRunes truncates s to at most max runes, appending "…" if truncated.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
