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
	"sync"
	"time"
)

// AssemblyManager implements ModeAssembly — the 9-layer context assembly engine
// (线A / C轨). This is the wp-a1 skeleton: dependencies are wired via
// NewAssemblyManager and the interface is fully satisfied with empty/default
// implementations. The Retrieval/Render/Decay layers are filled in later
// sub-stages (wp-a2..a5) entirely within this type, without touching the
// frozen ContextManager interface. See docs/plans/00-integrated-master-spec.md 线A.
type AssemblyManager struct {
	cfg   Config
	store StepStoreLike

	mu sync.RWMutex

	// A-11: assembly audit trail (kept; other speculative types are YAGNI).
	audit *AssemblyAudit
}

// AssemblyAudit records a bounded ring of assembly entry audits.
type AssemblyAudit struct {
	mu         sync.Mutex
	entries    []AuditEntry
	maxEntries int
}

// NewAssemblyAudit creates an empty audit trail with a bounded capacity.
func NewAssemblyAudit() *AssemblyAudit {
	return &AssemblyAudit{maxEntries: 32}
}

// Record appends an audit entry, dropping the oldest beyond capacity.
func (a *AssemblyAudit) Record(entry AuditEntry) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, entry)
	if len(a.entries) > a.maxEntries {
		a.entries = a.entries[len(a.entries)-a.maxEntries:]
	}
}

// AuditEntry is a single round's assembly audit summary.
type AuditEntry struct {
	RoundID     string
	Phase       string
	Layers      []LayerAudit
	TotalTokens int
	CacheHits   int
	CacheMisses int
	Duration    time.Duration
}

// LayerAudit describes one layer's participation in an assembly round.
type LayerAudit struct {
	Layer      string
	BlockCount int
	Tokens     int
}

// NewAssemblyManager creates an assembly mode context manager.
// The StepStore is shared and reused across mode switches (Registry).
func NewAssemblyManager(cfg Config, store StepStoreLike) (ContextManager, error) {
	return &AssemblyManager{
		cfg:   cfg,
		store: store,
		audit: NewAssemblyAudit(),
	}, nil
}

func (a *AssemblyManager) Mode() Mode { return ModeAssembly }

// Ingest wires the step into the shared store and keeps an active ref so the
// assembler (BuildContext/Stats) can see it. Assembly behaviour is deliberately
// minimal at wp-a1; the assembly engine is added later.
func (a *AssemblyManager) Ingest(step StepRecord) error {
	if err := a.store.AppendStep(step); err != nil {
		return err
	}
	// Maintain an active ref so AllActiveStepIDs/BuildContext surface the step.
	return a.store.UpsertRef(RefRecord{
		StepID:  step.StepID,
		Level:   0,
		RefCount: 0,
		Strength: 1.0,
	})
}

// BuildContext is a skeleton render: returns steps at L0 (no assembly yet).
func (a *AssemblyManager) BuildContext(ctx context.Context, windowTokens int) ([]RenderedBlock, error) {
	ids, err := a.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}
	blocks := make([]RenderedBlock, 0, len(ids))
	for _, id := range ids {
		step, err := a.store.GetStep(id)
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

func (a *AssemblyManager) TriggerCompression(ctx context.Context, opts CompressOpts) (*CompressResult, error) {
	return &CompressResult{NewLevels: make(map[int]int)}, nil
}

func (a *AssemblyManager) Search(ctx context.Context, query SearchQuery) ([]SearchHit, error) {
	return nil, fmt.Errorf("assembly: search not implemented (wp-a3)")
}

func (a *AssemblyManager) SearchLongMem(ctx context.Context, query string, category string, limit int) ([]LongMemRecord, error) {
	return nil, fmt.Errorf("assembly: long-term memory search not implemented")
}

func (a *AssemblyManager) Expand(stepID int) (*ExpandResult, error) {
	step, err := a.store.GetStep(stepID)
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

func (a *AssemblyManager) Surround(stepID int, before, after int) ([]RenderedBlock, error) {
	ids, err := a.store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}
	var blocks []RenderedBlock
	for _, id := range ids {
		if id >= stepID-before && id <= stepID+after {
			step, err := a.store.GetStep(id)
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

func (a *AssemblyManager) Stats() ContextStats {
	ids, _ := a.store.AllActiveStepIDs()
	return ContextStats{
		TotalSteps:  len(ids),
		ActiveSteps: len(ids),
	}
}

func (a *AssemblyManager) Close() error {
	return nil
}

// compile-time assertion that AssemblyManager satisfies ContextManager.
var _ ContextManager = (*AssemblyManager)(nil)