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

type mockLLMCaller struct {
	response string
	err      error
}

func (m *mockLLMCaller) Generate(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func TestLLMReranker(t *testing.T) {
	caller := &mockLLMCaller{
		response: "Document 0: 8\nDocument 1: 3\nDocument 2: 9\n",
	}

	reranker := NewLLMReranker(caller)
	docs := []Document{
		{Content: "Go programming"},
		{Content: "Cooking recipes"},
		{Content: "Go concurrency"},
	}

	ranked, err := reranker.Rerank(context.Background(), "Go programming", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked docs, got %d", len(ranked))
	}
	// Document 2 should be first (score 9)
	if ranked[0].Index != 2 {
		t.Errorf("expected top doc index 2, got %d", ranked[0].Index)
	}
	if ranked[0].Score != 9.0 {
		t.Errorf("expected top score 9.0, got %f", ranked[0].Score)
	}
}

func TestLLMRerankerTopN(t *testing.T) {
	caller := &mockLLMCaller{
		response: "Document 0: 8\nDocument 1: 3\nDocument 2: 9\n",
	}

	reranker := &LLMReranker{Caller: caller, TopN: 2}
	docs := []Document{
		{Content: "A"},
		{Content: "B"},
		{Content: "C"},
	}

	ranked, err := reranker.Rerank(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked docs (TopN=2), got %d", len(ranked))
	}
}

func TestLLMRerankerEmpty(t *testing.T) {
	caller := &mockLLMCaller{}
	reranker := NewLLMReranker(caller)

	ranked, err := reranker.Rerank(context.Background(), "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ranked != nil {
		t.Fatalf("expected nil for empty docs, got %d", len(ranked))
	}
}

func TestLLMRerankerError(t *testing.T) {
	caller := &mockLLMCaller{err: fmt.Errorf("LLM unavailable")}
	reranker := NewLLMReranker(caller)

	_, err := reranker.Rerank(context.Background(), "query", []Document{{Content: "test"}})
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestParseRankingResponse(t *testing.T) {
	response := "Document 0: 7.5\nDocument 1: 2\nDocument 2: 10\n"
	scores := parseRankingResponse(response, 3)
	if scores[0] != 7.5 {
		t.Errorf("expected score 7.5, got %f", scores[0])
	}
	if scores[1] != 2.0 {
		t.Errorf("expected score 2.0, got %f", scores[1])
	}
	if scores[2] != 10.0 {
		t.Errorf("expected score 10.0, got %f", scores[2])
	}
}
