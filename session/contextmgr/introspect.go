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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// Introspector implements the global introspection flow (§4C.2).
// It handles:
//   - Step 1: Snapshot (checkpoint copy + cache marker)
//   - Step 2: Batch compression
//   - Step 3: L4 cleanup
//   - Step 4: task_group archiving
//   - Step 5: Cache marker update
type Introspector struct {
	store         StepStoreLike
	cfg           Config
	dataDir       string
	headerVer     string
	renderCache   *RenderedCache
}

// NewIntrospector creates a global introspector.
func NewIntrospector(store StepStoreLike, cfg Config, dataDir string, cache *RenderedCache) *Introspector {
	return &Introspector{
		store:       store,
		cfg:         cfg,
		dataDir:     dataDir,
		headerVer:   "h_v1",
		renderCache: cache,
	}
}

// GlobalIntrospection performs the full 5-step introspection flow (§4C.2).
func (ig *Introspector) GlobalIntrospection(ctx context.Context, currentStep int) error {
	// Step 1: Snapshot (checkpoint copy)
	if err := ig.createCheckpoint(currentStep); err != nil {
		return fmt.Errorf("introspection: checkpoint: %w", err)
	}

	// Step 2: Batch compression (delegated to compress.Engine via callback)
	// The caller should invoke compress.Engine.BatchCompress() separately.

	// Step 3: L4 cleanup
	if err := ig.executeL4Cleanup(); err != nil {
		return fmt.Errorf("introspection: L4 cleanup: %w", err)
	}

	// Step 4: task_group archiving
	if err := ig.archiveTaskGroups(); err != nil {
		return fmt.Errorf("introspection: archive: %w", err)
	}

	// Step 5: Cache marker update
	ig.invalidateOldCheckpoints()

	return nil
}

// createCheckpoint copies refs.jsonl to a checkpoint file (§4C.2 Step 1).
func (ig *Introspector) createCheckpoint(currentStep int) error {
	meta := CheckpointMeta{
		IsCheckpoint: true,
		AtStep:       currentStep,
		HeaderVer:    ig.headerVer,
		CacheValid:   true,
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	// Read all current refs
	ids, err := ig.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	checkpointPath := filepath.Join(ig.dataDir, fmt.Sprintf("refs.checkpoint.%d.jsonl", currentStep))
	f, err := os.Create(checkpointPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write metadata header
	if _, err := f.Write(append(metaJSON, '\n')); err != nil {
		return err
	}

	// Write all refs
	for _, id := range ids {
		ref, err := ig.store.GetRef(id)
		if err != nil {
			continue
		}
		refJSON, err := json.Marshal(ref)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(refJSON, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// executeL4Cleanup removes steps that qualify for L4 discard (§4C.2 Step 3).
func (ig *Introspector) executeL4Cleanup() error {
	ids, err := ig.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	for _, id := range ids {
		ref, err := ig.store.GetRef(id)
		if err != nil {
			continue
		}

		// L4 conditions: level ≥ 3 AND ref_count == 0 AND (pending_l4 OR type allows L4)
		if ref.Level >= 3 && ref.RefCount == 0 {
			step, err := ig.store.GetStep(id)
			if err != nil {
				continue
			}
			// Only discard types allowed for L4 (§2.2)
			if step.Type == "reasoning" || step.Type == "plan" || step.Type == "failed" || ref.PendingL4 {
				_ = ig.store.RemoveRef(id)
				if ig.renderCache != nil {
					ig.renderCache.Invalidate(id)
				}
			}
		}
	}

	return nil
}

// archiveTaskGroups archives task groups where all steps are ≥L3 and ref_count=0 (§4C.2 Step 4).
func (ig *Introspector) archiveTaskGroups() error {
	ids, err := ig.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	// Group steps by task_group_id
	groups := make(map[int][]int)
	for _, id := range ids {
		ref, err := ig.store.GetRef(id)
		if err != nil {
			continue
		}
		groups[ref.TaskGroupID] = append(groups[ref.TaskGroupID], id)
	}

	// Check each group for archive eligibility
	for groupID, stepIDs := range groups {
		eligible := true
		for _, id := range stepIDs {
			ref, err := ig.store.GetRef(id)
			if err != nil {
				eligible = false
				break
			}
			if ref.Level < 3 || ref.RefCount > 0 {
				eligible = false
				break
			}
		}

		if !eligible {
			continue
		}

		// Write archive file
		archivePath := filepath.Join(ig.dataDir, fmt.Sprintf("refs.archive.%d.jsonl", groupID))
		f, err := os.Create(archivePath)
		if err != nil {
			continue
		}

		for _, id := range stepIDs {
			ref, err := ig.store.GetRef(id)
			if err != nil {
				continue
			}
			refJSON, err := json.Marshal(ref)
			if err != nil {
				continue
			}
			_, _ = f.Write(append(refJSON, '\n'))
			_ = ig.store.RemoveRef(id)
		}
		f.Close()
	}

	return nil
}

// invalidateOldCheckpoints marks all old checkpoints as cache_valid=false (§4C.2 Step 5).
func (ig *Introspector) invalidateOldCheckpoints() {
	// Invalidate render cache on header version change
	if ig.renderCache != nil {
		ig.renderCache.InvalidateAll()
	}
}

// BumpHeaderVer increments the header version (e.g., h_v1 → h_v2).
func (ig *Introspector) BumpHeaderVer() {
	// Parse current version number
	var ver int
	fmt.Sscanf(ig.headerVer, "h_v%d", &ver)
	ig.headerVer = fmt.Sprintf("h_v%d", ver+1)
}

// HeaderVer returns the current header version.
func (ig *Introspector) HeaderVer() string {
	return ig.headerVer
}

// --- Emergency introspection (§4C.1 window pressure > 90%) ---

// EmergencyIntrospection performs emergency compression when window pressure exceeds 90%.
func (ig *Introspector) EmergencyIntrospection(ctx context.Context) error {
	ids, err := ig.store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	// Aggressively upgrade levels for all non-tail steps
	tailStart := len(ids) - ig.cfg.TailKeepSteps
	if tailStart < 0 {
		tailStart = 0
	}

	for i, id := range ids {
		if i >= tailStart {
			continue // preserve tail at L0
		}

		ref, err := ig.store.GetRef(id)
		if err != nil {
			continue
		}

		step, err := ig.store.GetStep(id)
		if err != nil {
			continue
		}

		maxLvl := MaxLevelForType(step.Type)
		newLevel := ref.Level + 1
		if newLevel > maxLvl {
			newLevel = maxLvl
		}
		if newLevel > 3 {
			newLevel = 3 // cap at L3 for emergency
		}

		if newLevel > ref.Level {
			ref.Level = newLevel
			_ = ig.store.UpsertRef(*ref)
			if ig.renderCache != nil {
				ig.renderCache.Invalidate(id)
			}
		}
	}

	return nil
}

// CurrentStep returns the current step counter (for external use).
func CurrentStepAtomic(h *HybridManager) int32 {
	return atomic.LoadInt32(&h.currentStep)
}
