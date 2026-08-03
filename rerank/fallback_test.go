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

package rerank

import (
	"context"
	"fmt"
	"testing"
)

type failingReranker struct{}

func (f *failingReranker) Rerank(_ context.Context, _ string, _ []Document) ([]RankedDocument, error) {
	return nil, fmt.Errorf("reranker failed")
}

type successReranker struct{}

func (s *successReranker) Rerank(_ context.Context, _ string, docs []Document) ([]RankedDocument, error) {
	result := make([]RankedDocument, len(docs))
	for i, doc := range docs {
		result[i] = RankedDocument{Document: doc, Score: float64(len(docs) - i), Index: i}
	}
	return result, nil
}

func TestFallbackOnSuccess(t *testing.T) {
	fb := WithFallback(&successReranker{})
	docs := []Document{{Content: "A"}, {Content: "B"}}

	ranked, err := fb.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked docs, got %d", len(ranked))
	}
	if ranked[0].Score != 2.0 {
		t.Errorf("expected score 2.0, got %f", ranked[0].Score)
	}
}

func TestFallbackOnFailure(t *testing.T) {
	fb := WithFallback(&failingReranker{})
	docs := []Document{{Content: "A"}, {Content: "B"}, {Content: "C"}}

	ranked, err := fb.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("fallback should not return error: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked docs, got %d", len(ranked))
	}
	// Should be in original order with zero scores
	for i, r := range ranked {
		if r.Index != i {
			t.Errorf("expected index %d, got %d", i, r.Index)
		}
		if r.Score != 0 {
			t.Errorf("expected score 0, got %f", r.Score)
		}
		if r.Content != docs[i].Content {
			t.Errorf("expected content %q, got %q", docs[i].Content, r.Content)
		}
	}
}

func TestNewRerankerFromConfig(t *testing.T) {
	// Test cohere config
	cfg := Config{
		Provider:     "cohere",
		CohereAPIKey: "test-key",
		CohereModel:  "rerank-multilingual-v3.0",
		TopN:         5,
	}
	r, err := NewRerankerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cr, ok := r.(*CohereReranker)
	if !ok {
		t.Fatal("expected *CohereReranker")
	}
	if cr.Model != "rerank-multilingual-v3.0" {
		t.Errorf("expected model 'rerank-multilingual-v3.0', got %q", cr.Model)
	}
	if cr.TopN != 5 {
		t.Errorf("expected TopN 5, got %d", cr.TopN)
	}
}

func TestNewRerankerFromConfigMissingKey(t *testing.T) {
	cfg := Config{Provider: "cohere"}
	_, err := NewRerankerFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewRerankerFromConfigUnknown(t *testing.T) {
	cfg := Config{Provider: "unknown"}
	_, err := NewRerankerFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
