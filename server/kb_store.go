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

package server

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/inferglow/rag"
)

// KBRecord is the JSON-safe projection of a knowledge base exposed by the
// Knowledge Base API (spec C-8). The underlying vector store is never
// serialized; only metadata and the document count are surfaced.
type KBRecord struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	DocCount    int       `json:"doc_count"`
}

// hashEmbedder is a dependency-free embedding model used as the default
// embedder for the in-memory KB store. It produces a fixed-width vector from
// character n-gram hashes, so the store is fully usable with zero external
// dependencies while still satisfying the rag.EmbeddingModel contract.
type hashEmbedder struct {
	dim int
}

// NewHashEmbedder returns a hash-based embedding model with the given width.
func NewHashEmbedder(dim int) rag.EmbeddingModel {
	if dim <= 0 {
		dim = 256
	}
	return &hashEmbedder{dim: dim}
}

// Embed generates a fixed-width vector for each text via character hashing.
func (h *hashEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, h.dim)
		for gi := 0; gi+3 <= len(t); gi++ {
			// 3-gram hash.
			key := uint32(t[gi])<<16 | uint32(t[gi+1])<<8 | uint32(t[gi+2])
			key = (key * 2654435761) >> 16
			idx := int(key) % h.dim
			v[idx] += 1.0
		}
		// Normalize to unit length.
		var norm float32
		for _, x := range v {
			norm += x * x
		}
		if norm > 0 {
			inv := 1.0 / float32(math.Sqrt(float64(norm)))
			for j := range v {
				v[j] *= inv
			}
		}
		out[i] = v
	}
	return out, nil
}

// Dim returns the embedding vector width.
func (h *hashEmbedder) Dim() int { return h.dim }

// memoryVectorStore is a thin adapter implementing rag.DocumentStore plus a
// cosine similarity search. It is the "thin adapter layer" the C-8 spec
// mandates when rag.Store is absent from the repository.
type memoryVectorStore struct {
	mu   sync.RWMutex
	docs []rag.EmbeddedDocument
}

// Add implements rag.DocumentStore.
func (s *memoryVectorStore) Add(_ context.Context, docs []rag.EmbeddedDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append(s.docs, docs...)
	return nil
}

// Count returns the number of embedded documents stored.
func (s *memoryVectorStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.docs)
}

// Search returns the top-limit documents ranked by cosine similarity to query.
func (s *memoryVectorStore) Search(query []float32, limit int) []rag.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || len(s.docs) == 0 {
		return nil
	}
	type scored struct {
		doc   rag.EmbeddedDocument
		score float64
	}
	scoredDocs := make([]scored, 0, len(s.docs))
	for _, d := range s.docs {
		scoredDocs = append(scoredDocs, scored{doc: d, score: cosine(d.Vector, query)})
	}
	sort.SliceStable(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})
	if limit > len(scoredDocs) {
		limit = len(scoredDocs)
	}
	out := make([]rag.SearchResult, 0, limit)
	for _, sd := range scoredDocs[:limit] {
		out = append(out, rag.SearchResult{Document: sd.doc.Document, Score: sd.score})
	}
	return out
}

// knowledgeBase is an internal record holding a named KB and its vector store.
type knowledgeBase struct {
	name        string
	description string
	createdAt   time.Time
	store       *memoryVectorStore
}

// KBStore is the C-8 Knowledge Base store. It reuses the rag module's
// Document/EmbeddingModel contracts and a thin in-memory DocumentStore
// adapter, providing per-KB ingest and vector search.
type KBStore struct {
	mu       sync.RWMutex
	kbs      map[string]*knowledgeBase
	embedder rag.EmbeddingModel
}

// NewKBStore creates an empty KB store. When embedder is nil a default
// hash-based embedder is used so the store is usable with zero configuration.
func NewKBStore(embedder rag.EmbeddingModel) *KBStore {
	if embedder == nil {
		embedder = NewHashEmbedder(256)
	}
	return &KBStore{
		kbs:      make(map[string]*knowledgeBase),
		embedder: embedder,
	}
}

// Create registers a new empty knowledge base.
func (ks *KBStore) Create(name, description string) error {
	if name == "" {
		return fmt.Errorf("knowledge base name is required")
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if _, ok := ks.kbs[name]; ok {
		return fmt.Errorf("knowledge base %q already exists", name)
	}
	ks.kbs[name] = &knowledgeBase{
		name:        name,
		description: description,
		createdAt:   time.Now(),
		store:       &memoryVectorStore{},
	}
	return nil
}

// List returns metadata for all knowledge bases, sorted by name.
func (ks *KBStore) List() []KBRecord {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	out := make([]KBRecord, 0, len(ks.kbs))
	for _, kb := range ks.kbs {
		out = append(out, KBRecord{
			Name:        kb.name,
			Description: kb.description,
			CreatedAt:   kb.createdAt,
			DocCount:    kb.store.Count(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns metadata for a single knowledge base.
func (ks *KBStore) Get(name string) (KBRecord, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	kb, ok := ks.kbs[name]
	if !ok {
		return KBRecord{}, fmt.Errorf("knowledge base %q not found", name)
	}
	return KBRecord{
		Name:        kb.name,
		Description: kb.description,
		CreatedAt:   kb.createdAt,
		DocCount:    kb.store.Count(),
	}, nil
}

// Delete removes a knowledge base and all its documents.
func (ks *KBStore) Delete(name string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if _, ok := ks.kbs[name]; !ok {
		return fmt.Errorf("knowledge base %q not found", name)
	}
	delete(ks.kbs, name)
	return nil
}

// Ingest embeds and stores the given documents into a knowledge base,
// returning the number of documents stored.
func (ks *KBStore) Ingest(ctx context.Context, name string, docs []rag.Document) (int, error) {
	ks.mu.RLock()
	kb, ok := ks.kbs[name]
	ks.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("knowledge base %q not found", name)
	}
	if len(docs) == 0 {
		return 0, nil
	}
	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}
	vectors, err := ks.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed docs: %w", err)
	}
	embedded := make([]rag.EmbeddedDocument, len(docs))
	for i := range docs {
		embedded[i] = rag.EmbeddedDocument{Document: docs[i], Vector: vectors[i]}
	}
	if err := kb.store.Add(ctx, embedded); err != nil {
		return 0, fmt.Errorf("store docs: %w", err)
	}
	return len(docs), nil
}

// Search runs a vector search within a knowledge base for the given query.
func (ks *KBStore) Search(ctx context.Context, name, query string, limit int) ([]rag.SearchResult, error) {
	ks.mu.RLock()
	kb, ok := ks.kbs[name]
	ks.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("knowledge base %q not found", name)
	}
	vectors, err := ks.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	return kb.store.Search(vectors[0], limit), nil
}

// cosine returns the cosine similarity between two vectors.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}