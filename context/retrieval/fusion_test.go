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
	"testing"
)

// --- mock searchers (fixed results, no external dependencies) ---

type mockSemantic struct{ results []SearchResult }

func (m *mockSemantic) Search(ctx context.Context, query []float32, limit int) ([]SearchResult, error) {
	return m.results, nil
}

type mockKeyword struct{ results []SearchResult }

func (m *mockKeyword) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	return m.results, nil
}

type mockRecency struct{ results []SearchResult }

func (m *mockRecency) Search(ctx context.Context, limit int) ([]SearchResult, error) {
	return m.results, nil
}

// mockEmbedder returns a non-nil vector so the semantic path is exercised.
type mockEmbedder struct{}

func (mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0, 0.0}, nil
}
func (mockEmbedder) Dim() int { return 2 }

// TestFusionThreeWayVsTwoWay is the §4 hand-computed acceptance: recency lifts a
// weak keyword/semantic match (B) back above the threshold, proving the 0.20
// recency weight takes effect and that two-way → three-way is measurably different.
//
//	semantic {A:1.0, B:0.2}, keyword {A:1.0, B:0.2}, recency {B:1.0}, threshold 0.2
//	two-way:   A=0.5+0.3=0.80, B=0.1+0.06=0.16 (filtered)  -> [A]
//	three-way: B=0.16 + 1.0*0.20 = 0.36 (rescued)          -> [A(0.80), B(0.36)]
func TestFusionThreeWayVsTwoWay(t *testing.T) {
	sem := &mockSemantic{results: []SearchResult{{StepID: 1, Score: 1.0}, {StepID: 2, Score: 0.2}}}
	kw := &mockKeyword{results: []SearchResult{{StepID: 1, Score: 1.0}, {StepID: 2, Score: 0.2}}}
	rec := &mockRecency{results: []SearchResult{{StepID: 2, Score: 1.0}}}

	// Two-way (recency = nil).
	twoWay := NewFusionRetriever(sem, kw, nil, mockEmbedder{})
	twoWay.Threshold = 0.2
	gotTwo, err := twoWay.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatalf("two-way error: %v", err)
	}
	if len(gotTwo) != 1 || gotTwo[0].StepID != 1 {
		t.Fatalf("two-way = %+v, want only A(step1)", gotTwo)
	}

	// Three-way (recency attached).
	threeWay := NewFusionRetriever(sem, kw, rec, mockEmbedder{})
	threeWay.Threshold = 0.2
	gotThree, err := threeWay.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatalf("three-way error: %v", err)
	}
	if len(gotThree) != 2 {
		t.Fatalf("three-way got %d results %+v, want 2 (B rescued)", len(gotThree), gotThree)
	}
	if gotThree[0].StepID != 1 || gotThree[1].StepID != 2 {
		t.Errorf("three-way ordering = [%d,%d], want [1,2]", gotThree[0].StepID, gotThree[1].StepID)
	}
	// B's fused score must be 0.36 (0.16 base + 0.20 recency contribution).
	if !nearlyEqual(gotThree[1].Score, 0.36) {
		t.Errorf("B score = %v, want 0.36", gotThree[1].Score)
	}
	// A's score is unchanged by recency (A absent from recency path).
	if !nearlyEqual(gotThree[0].Score, 0.80) {
		t.Errorf("A score = %v, want 0.80", gotThree[0].Score)
	}
}

// TestFusionRecencyWeight isolates the recency contribution: with a doc present
// in all three paths at full normalised score, attaching recency raises its fused
// score by exactly Weights[2] (0.20).
func TestFusionRecencyWeight(t *testing.T) {
	sem := &mockSemantic{results: []SearchResult{{StepID: 1, Score: 1.0}}}
	kw := &mockKeyword{results: []SearchResult{{StepID: 1, Score: 1.0}}}
	rec := &mockRecency{results: []SearchResult{{StepID: 1, Score: 1.0}}}

	without := NewFusionRetriever(sem, kw, nil, mockEmbedder{})
	without.Threshold = 0.0
	gotWithout, _ := without.Search(context.Background(), "q", 10)
	if len(gotWithout) != 1 {
		t.Fatalf("without recency got %+v, want 1 result", gotWithout)
	}
	base := gotWithout[0].Score // 0.5 + 0.3 = 0.8

	with := NewFusionRetriever(sem, kw, rec, mockEmbedder{})
	with.Threshold = 0.0
	gotWith, _ := with.Search(context.Background(), "q", 10)
	if len(gotWith) != 1 {
		t.Fatalf("with recency got %+v, want 1 result", gotWith)
	}

	delta := gotWith[0].Score - base
	if !nearlyEqual(delta, 0.20) {
		t.Errorf("recency delta = %v, want 0.20 (Weights[2] × normRecency 1.0)", delta)
	}
}

// TestFusionThresholdFilter verifies results below the threshold are dropped.
func TestFusionThresholdFilter(t *testing.T) {
	// A only reaches semantic (0.5 fused) — below a 0.9 threshold.
	sem := &mockSemantic{results: []SearchResult{{StepID: 1, Score: 1.0}}}
	kw := &mockKeyword{results: nil}
	f := NewFusionRetriever(sem, kw, nil, mockEmbedder{})
	f.Threshold = 0.9
	got, _ := f.Search(context.Background(), "q", 10)
	if len(got) != 0 {
		t.Errorf("got %+v, want empty (0.5 < 0.9 threshold)", got)
	}
}

// TestFusionNilRecencyCompat verifies a nil recency searcher degrades gracefully
// to two-way fusion without panicking (compatibility with existing callers such
// as cli/memory_bridge.go that pass recency=nil).
func TestFusionNilRecencyCompat(t *testing.T) {
	sem := &mockSemantic{results: []SearchResult{{StepID: 1, Score: 1.0}}}
	kw := &mockKeyword{results: []SearchResult{{StepID: 1, Score: 1.0}}}
	f := NewFusionRetriever(sem, kw, nil, mockEmbedder{})
	f.Threshold = 0.2
	got, err := f.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatalf("nil recency should not error: %v", err)
	}
	if len(got) != 1 || got[0].StepID != 1 {
		t.Errorf("got %+v, want [step1]", got)
	}
}
