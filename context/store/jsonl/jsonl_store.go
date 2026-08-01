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

// Package jsonl implements the StepStore interface using local JSONL files.
//
// File layout:
//
//	{uuid}.jsonl          - L0 original content (append-only)
//	{uuid}.refs.jsonl     - reference tracking + level markers (upsert per step)
//	{uuid}.l1.jsonl       - L1 simple compression (append, indexed by step_id)
//	{uuid}.l2.jsonl       - L2 fact extraction (append, indexed by step_id)
//	{uuid}.l3.jsonl       - L3 behavior mask (append, indexed by step_id)
//	{uuid}.longmem.jsonl  - long-term memory entries
package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/inferglow/context"
)

// Store implements store.StepStore using local JSONL files.
type Store struct {
	mu       sync.RWMutex
	dir      string
	uuid     string
	closed   bool
	stepIdx  map[int]*contextmgr.StepRecord // in-memory index for fast lookup
	refIdx   map[int]*contextmgr.RefRecord
	l1Idx    map[int]*contextmgr.L1Record
	l2Idx    map[int]*contextmgr.L2Record
	l3Idx    map[int]*contextmgr.L3Record
	memIdx   map[string]*contextmgr.LongMemRecord
}

// New creates a new JSONL store in the given directory with the specified UUID.
func New(dir, uuid string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("jsonl store: create dir: %w", err)
	}

	s := &Store{
		dir:    dir,
		uuid:   uuid,
		stepIdx: make(map[int]*contextmgr.StepRecord),
		refIdx: make(map[int]*contextmgr.RefRecord),
		l1Idx:  make(map[int]*contextmgr.L1Record),
		l2Idx:  make(map[int]*contextmgr.L2Record),
		l3Idx:  make(map[int]*contextmgr.L3Record),
		memIdx: make(map[string]*contextmgr.LongMemRecord),
	}

	// Load existing data from files into memory
	if err := s.loadAll(); err != nil {
		return nil, fmt.Errorf("jsonl store: load: %w", err)
	}

	return s, nil
}

func (s *Store) path(ext string) string {
	return filepath.Join(s.dir, s.uuid+ext)
}

