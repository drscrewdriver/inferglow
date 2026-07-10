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
	"math"
	"sort"
	"sync"
)

// VectorStore is a simple in-memory vector store for semantic search.
// For production use, replace with Redis VSS, pgvector, or a dedicated vector DB.
type VectorStore struct {
	mu      sync.RWMutex
	vectors map[int][]float32
	texts   map[int]string
}

// NewVectorStore creates a new in-memory vector store.
func NewVectorStore() *VectorStore {
	return &VectorStore{
		vectors: make(map[int][]float32),
		texts:   make(map[int]string),
	}
}

// Add adds a document vector to the store.
func (v *VectorStore) Add(stepID int, vec []float32, text string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.vectors[stepID] = vec
	v.texts[stepID] = text
}

// Search performs cosine similarity search.
func (v *VectorStore) Search(ctx context.Context, query []float32, limit int) ([]SearchResult, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(query) == 0 {
		return nil, nil
	}

	var results []SearchResult
	for stepID, vec := range v.vectors {
		score := cosineSimilarity(query, vec)
		if score > 0 {
			results = append(results, SearchResult{
				StepID: stepID,
				Score:  score,
				Text:   v.texts[stepID],
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
