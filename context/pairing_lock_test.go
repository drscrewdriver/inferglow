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
	"testing"
)

// markerStore wraps fakeStore with compaction marker support so the lock
// lifecycle can be observed in tests.
type markerStore struct {
	*fakeStore
	markers []AuditRecord
}

func (m *markerStore) AppendCompactionMarker(action, compactionID string) error {
	m.markers = append(m.markers, AuditRecord{Action: action, Detail: compactionID})
	return nil
}

func (m *markerStore) OrphanCompactions() ([]string, error) {
	var ids []string
	for _, r := range m.markers {
		switch r.Action {
		case "compaction/start":
			ids = append(ids, r.Detail)
		case "compaction/end":
			for i, id := range ids {
				if id == r.Detail {
					ids = append(ids[:i], ids[i+1:]...)
					break
				}
			}
		}
	}
	return ids, nil
}

func TestTriggerCompression_ConcurrentSecondIsBusy(t *testing.T) {
	store := &markerStore{fakeStore: newFakeStore()}
	mgr, err := NewHybridManager(DefaultConfig(), store)
	if err != nil {
		t.Fatalf("NewHybridManager error: %v", err)
	}
	h := mgr.(*HybridManager)

	if !h.tryAcquireCompactionLock("test-holder") {
		t.Fatal("expected first lock acquisition to succeed")
	}
	defer h.releaseCompactionLock()

	if _, err := h.TriggerCompression(context.Background(), CompressOpts{}); err != ErrCompressionBusy {
		t.Errorf("expected ErrCompressionBusy, got %v", err)
	}
}

func TestTriggerCompression_WritesAuditMarkers(t *testing.T) {
	store := &markerStore{fakeStore: newFakeStore()}
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "x", TokenCount: 100}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	mgr, err := NewHybridManager(DefaultConfig(), store)
	if err != nil {
		t.Fatalf("NewHybridManager error: %v", err)
	}
	h := mgr.(*HybridManager)

	res, err := h.TriggerCompression(context.Background(), CompressOpts{})
	if err != nil {
		t.Fatalf("TriggerCompression error: %v", err)
	}
	if res.CompactionID == "" {
		t.Errorf("expected a compaction transaction id")
	}

	// Lifecycle markers must bracket the run: start before end, same ID.
	if len(store.markers) != 2 {
		t.Fatalf("expected 2 markers, got %d: %+v", len(store.markers), store.markers)
	}
	if store.markers[0].Action != "compaction/start" || store.markers[1].Action != "compaction/end" {
		t.Errorf("expected start then end markers, got %+v", store.markers)
	}
	if store.markers[0].Detail != res.CompactionID || store.markers[1].Detail != res.CompactionID {
		t.Errorf("markers must carry the run's CompactionID, got %+v", store.markers)
	}
}

func TestOrphanLockDetectedOnInit(t *testing.T) {
	store := &markerStore{fakeStore: newFakeStore()}
	store.markers = []AuditRecord{
		{Action: "compaction/start", Detail: "orphan-1"},
		{Action: "compaction/start", Detail: "orphan-2"},
		{Action: "compaction/end", Detail: "orphan-2"},
	}

	// Startup must not fail; the orphan scan only warns via log.
	if _, err := NewHybridManager(DefaultConfig(), store); err != nil {
		t.Fatalf("NewHybridManager must tolerate orphan locks: %v", err)
	}

	orphans, err := store.OrphanCompactions()
	if err != nil {
		t.Fatalf("OrphanCompactions error: %v", err)
	}
	if len(orphans) != 1 || orphans[0] != "orphan-1" {
		t.Errorf("expected orphan [orphan-1], got %v", orphans)
	}
}

func TestRenderedBlock_SourceStepIDs(t *testing.T) {
	b := renderBlock(7, 2, "content", "tool")
	if len(b.SourceStepIDs) != 1 || b.SourceStepIDs[0] != 7 {
		t.Errorf("SourceStepIDs = %v, want [7]", b.SourceStepIDs)
	}
}
