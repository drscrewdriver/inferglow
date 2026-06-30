// Copyright 2026 InferGlow Authors

package server

import (
	"fmt"
	"strings"
	"sync"
)

// InMemoryStore is a simple in-memory MemoryStore for testing and development.
type InMemoryStore struct {
	mu      sync.RWMutex
	records map[string]MemoryRecord
}

// NewInMemoryStore creates a new in-memory memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		records: make(map[string]MemoryRecord),
	}
}

func (s *InMemoryStore) UpsertMemory(rec MemoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.ID] = rec
	return nil
}

func (s *InMemoryStore) GetMemory(id string) (*MemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	return &rec, nil
}

func (s *InMemoryStore) SearchMemory(query string, category string, limit int) ([]MemoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []MemoryRecord
	for _, rec := range s.records {
		if category != "" && rec.Category != category {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(rec.Content), strings.ToLower(query)) {
			// Also search in facts.
			found := false
			for _, f := range rec.Facts {
				if strings.Contains(strings.ToLower(f), strings.ToLower(query)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		results = append(results, rec)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *InMemoryStore) DeleteMemory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(s.records, id)
	return nil
}
