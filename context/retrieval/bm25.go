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
	"strings"
	"sync"
)

// BM25Index is an in-memory BM25 index for keyword search.
type BM25Index struct {
	mu       sync.RWMutex
	docs     map[int]string
	docCount int
	k1       float64
	b        float64
}

// NewBM25Index creates a new BM25 index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs: make(map[int]string),
		k1:   1.5,
		b:    0.75,
	}
}

// Add adds a document to the index.
func (idx *BM25Index) Add(stepID int, text string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.docs[stepID] = text
	idx.docCount++
}

// Search performs BM25 keyword search.
func (idx *BM25Index) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.docCount == 0 {
		return nil, nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	// Calculate average document length
	totalLen := 0
	for _, doc := range idx.docs {
		totalLen += len(tokenize(doc))
	}
	avgDL := float64(totalLen) / float64(idx.docCount)
	if avgDL == 0 {
		avgDL = 1
	}

	// Calculate IDF for each query term
	idf := make(map[string]float64)
	for _, term := range queryTerms {
		df := 0
		for _, doc := range idx.docs {
			if strings.Contains(strings.ToLower(doc), term) {
				df++
			}
		}
		idf[term] = math.Log(1 + (float64(idx.docCount-df)+0.5)/(float64(df)+0.5))
	}

	// Calculate BM25 score for each document
	var results []SearchResult
	for stepID, doc := range idx.docs {
		docTerms := tokenize(doc)
		docLen := float64(len(docTerms))

		// Count term frequencies
		tf := make(map[string]int)
		for _, t := range docTerms {
			tf[t]++
		}

		score := 0.0
		for _, term := range queryTerms {
			f := float64(tf[term])
			if f == 0 {
				continue
			}
			numerator := f * (idx.k1 + 1)
			denominator := f + idx.k1*(1-idx.b+idx.b*docLen/avgDL)
			score += idf[term] * numerator / denominator
		}

		if score > 0 {
			snippet := doc
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			results = append(results, SearchResult{
				StepID: stepID,
				Score:  score,
				Text:   snippet,
			})
		}
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// tokenize splits text into lowercase terms.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	var tokens []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()[]{}\"'`")
		if len(w) > 1 {
			tokens = append(tokens, w)
		}
	}
	return tokens
}
