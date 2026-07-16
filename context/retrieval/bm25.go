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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package retrieval

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"sort"
	"sync"
)

// BM25Index is an in-memory BM25 index with CJK bigram support, inverted
// index for sub-linear search, and optional disk persistence.
type BM25Index struct {
	mu       sync.RWMutex
	docs     map[int]string
	docCount int
	k1       float64
	b        float64

	// Inverted index: term → {docID → termFreq}.
	inverted map[string]map[int]int
	// Cached document lengths: docID → token count.
	docLens map[int]int
	// Cached average document length.
	avgDL float64
}

// NewBM25Index creates a new BM25 index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs:     make(map[int]string),
		k1:       1.5,
		b:        0.75,
		inverted: make(map[string]map[int]int),
		docLens:  make(map[int]int),
	}
}

// Add adds a document to the index, updating the inverted index and caches.
func (idx *BM25Index) Add(stepID int, text string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.docs[stepID] = text
	idx.docCount++

	// Tokenize and update inverted index.
	tokens := Tokenize(text)
	idx.docLens[stepID] = len(tokens)

	// Update average doc length.
	totalLen := 0
	for _, dl := range idx.docLens {
		totalLen += dl
	}
	idx.avgDL = float64(totalLen) / float64(idx.docCount)
	if idx.avgDL == 0 {
		idx.avgDL = 1
	}

	// Count term frequencies for this document.
	tf := make(map[string]int)
	for _, t := range tokens {
		tf[t]++
	}

	// Update inverted index.
	for term, freq := range tf {
		if idx.inverted[term] == nil {
			idx.inverted[term] = make(map[int]int)
		}
		idx.inverted[term][stepID] = freq
	}
}

// Search performs BM25 keyword search using the inverted index for
// sub-linear retrieval.
func (idx *BM25Index) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil, nil
	}

	queryTerms := Tokenize(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	// Calculate IDF for each query term from the inverted index (O(1) per term).
	idf := make(map[string]float64)
	for _, term := range queryTerms {
		df := len(idx.inverted[term])
		if df == 0 {
			continue
		}
		idf[term] = math.Log(1 + (float64(idx.docCount-df)+0.5)/(float64(df)+0.5))
	}

	// Build candidate set: union of all docs containing at least one query term.
	candidates := make(map[int]bool)
	for _, term := range queryTerms {
		if idf[term] == 0 {
			continue
		}
		for docID := range idx.inverted[term] {
			candidates[docID] = true
		}
	}

	// Calculate BM25 score only for candidate documents.
	var results []SearchResult
	for docID := range candidates {
		docLen := float64(idx.docLens[docID])
		score := 0.0
		for _, term := range queryTerms {
			if idf[term] == 0 {
				continue
			}
			f := float64(idx.inverted[term][docID])
			if f == 0 {
				continue
			}
			numerator := f * (idx.k1 + 1)
			denominator := f + idx.k1*(1-idx.b+idx.b*docLen/idx.avgDL)
			score += idf[term] * numerator / denominator
		}
		if score > 0 {
			snippet := idx.docs[docID]
			if len([]rune(snippet)) > 200 {
				snippet = string([]rune(snippet)[:200]) + "..."
			}
			results = append(results, SearchResult{
				StepID: docID,
				Score:  score,
				Text:   snippet,
			})
		}
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// Persistence — serialize/deserialize the docs map; inverted index is rebuilt.
// ---------------------------------------------------------------------------

type bm25Snapshot struct {
	Docs map[int]string `json:"docs"`
}

// Save serializes the document map to the given writer as JSON.
// The inverted index is not persisted — it is rebuilt on Load.
func (idx *BM25Index) Save(w io.Writer) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	snap := bm25Snapshot{Docs: idx.docs}
	enc := json.NewEncoder(w)
	return enc.Encode(snap)
}

// Load restores documents from the given reader and rebuilds the inverted
// index. On failure the index is left unchanged (best-effort).
func (idx *BM25Index) Load(r io.Reader) error {
	var snap bm25Snapshot
	if err := json.NewDecoder(r).Decode(&snap); err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Reset and rebuild.
	idx.docs = snap.Docs
	idx.docCount = len(snap.Docs)
	idx.inverted = make(map[string]map[int]int)
	idx.docLens = make(map[int]int)

	for docID, text := range idx.docs {
		tokens := Tokenize(text)
		idx.docLens[docID] = len(tokens)

		tf := make(map[string]int)
		for _, t := range tokens {
			tf[t]++
		}
		for term, freq := range tf {
			if idx.inverted[term] == nil {
				idx.inverted[term] = make(map[int]int)
			}
			idx.inverted[term][docID] = freq
		}
	}

	// Recompute avgDL.
	totalLen := 0
	for _, dl := range idx.docLens {
		totalLen += dl
	}
	if idx.docCount > 0 {
		idx.avgDL = float64(totalLen) / float64(idx.docCount)
	}
	if idx.avgDL == 0 {
		idx.avgDL = 1
	}

	return nil
}
