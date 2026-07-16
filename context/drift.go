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

package contextmgr

import (
	"sort"
	"sync/atomic"

	"github.com/inferglow/context/retrieval"
)

// DriftDetector performs lightweight semantic drift detection by comparing
// token overlap between recent step content and Zone 1 (head buffer) keywords.
// It uses retrieval.Tokenize (CJK bigram + Latin) for tokenization.
type DriftDetector struct {
	interval   int32   // check every M steps (default 5)
	threshold  float64 // overlap ratio below this = drift (default 0.15)
	lastCheck  int32   // last step ID where check was performed
	hintActive atomic.Bool
}

// NewDriftDetector creates a drift detector with the given interval and threshold.
// If interval <= 0, defaults to 5. If threshold <= 0, defaults to 0.15.
func NewDriftDetector(interval int, threshold float64) *DriftDetector {
	if interval <= 0 {
		interval = 5
	}
	if threshold <= 0 {
		threshold = 0.15
	}
	return &DriftDetector{
		interval:  int32(interval),
		threshold: threshold,
	}
}

// MaybeCheck performs a drift check if enough steps have elapsed since the
// last check. Returns true if drift is detected (overlap below threshold).
func (d *DriftDetector) MaybeCheck(stepID int32, stepContent string, headKeywords []string) bool {
	if d.interval <= 0 {
		return false
	}
	// Only check every M steps.
	if stepID-d.lastCheck < d.interval {
		return false
	}
	d.lastCheck = stepID

	if len(headKeywords) == 0 || stepContent == "" {
		return false
	}

	overlap := tokenOverlap(stepContent, headKeywords)
	return overlap < d.threshold
}

// HintActive returns whether a drift hint should be displayed.
func (d *DriftDetector) HintActive() bool {
	return d.hintActive.Load()
}

// ClearHint resets the drift hint flag (called after displaying).
func (d *DriftDetector) ClearHint() {
	d.hintActive.Store(false)
}

// SetHint activates the drift hint.
func (d *DriftDetector) SetHint() {
	d.hintActive.Store(true)
}

// tokenOverlap computes the Jaccard-like overlap ratio between the tokens
// extracted from content and the provided head keywords.
func tokenOverlap(content string, headKeywords []string) float64 {
	contentTokens := retrieval.Tokenize(content)
	if len(contentTokens) == 0 {
		return 0
	}

	// Build set from head keywords.
	headSet := make(map[string]struct{}, len(headKeywords))
	for _, kw := range headKeywords {
		headSet[kw] = struct{}{}
	}
	if len(headSet) == 0 {
		return 0
	}

	// Count how many content tokens appear in head keywords.
	seen := make(map[string]struct{}, len(contentTokens))
	intersection := 0
	for _, tok := range contentTokens {
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		if _, ok := headSet[tok]; ok {
			intersection++
		}
	}

	// Jaccard: |intersection| / |union|
	union := len(seen) + len(headSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// ExtractKeywords extracts the top-N most frequent tokens from content.
// Uses retrieval.Tokenize for CJK-aware tokenization.
func ExtractKeywords(content string, n int) []string {
	tokens := retrieval.Tokenize(content)
	if len(tokens) == 0 {
		return nil
	}

	freq := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		freq[tok]++
	}

	type kv struct {
		key string
		val int
	}
	pairs := make([]kv, 0, len(freq))
	for k, v := range freq {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].val > pairs[j].val
	})

	if n > len(pairs) {
		n = len(pairs)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = pairs[i].key
	}
	return out
}
