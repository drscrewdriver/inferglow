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
	"sync"
)

// RecencyIndex is an in-memory recency-weighted index implementing
// RecencySearcher (§7.3 path 3 / A-7). It ranks each step by a weighted
// combination of how recently it was referenced (LastRefAtStep) and its
// accumulated access Strength, both normalised to [0,1] so the score fuses
// cleanly with the semantic/keyword paths in FusionRetriever.
type RecencyIndex struct {
	mu          sync.RWMutex
	entries     map[int]recencyEntry
	maxStep     int     // highest lastRefAtStep observed (acts as "now")
	maxStrength float64 // highest strength observed (normalisation denominator)
	recencyW    float64 // weight of the recency term (default 0.6)
	strengthW   float64 // weight of the strength term (default 0.4)
}

// recencyEntry is the per-step recency signal.
type recencyEntry struct {
	lastRefAtStep int
	strength      float64
	text          string
}

// NewRecencyIndex creates a recency index with default weights (0.6/0.4).
func NewRecencyIndex() *RecencyIndex {
	return &RecencyIndex{
		entries:   make(map[int]recencyEntry),
		recencyW:  0.6,
		strengthW: 0.4,
	}
}

// NewRecencyIndexWithWeights creates a recency index with explicit weights.
// Non-positive weights fall back to the defaults (0.6/0.4).
func NewRecencyIndexWithWeights(recencyW, strengthW float64) *RecencyIndex {
	r := NewRecencyIndex()
	if recencyW > 0 || strengthW > 0 {
		r.recencyW = recencyW
		r.strengthW = strengthW
	}
	return r
}

// Reset clears all entries and normalisation state. Used to make a full
// re-index idempotent.
func (r *RecencyIndex) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[int]recencyEntry)
	r.maxStep = 0
	r.maxStrength = 0
}

// Add records (or updates) a step's recency signals and refreshes the
// normalisation denominators.
func (r *RecencyIndex) Add(stepID, lastRefAtStep int, strength float64, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries[stepID] = recencyEntry{
		lastRefAtStep: lastRefAtStep,
		strength:      strength,
		text:          text,
	}
	if lastRefAtStep > r.maxStep {
		r.maxStep = lastRefAtStep
	}
	if strength > r.maxStrength {
		r.maxStrength = strength
	}
}

// Search returns the top-limit steps ranked by recency-weighted score:
//
//	score = recencyW × (lastRefAtStep / maxStep) + strengthW × (strength / maxStrength)
//
// An empty index yields nil (consistent with VectorStore/BM25Index), so the
// fusion path treats it as a neutral contributor.
func (r *RecencyIndex) Search(ctx context.Context, limit int) ([]SearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.entries) == 0 {
		return nil, nil
	}

	var results []SearchResult
	for stepID, e := range r.entries {
		recencyNorm := 0.0
		if r.maxStep > 0 {
			recencyNorm = float64(e.lastRefAtStep) / float64(r.maxStep)
		}
		strengthNorm := 0.0
		if r.maxStrength > 0 {
			strengthNorm = e.strength / r.maxStrength
		}
		score := r.recencyW*recencyNorm + r.strengthW*strengthNorm
		if score <= 0 {
			continue
		}

		snippet := e.text
		if len([]rune(snippet)) > 200 {
			snippet = string([]rune(snippet)[:200]) + "..."
		}
		results = append(results, SearchResult{
			StepID: stepID,
			Score:  score,
			Text:   snippet,
		})
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
