// Copyright 2026 InferGlow Authors

package server

import (
	"fmt"
	"strings"

	"github.com/inferglow/storage"
)

// InMemoryStore is a simple in-memory MemoryStore for testing and development.
// The backing KV storage is provided by the generic storage.Map primitive.
type InMemoryStore struct {
	*storage.Map[string, MemoryRecord]
}

// NewInMemoryStore creates a new in-memory memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{Map: storage.NewMap[string, MemoryRecord]()}
}

func (s *InMemoryStore) UpsertMemory(rec MemoryRecord) error {
	s.Map.Set(rec.ID, rec)
	return nil
}

func (s *InMemoryStore) GetMemory(id string) (*MemoryRecord, error) {
	rec, ok := s.Map.Get(id)
	if !ok {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	return &rec, nil // return a copy, preserving existing semantics
}

func (s *InMemoryStore) SearchMemory(query string, category string, limit int) ([]MemoryRecord, error) {
	var results []MemoryRecord
	for _, rec := range s.Map.Values() {
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

// Compile-time assertion that InMemoryStore still satisfies the MemoryStore
// interface injected by the server.
var _ MemoryStore = (*InMemoryStore)(nil)

func (s *InMemoryStore) DeleteMemory(id string) error {
	if _, ok := s.Map.Get(id); !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	s.Map.Delete(id)
	return nil
}
