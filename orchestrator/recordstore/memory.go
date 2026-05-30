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
	"sort"
	"sync"
	"time"
)

// Compile-time check that MemoryStore implements RecordStore.
var _ RecordStore = (*MemoryStore)(nil)

// MemoryStore is an in-memory RecordStore implementation suitable for
// testing and single-process use.
type MemoryStore struct {
	mu          sync.RWMutex
	records     map[string]*Record
	checkpoints map[string]*Checkpoint // keyed by executionID
	snapshots   map[string]*Snapshot   // keyed by executionID
	events      []Event
}

// NewMemoryStore creates an empty in-memory RecordStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records:     make(map[string]*Record),
		checkpoints: make(map[string]*Checkpoint),
		snapshots:   make(map[string]*Snapshot),
	}
}

func (s *MemoryStore) AppendRecord(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	s.records[rec.ID] = &rec
	return nil
}

func (s *MemoryStore) GetRecord(id string) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

func (s *MemoryStore) QueryRecords(scope Scope, kind string) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Record
	for _, rec := range s.records {
		if kind != "" && rec.Kind != kind {
			continue
		}
		if scope.AgentID != "" && rec.Scope.AgentID != scope.AgentID {
			continue
		}
		if scope.SessionID != "" && rec.Scope.SessionID != scope.SessionID {
			continue
		}
		if scope.ExecutionID != "" && rec.Scope.ExecutionID != scope.ExecutionID {
			continue
		}
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

func (s *MemoryStore) SaveCheckpoint(executionID string, cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	cp.ExecutionID = executionID
	s.checkpoints[executionID] = cp
	return nil
}

func (s *MemoryStore) LoadCheckpoint(executionID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.checkpoints[executionID]
	if !ok {
		return nil, nil
	}
	cpCopy := *cp
	return &cpCopy, nil
}

func (s *MemoryStore) SaveSnapshot(executionID string, snap *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = time.Now()
	}
	snap.ExecutionID = executionID
	s.snapshots[executionID] = snap
	return nil
}

func (s *MemoryStore) LoadSnapshot(executionID string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.snapshots[executionID]
	if !ok {
		return nil, nil
	}
	snapCopy := *snap
	return &snapCopy, nil
}

func (s *MemoryStore) AppendEvent(evt Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	s.events = append(s.events, evt)
	return nil
}

func (s *MemoryStore) QueryEvents(filter EventFilter) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Event
	for _, evt := range s.events {
		if filter.ExecutionID != "" && evt.ExecutionID != filter.ExecutionID {
			continue
		}
		if filter.Kind != "" && evt.Kind != filter.Kind {
			continue
		}
		if !filter.Since.IsZero() && evt.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && evt.Timestamp.After(filter.Until) {
			continue
		}
		out = append(out, evt)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}