func (s *Store) loadAll() error {
	// Load L0 steps
	if err := s.loadJSONL(s.path(".jsonl"), func(data []byte) error {
		var rec contextmgr.StepRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.stepIdx[rec.StepID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load steps: %w", err)
	}

	// Load refs
	if err := s.loadJSONL(s.path(".refs.jsonl"), func(data []byte) error {
		var rec contextmgr.RefRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.refIdx[rec.StepID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load refs: %w", err)
	}

	// Load L1
	if err := s.loadJSONL(s.path(".l1.jsonl"), func(data []byte) error {
		var rec contextmgr.L1Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.l1Idx[rec.StepID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load l1: %w", err)
	}

	// Load L2
	if err := s.loadJSONL(s.path(".l2.jsonl"), func(data []byte) error {
		var rec contextmgr.L2Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.l2Idx[rec.StepID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load l2: %w", err)
	}

	// Load L3
	if err := s.loadJSONL(s.path(".l3.jsonl"), func(data []byte) error {
		var rec contextmgr.L3Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.l3Idx[rec.StepID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load l3: %w", err)
	}

	// Load long-term memory
	if err := s.loadJSONL(s.path(".longmem.jsonl"), func(data []byte) error {
		var rec contextmgr.LongMemRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		s.memIdx[rec.MemID] = &rec
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load longmem: %w", err)
	}

	return nil
}

func (s *Store) loadJSONL(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Store) appendJSONL(path string, v any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// --- L0 original content ---

// AppendStep is an idempotent upsert keyed by step_id: if the step already
// exists, the whole .jsonl file is rewritten so the on-disk store never holds
// duplicate rows for the same step_id (mirrors the SQLite/PostgreSQL upsert
// semantics). New steps take the fast append path.
func (s *Store) AppendStep(step contextmgr.StepRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.stepIdx[step.StepID]; exists {
		s.stepIdx[step.StepID] = &step
		return s.rewriteSteps()
	}

	if err := s.appendJSONL(s.path(".jsonl"), step); err != nil {
		return err
	}
	s.stepIdx[step.StepID] = &step
	return nil
}

// rewriteSteps rewrites the entire .jsonl file (temp file + rename) in
// step_id order, mirroring rewriteRefs. Caller must hold s.mu.
func (s *Store) rewriteSteps() error {
	// Collect all steps sorted by step_id
	ids := make([]int, 0, len(s.stepIdx))
	for id := range s.stepIdx {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	// Write to temp file then rename
	tmpPath := s.path(".jsonl.tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, id := range ids {
		data, err := json.Marshal(s.stepIdx[id])
		if err != nil {
			f.Close()
			return err
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path(".jsonl"))
}

func (s *Store) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.stepIdx[stepID]
	if !ok {
		return nil, fmt.Errorf("step %d not found", stepID)
	}
	return rec, nil
}

func (s *Store) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []contextmgr.StepRecord
	for id := from; id <= to; id++ {
		if rec, ok := s.stepIdx[id]; ok {
			result = append(result, *rec)
		}
	}
	return result, nil
}

// --- refs ---

func (s *Store) UpsertRef(ref contextmgr.RefRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rewrite the entire refs file on upsert (simple but correct)
	s.refIdx[ref.StepID] = &ref
	return s.rewriteRefs()
}

func (s *Store) rewriteRefs() error {
	// Collect all refs sorted by step_id
	ids := make([]int, 0, len(s.refIdx))
	for id := range s.refIdx {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	// Write to temp file then rename
	tmpPath := s.path(".refs.jsonl.tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, id := range ids {
		data, err := json.Marshal(s.refIdx[id])
		if err != nil {
			f.Close()
			return err
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path(".refs.jsonl"))
}

func (s *Store) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.refIdx[stepID]
	if !ok {
		return nil, fmt.Errorf("ref %d not found", stepID)
	}
	return rec, nil
}

func (s *Store) AllActiveStepIDs() ([]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]int, 0, len(s.refIdx))
	for id := range s.refIdx {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}

func (s *Store) RemoveRef(stepID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.refIdx, stepID)
	return s.rewriteRefs()
}

// --- L1 ---

func (s *Store) AppendL1(rec contextmgr.L1Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendJSONL(s.path(".l1.jsonl"), rec); err != nil {
		return err
	}
	s.l1Idx[rec.StepID] = &rec
	return nil
}

func (s *Store) GetL1(stepID int) (*contextmgr.L1Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.l1Idx[stepID]
	if !ok {
		return nil, fmt.Errorf("l1 record %d not found", stepID)
	}
	return rec, nil
}

// --- L2 ---

func (s *Store) AppendL2(rec contextmgr.L2Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendJSONL(s.path(".l2.jsonl"), rec); err != nil {
		return err
	}
	s.l2Idx[rec.StepID] = &rec
	return nil
}

func (s *Store) GetL2(stepID int) (*contextmgr.L2Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.l2Idx[stepID]
	if !ok {
		return nil, fmt.Errorf("l2 record %d not found", stepID)
	}
	return rec, nil
}

func (s *Store) HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []contextmgr.L2Record
	for stepID, l2 := range s.l2Idx {
		ref, ok := s.refIdx[stepID]
		if !ok {
			continue
		}
		if ref.RefCount >= minRefCount && ref.Strength >= minStrength {
			result = append(result, *l2)
		}
	}
	return result, nil
}

// --- L3 ---

func (s *Store) AppendL3(rec contextmgr.L3Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendJSONL(s.path(".l3.jsonl"), rec); err != nil {
		return err
	}
	s.l3Idx[rec.StepID] = &rec
	return nil
}

func (s *Store) GetL3(stepID int) (*contextmgr.L3Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.l3Idx[stepID]
	if !ok {
		return nil, fmt.Errorf("l3 record %d not found", stepID)
	}
	return rec, nil
}

// --- Long-term memory ---

func (s *Store) UpsertLongMem(mem contextmgr.LongMemRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.memIdx[mem.MemID] = &mem
	return s.rewriteLongMem()
}

func (s *Store) rewriteLongMem() error {
	// Collect all memories
	mems := make([]*contextmgr.LongMemRecord, 0, len(s.memIdx))
	for _, m := range s.memIdx {
		mems = append(mems, m)
	}

	// Write to temp file then rename
	tmpPath := s.path(".longmem.jsonl.tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, m := range mems {
		data, err := json.Marshal(m)
		if err != nil {
			f.Close()
			return err
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
	}

	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path(".longmem.jsonl"))
}

func (s *Store) GetLongMem(memID string) (*contextmgr.LongMemRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.memIdx[memID]
	if !ok {
		return nil, fmt.Errorf("longmem %s not found", memID)
	}
	return rec, nil
}

func (s *Store) SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	var result []contextmgr.LongMemRecord

	for _, m := range s.memIdx {
		// Filter by category if specified
		if category != "" && m.Category != category {
			continue
		}
		// Filter by confidence threshold
		if m.Confidence < 0.5 {
			continue
		}
		// Simple keyword match (BM25/VSS would be in Redis/PG backends)
		matched := false
		for _, fact := range m.Facts {
			if strings.Contains(strings.ToLower(fact), query) {
				matched = true
				break
			}
		}
		if matched {
			result = append(result, *m)
		}
	}

	// Sort by confidence descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Confidence > result[j].Confidence
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (s *Store) RemoveLongMem(memID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.memIdx, memID)
	return s.rewriteLongMem()
}

// Close releases resources.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	return nil
}

// --- audit (.audit.jsonl) ---

func (s *Store) AppendAudit(rec contextmgr.AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendJSONL(s.path(".audit.jsonl"), rec)
}
