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

package compress

import (
	"context"
	"sync/atomic"

	"github.com/inferglow/context"
)

// IdleConsolidator implements layer 2 of the five-layer defense (§6.1).
// It triggers lightweight consolidation after N idle steps.
type IdleConsolidator struct {
	store     contextmgr.StepStoreLike
	cfg       contextmgr.Config
	idleCount int32
}

// NewIdleConsolidator creates an idle consolidation handler.
func NewIdleConsolidator(store contextmgr.StepStoreLike, cfg contextmgr.Config) *IdleConsolidator {
	return &IdleConsolidator{store: store, cfg: cfg}
}

// Tick is called on each step. Returns true if idle consolidation should trigger.
func (ic *IdleConsolidator) Tick() bool {
	if !ic.cfg.IdleConsolidation.Enabled {
		return false
	}
	count := atomic.AddInt32(&ic.idleCount, 1)
	return int(count) >= ic.cfg.IdleConsolidation.IdleSteps
}

// Reset resets the idle counter (called on Ingest).
func (ic *IdleConsolidator) Reset() {
	atomic.StoreInt32(&ic.idleCount, 0)
}

// Consolidate performs lightweight idle consolidation (§4C.4).
// Unlike global consolidation, this does NOT:
//   - Create checkpoint copies
//   - Trigger head_buffer version changes
//   - Flush Redis cache
//
// It only:
//   - Pre-marks candidate L4 steps (pending_l4=true)
//   - Strengthens frequently-referenced steps
//   - Merges adjacent L2 facts from same task group
func (ic *IdleConsolidator) Consolidate(ctx context.Context) error {
	ids, err := ic.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	for _, id := range ids {
		ref, err := ic.store.GetRef(id)
		if err != nil {
			continue
		}

		// Strengthen: boost steps with ref_count > 0
		if ref.RefCount > 0 {
			ref.Strength += 0.05
			_ = ic.store.UpsertRef(*ref)
		}

		// Pre-mark L4 candidates: reasoning/plan/failed with decay > L4 threshold
		step, err := ic.store.GetStep(id)
		if err != nil {
			continue
		}

		if step.Type == "reasoning" || step.Type == "plan" || step.Type == "failed" {
			if ref.Level >= 3 && ref.RefCount == 0 {
				ref.PendingL4 = true
				_ = ic.store.UpsertRef(*ref)
			}
		}
	}

	return nil
}

// ExecutePendingL4 removes steps marked with pending_l4=true.
// Called during global consolidation (§4C.2 step 3).
func (ic *IdleConsolidator) ExecutePendingL4() error {
	ids, err := ic.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	for _, id := range ids {
		ref, err := ic.store.GetRef(id)
		if err != nil {
			continue
		}
		if ref.PendingL4 {
			_ = ic.store.RemoveRef(id)
		}
	}
	return nil
}
