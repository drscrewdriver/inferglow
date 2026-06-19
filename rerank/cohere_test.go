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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCohereReranker(t *testing.T) {
	// Mock Cohere API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header 'Bearer test-key', got %q", r.Header.Get("Authorization"))
		}

		var req cohereRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		resp := cohereResponse{
			Results: []cohereResult{
				{Index: 2, RelevanceScore: 0.95},
				{Index: 0, RelevanceScore: 0.75},
				{Index: 1, RelevanceScore: 0.30},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	reranker := &CohereReranker{
		APIKey:  "test-key",
		Model:   "rerank-english-v3.0",
		BaseURL: server.URL,
	}

	docs := []Document{
		{Content: "Go programming language"},
		{Content: "Python for data science"},
		{Content: "Go concurrency patterns"},
	}

	ranked, err := reranker.Rerank(context.Background(), "Go programming", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked docs, got %d", len(ranked))
	}
	if ranked[0].Score != 0.95 {
		t.Errorf("expected top score 0.95, got %f", ranked[0].Score)
	}
	if ranked[0].Index != 2 {
		t.Errorf("expected top doc index 2, got %d", ranked[0].Index)
	}
}

func TestCohereRerankerEmpty(t *testing.T) {
	reranker := NewCohereReranker("test-key")
	ranked, err := reranker.Rerank(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ranked != nil {
		t.Fatalf("expected nil for empty docs, got %d", len(ranked))
	}
}

func TestCohereRerankerAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	reranker := &CohereReranker{
		APIKey:  "bad-key",
		BaseURL: server.URL,
	}

	_, err := reranker.Rerank(context.Background(), "query", []Document{{Content: "test"}})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
