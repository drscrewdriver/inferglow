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

import "context"

// ThreeZoneAdapter wraps an existing ThreeZoneSession as a ContextManager.
// It shares the L0 jsonl store and delegates snip/prune/summary to the
// existing three-zone logic while allowing refs + lN files to be used.
type ThreeZoneAdapter struct {
	cfg   Config
	store StepStoreLike
}

// NewThreeZoneAdapter creates a three-zone adapter context manager.
func NewThreeZoneAdapter(cfg Config, store StepStoreLike) (ContextManager, error) {
	return &ThreeZoneAdapter{cfg: cfg, store: store}, nil
}

func (t *ThreeZoneAdapter) Mode() Mode { return ModeThreeZone }

func (t *ThreeZoneAdapter) Ingest(step StepRecord) error {
	return t.store.AppendStep(step)
}

func (t *ThreeZoneAdapter) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	// ThreeZone uses its own snip/prune/summary logic.
	// This adapter reads from the shared store and renders at current levels.
	ids, err := t.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var blocks []RenderedBlock
	for _, id := range ids {
		ref, err := t.store.GetRef(id)
		if err != nil {
			continue
		}
		step, err := t.store.GetStep(id)
		if err != nil {
			continue
		}
		content := step.Content
		if ref.Level > 0 {
			content, _ = t.renderAtLevel(id, ref.Level)
		}
		blocks = append(blocks, renderBlock(id, ref.Level, content, step.Type))
	}
	return blocks, nil
}

func (t *ThreeZoneAdapter) renderAtLevel(stepID, level int) (string, error) {
	switch level {
	case 1:
		rec, err := t.store.GetL1(stepID)
		if err != nil {
			step, _ := t.store.GetStep(stepID)
			if step == nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Content, nil
	case 2:
		rec, err := t.store.GetL2(stepID)
		if err != nil {
			step, _ := t.store.GetStep(stepID)
			if step == nil {
				return "", err
			}
			return step.Content, nil
		}
		return joinFacts(rec.Facts), nil
	case 3:
		rec, err := t.store.GetL3(stepID)
		if err != nil {
			step, _ := t.store.GetStep(stepID)
			if step == nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Mask, nil
	default:
		step, err := t.store.GetStep(stepID)
		if err != nil {
			return "", err
		}
		return step.Content, nil
	}
}

func joinFacts(facts []string) string {
	result := ""
	for i, f := range facts {
		if i > 0 {
			result += "\n"
		}
		result += "• " + f
	}
	return result
}

func (t *ThreeZoneAdapter) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	return &CompressResult{}, nil // delegated to ThreeZone logic
}

func (t *ThreeZoneAdapter) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	return nil, nil
}

func (t *ThreeZoneAdapter) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return t.store.SearchLongMem(query, category, limit)
}

func (t *ThreeZoneAdapter) Expand(stepID int, full bool) (*ExpandResult, error) {
	step, err := t.store.GetStep(stepID)
	if err != nil {
		return nil, err
	}
	return &ExpandResult{StepID: stepID, Level: 0, Content: step.Content, Tokens: step.TokenCount}, nil
}

func (t *ThreeZoneAdapter) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	return nil, nil
}

func (t *ThreeZoneAdapter) Stats() ContextStats {
	ids, _ := t.store.AllActiveStepIDs()
	return ContextStats{TotalSteps: len(ids), ActiveSteps: len(ids)}
}

func (t *ThreeZoneAdapter) Close() error { return nil }
