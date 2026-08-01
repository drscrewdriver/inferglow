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

// PassthroughManager implements ModePassthrough — no compression, direct pass-through.
type PassthroughManager struct {
	cfg   Config
	store StepStoreLike
}

// NewPassthroughManager creates a passthrough context manager.
func NewPassthroughManager(cfg Config, store StepStoreLike) (ContextManager, error) {
	return &PassthroughManager{cfg: cfg, store: store}, nil
}

func (p *PassthroughManager) Mode() Mode {
	return ModePassthrough
}

func (p *PassthroughManager) Ingest(step StepRecord) error {
	// Passthrough still stores steps for potential mode switch later
	return p.store.AppendStep(step)
}

func (p *PassthroughManager) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	// Passthrough: return all steps at L0 (no compression)
	ids, err := p.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var blocks []RenderedBlock
	for _, id := range ids {
		step, err := p.store.GetStep(id)
		if err != nil {
			continue
		}
		blocks = append(blocks, RenderedBlock{
			StepID:  id,
			Level:   0,
			Content: step.Content,
		})
	}
	return blocks, nil
}

func (p *PassthroughManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	// Passthrough does not compress
	return &CompressResult{}, nil
}

func (p *PassthroughManager) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	// Passthrough does not support search
	return nil, nil
}

func (p *PassthroughManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	// Passthrough does not support long-term memory search
	return nil, nil
}

func (p *PassthroughManager) Expand(stepID int, full bool) (*ExpandResult, error) {
	step, err := p.store.GetStep(stepID)
	if err != nil {
		return nil, err
	}
	return &ExpandResult{
		StepID:  stepID,
		Level:   0,
		Content: step.Content,
		Tokens:  step.TokenCount,
	}, nil
}

func (p *PassthroughManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	ids, err := p.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var blocks []RenderedBlock
	for _, id := range ids {
		if id >= stepID-before && id <= stepID+after {
			step, err := p.store.GetStep(id)
			if err != nil {
				continue
			}
			blocks = append(blocks, RenderedBlock{
				StepID:  id,
				Level:   0,
				Content: step.Content,
			})
		}
	}
	return blocks, nil
}

func (p *PassthroughManager) Stats() ContextStats {
	ids, _ := p.store.AllActiveStepIDs()
	return ContextStats{
		TotalSteps:  len(ids),
		ActiveSteps: len(ids),
	}
}

func (p *PassthroughManager) Close() error {
	return nil
}
