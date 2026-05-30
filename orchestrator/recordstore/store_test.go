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

package recordstore

import (
	"testing"
	"time"
)

func TestMemoryStoreRecords(t *testing.T) {
	s := NewMemoryStore()

	rec := Record{
		ID:   "r1",
		Scope: Scope{AgentID: "a1", SessionID: "s1"},
		Kind: "decision",
		Data: map[string]string{"action": "execute"},
	}
	if err := s.AppendRecord(rec); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	got, err := s.GetRecord("r1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.Kind != "decision" {
		t.Fatalf("expected decision, got %s", got.Kind)
	}

	// Not found.
	got, err = s.GetRecord("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent record")
	}
}

func TestMemoryStoreQueryRecords(t *testing.T) {
	s := NewMemoryStore()
	s.AppendRecord(Record{ID: "r1", Scope: Scope{AgentID: "a1"}, Kind: "decision"})
	s.AppendRecord(Record{ID: "r2", Scope: Scope{AgentID: "a1"}, Kind: "action_result"})
	s.AppendRecord(Record{ID: "r3", Scope: Scope{AgentID: "a2"}, Kind: "decision"})

	// Query by agent.
	recs, _ := s.QueryRecords(Scope{AgentID: "a1"}, "")
	if len(recs) != 2 {
		t.Fatalf("expected 2 records for a1, got %d", len(recs))
	}

	// Query by kind.
	recs, _ = s.QueryRecords(Scope{}, "decision")
	if len(recs) != 2 {
		t.Fatalf("expected 2 decision records, got %d", len(recs))
	}

	// Query by both.
	recs, _ = s.QueryRecords(Scope{AgentID: "a1"}, "decision")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	// Query all.
	recs, _ = s.QueryRecords(Scope{}, "")
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
}

func TestMemoryStoreCheckpoint(t *testing.T) {
	s := NewMemoryStore()

	// Load non-existent.
	cp, err := s.LoadCheckpoint("exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp != nil {
		t.Fatal("expected nil checkpoint")
	}

	// Save and load.
	cp = &Checkpoint{StepIndex: 3, State: "running"}
	if err := s.SaveCheckpoint("exec-1", cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := s.LoadCheckpoint("exec-1")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected checkpoint")
	}
	if loaded.StepIndex != 3 {
		t.Fatalf("expected step 3, got %d", loaded.StepIndex)
	}
}

func TestMemoryStoreSnapshot(t *testing.T) {
	s := NewMemoryStore()

	snap, err := s.LoadSnapshot("exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap != nil {
		t.Fatal("expected nil snapshot")
	}

	snap = &Snapshot{Data: map[string]any{"status": "paused"}, Metadata: map[string]string{"v": "1"}}
	if err := s.SaveSnapshot("exec-1", snap); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := s.LoadSnapshot("exec-1")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected snapshot")
	}
	if loaded.Metadata["v"] != "1" {
		t.Fatalf("expected v=1, got %s", loaded.Metadata["v"])
	}
}

func TestMemoryStoreEvents(t *testing.T) {
	s := NewMemoryStore()
	now := time.Now()

	s.AppendEvent(Event{ID: "e1", ExecutionID: "exec-1", Kind: "started", Timestamp: now})
	s.AppendEvent(Event{ID: "e2", ExecutionID: "exec-1", Kind: "action_started", Timestamp: now.Add(time.Second)})
	s.AppendEvent(Event{ID: "e3", ExecutionID: "exec-2", Kind: "started", Timestamp: now.Add(2 * time.Second)})

	// Query by execution.
	evts, _ := s.QueryEvents(EventFilter{ExecutionID: "exec-1"})
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}

	// Query by kind.
	evts, _ = s.QueryEvents(EventFilter{Kind: "started"})
	if len(evts) != 2 {
		t.Fatalf("expected 2 started events, got %d", len(evts))
	}

	// Query with limit.
	evts, _ = s.QueryEvents(EventFilter{Limit: 1})
	if len(evts) != 1 {
		t.Fatalf("expected 1 event with limit, got %d", len(evts))
	}

	// Query with time range.
	evts, _ = s.QueryEvents(EventFilter{Since: now.Add(500 * time.Millisecond), Until: now.Add(1500 * time.Millisecond)})
	if len(evts) != 1 {
		t.Fatalf("expected 1 event in time range, got %d", len(evts))
	}
	if evts[0].ID != "e2" {
		t.Fatalf("expected e2, got %s", evts[0].ID)
	}
}

func TestMemoryStoreTimestampAutoSet(t *testing.T) {
	s := NewMemoryStore()
	s.AppendRecord(Record{ID: "r1", Kind: "test"})

	rec, _ := s.GetRecord("r1")
	if rec.Timestamp.IsZero() {
		t.Fatal("expected auto-set timestamp")
	}
}
