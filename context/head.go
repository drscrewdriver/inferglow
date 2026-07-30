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
	"fmt"
	"time"
)

// RewriteHeadBuffer replaces the Zone 1 head buffer and archives the
// previous version for audit/rollback purposes.
func (h *HybridManager) RewriteHeadBuffer(newContent []RenderedBlock, newVersion string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Archive old head with monotonic sequence number (A-3 version chain).
	if len(h.headBuffer) > 0 {
		h.headSeq++
		h.archivedHeads = append(h.archivedHeads, ArchivedHead{
			Content:    h.headBuffer,
			Version:    h.headBufferVer,
			ArchivedAt: time.Now(),
			Seq:        h.headSeq,
		})
	}

	h.headBuffer = newContent
	h.headBufferVer = newVersion
}

// IsHeadBufferEmpty reports whether Zone 1 (head buffer) has been populated.
func (h *HybridManager) IsHeadBufferEmpty() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.headBuffer) == 0
}

// GetArchivedHeads returns the list of archived head buffers.
func (h *HybridManager) GetArchivedHeads() []ArchivedHead {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ArchivedHead, len(h.archivedHeads))
	copy(out, h.archivedHeads)
	return out
}

// HeadBlocks returns a copy of the current Zone 1 head buffer blocks.
func (h *HybridManager) HeadBlocks() []RenderedBlock {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]RenderedBlock, len(h.headBuffer))
	copy(out, h.headBuffer)
	return out
}

// --- A-3: Rebackground semantic narrowing + version chain ---

// GetLayerVersion retrieves a historical head buffer version by sequence number.
// Only layer 4 (task background) is tracked currently.
func (h *HybridManager) GetLayerVersion(layer int, seq int) (ArchivedHead, bool) {
	if layer != 4 {
		return ArchivedHead{}, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for i := len(h.archivedHeads) - 1; i >= 0; i-- {
		if h.archivedHeads[i].Seq == seq {
			return h.archivedHeads[i], true
		}
	}
	return ArchivedHead{}, false
}

// RebackgroundRequest specifies what to recompute (A-3).
type RebackgroundRequest struct {
	NewTaskDescription     string
	NewProhibitions        []string
	CheckProhibitionChange bool
}

// Rebackground narrows the rebuild to L4 (head buffer) only.
// L1/L2/L3 are never touched. L5 (constitutional) is recomputed
// only when CheckProhibitionChange=true.
// Additive-only method; ContextManager interface unchanged.
func (h *HybridManager) Rebackground(req RebackgroundRequest) {
	// L4: rebuild task background block. An empty description must NOT clobber the
	// existing L4 head buffer, so the rewrite is skipped entirely when empty.
	if req.NewTaskDescription != "" {
		blocks := []RenderedBlock{{
			StepID:  0,
			Level:   0,
			Content: req.NewTaskDescription,
		}}
		h.RewriteHeadBuffer(blocks, fmt.Sprintf("rebg-%d", h.headSeq+1))
	}

	// L5: conditionally refresh prohibitions
	if req.CheckProhibitionChange && len(req.NewProhibitions) > 0 {
		h.constitutionalMu.Lock()
		h.constitutionalEntries = req.NewProhibitions
		h.constitutionalMu.Unlock()
	}
	// Explicitly: no UpsertRef, no markFusionDirty, no TriggerCompression.
}
