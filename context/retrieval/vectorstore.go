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

package retrieval

import (
	"context"
	"sort"
)

// VectorMeta holds metadata associated with a stored vector.
type VectorMeta struct {
	SessionID string `json:"session_id,omitempty"`
	StepID    int    `json:"step_id,omitempty"`
	Text      string `json:"text,omitempty"`
}

// VectorItem is a batch-insert item combining ID, vector, and metadata.
type VectorItem struct {
	ID   string
	Vec  []float32
	Meta VectorMeta
}

// VectorStoreBackend is the abstraction for vector storage engines.
// Implementations include in-memory (default), pgvector, and Redis VSS.
type VectorStoreBackend interface {
	// Add inserts a single vector with its metadata.
	Add(ctx context.Context, id string, vec []float32, meta VectorMeta) error

	// Search finds the top-k most similar vectors to the query vector.
	Search(ctx context.Context, query []float32, limit int) ([]SearchResult, error)

	// Delete removes a vector by ID.
	Delete(ctx context.Context, id string) error

	// BatchAdd inserts multiple vectors in a single operation.
	BatchAdd(ctx context.Context, items []VectorItem) error
}

// InMemoryVectorStore is a simple in-memory implementation of VectorStoreBackend.
// It uses brute-force cosine similarity search.
type InMemoryVectorStore struct {
	items map[string]storedVector
}

type storedVector struct {
	vec  []float32
	meta VectorMeta
}

// NewInMemoryVectorStore creates a new empty in-memory vector store.
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{items: make(map[string]storedVector)}
}

// Add inserts a single vector.
func (s *InMemoryVectorStore) Add(_ context.Context, id string, vec []float32, meta VectorMeta) error {
	s.items[id] = storedVector{vec: vec, meta: meta}
	return nil
}

// Search performs brute-force cosine similarity search.
func (s *InMemoryVectorStore) Search(_ context.Context, query []float32, limit int) ([]SearchResult, error) {
	var results []SearchResult
	for id, sv := range s.items {
		score := cosineSimilarity(query, sv.vec)
		if score > 0 {
			results = append(results, SearchResult{
				StepID: sv.meta.StepID,
				Score:  score,
				Text:   sv.meta.Text,
			})
		}
		_ = id
	}

	// Sort by score descending using sort.Slice (O(n log n)).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Delete removes a vector by ID.
func (s *InMemoryVectorStore) Delete(_ context.Context, id string) error {
	delete(s.items, id)
	return nil
}

// BatchAdd inserts multiple vectors.
func (s *InMemoryVectorStore) BatchAdd(ctx context.Context, items []VectorItem) error {
	for _, item := range items {
		if err := s.Add(ctx, item.ID, item.Vec, item.Meta); err != nil {
			return err
		}
	}
	return nil
}
