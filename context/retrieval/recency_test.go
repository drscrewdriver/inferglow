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
	"testing"
)

// nearlyEqual reports whether a and b are within 1e-9 of each other.
func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestRecencyIndexOrdering verifies that, with equal strength, a more recently
// referenced step ranks higher (recency term dominates the ordering).
func TestRecencyIndexOrdering(t *testing.T) {
	r := NewRecencyIndex()
	r.Add(1, 1, 1.0, "old step")
	r.Add(2, 5, 1.0, "newest step")
	r.Add(3, 3, 1.0, "mid step")

	// maxStep=5, maxStrength=1.0
	// step1: 0.6*(1/5)+0.4*1.0 = 0.52
	// step2: 0.6*(5/5)+0.4*1.0 = 1.00
	// step3: 0.6*(3/5)+0.4*1.0 = 0.76
	got, err := r.Search(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}
	if got[0].StepID != 2 {
		t.Errorf("top result = step %d, want step 2 (most recent)", got[0].StepID)
	}
	if got[1].StepID != 3 || got[2].StepID != 1 {
		t.Errorf("ordering = [%d,%d,%d], want [2,3,1]", got[0].StepID, got[1].StepID, got[2].StepID)
	}
	if !nearlyEqual(got[0].Score, 1.0) {
		t.Errorf("top score = %v, want 1.0", got[0].Score)
	}
}

// TestRecencyIndexStrengthWeight verifies that, with equal recency, a higher
// strength ranks higher (strength term 0.4 takes effect).
func TestRecencyIndexStrengthWeight(t *testing.T) {
	r := NewRecencyIndex()
	r.Add(1, 5, 1.0, "weak")
	r.Add(2, 5, 3.0, "strong")

	// maxStep=5, maxStrength=3.0
	// step1: 0.6*1.0 + 0.4*(1/3) = 0.7333...
	// step2: 0.6*1.0 + 0.4*1.0   = 1.0
	got, _ := r.Search(context.Background(), 10)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].StepID != 2 {
		t.Errorf("top result = step %d, want step 2 (higher strength)", got[0].StepID)
	}
}

// TestRecencyIndexEmpty verifies an empty index yields nil (neutral for fusion).
func TestRecencyIndexEmpty(t *testing.T) {
	r := NewRecencyIndex()
	got, err := r.Search(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("empty index got %v, want nil", got)
	}
}

// TestRecencyIndexLimit verifies results are truncated to limit.
func TestRecencyIndexLimit(t *testing.T) {
	r := NewRecencyIndex()
	for i := 1; i <= 5; i++ {
		r.Add(i, i, 1.0, "step")
	}
	got, _ := r.Search(context.Background(), 2)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2 (limit)", len(got))
	}
}

// TestRecencyIndexNormalization verifies all scores fall within (0, 1].
func TestRecencyIndexNormalization(t *testing.T) {
	r := NewRecencyIndex()
	r.Add(1, 2, 0.5, "a")
	r.Add(2, 7, 2.0, "b")
	r.Add(3, 4, 1.0, "c")
	got, _ := r.Search(context.Background(), 10)
	for _, res := range got {
		if res.Score <= 0 || res.Score > 1.0+1e-9 {
			t.Errorf("step %d score %v out of (0,1]", res.StepID, res.Score)
		}
	}
}

// TestRecencyIndexWithWeights verifies explicit weights take effect and that
// non-positive weights fall back to defaults.
func TestRecencyIndexWithWeights(t *testing.T) {
	// Pure recency (strength weight 0): strength must NOT influence ranking.
	r := NewRecencyIndexWithWeights(1.0, 0.0)
	r.Add(1, 1, 99.0, "old but strong")
	r.Add(2, 5, 1.0, "new but weak")
	got, _ := r.Search(context.Background(), 10)
	if got[0].StepID != 2 {
		t.Errorf("pure-recency top = step %d, want step 2 (newest)", got[0].StepID)
	}

	// Non-positive weights fall back to defaults (0.6/0.4) — should not panic
	// and should behave like NewRecencyIndex.
	fb := NewRecencyIndexWithWeights(0, 0)
	if fb.recencyW != 0.6 || fb.strengthW != 0.4 {
		t.Errorf("fallback weights = (%v,%v), want (0.6,0.4)", fb.recencyW, fb.strengthW)
	}
}

// TestRecencyIndexReset verifies Reset clears entries (idempotent re-index).
func TestRecencyIndexReset(t *testing.T) {
	r := NewRecencyIndex()
	r.Add(1, 5, 2.0, "x")
	r.Reset()
	got, _ := r.Search(context.Background(), 5)
	if got != nil {
		t.Errorf("after Reset got %v, want nil", got)
	}
}
