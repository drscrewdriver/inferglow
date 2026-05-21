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

package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
)

func TestMockSearchProviderDefault(t *testing.T) {
	p := &MockSearchProvider{}
	results, err := p.Search(context.Background(), "golang")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Mock result for golang" {
		t.Errorf("Title = %q", results[0].Title)
	}
}

func TestMockSearchProviderPreset(t *testing.T) {
	preset := []SearchResult{
		{Title: "First", URL: "https://first.example", Snippet: "s1"},
		{Title: "Second", URL: "https://second.example", Snippet: "s2"},
	}
	p := &MockSearchProvider{Results: preset}
	results, err := p.Search(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Ensure the returned slice is a copy (mutations don't leak back).
	results[0].Title = "Mutated"
	if preset[0].Title == "Mutated" {
		t.Errorf("Search returned a slice aliasing internal state")
	}
}

func TestMockSearchProviderError(t *testing.T) {
	p := &MockSearchProvider{Err: errors.New("backend down")}
	_, err := p.Search(context.Background(), "x")
	if err == nil || err.Error() != "backend down" {
		t.Errorf("expected 'backend down' error, got %v", err)
	}
}

func TestWebSearchSpec(t *testing.T) {
	if WebSearchSpec.SideEffectLevel != action.SideEffectRead {
		t.Errorf("SideEffectLevel = %q, want %q", WebSearchSpec.SideEffectLevel, action.SideEffectRead)
	}
	if WebSearchSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if WebSearchSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
}

func TestWebSearchExecutorSuccess(t *testing.T) {
	provider := &MockSearchProvider{
		Results: []SearchResult{{Title: "Hit", URL: "https://hit.example", Snippet: "snip"}},
	}
	a := NewWebSearchAction(provider)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"query": "test",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	results, ok := res.Result.([]SearchResult)
	if !ok {
		t.Fatalf("Result not []SearchResult: %T", res.Result)
	}
	if len(results) != 1 || results[0].Title != "Hit" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestWebSearchExecutorEmptyQuery(t *testing.T) {
	a := NewWebSearchAction(&MockSearchProvider{})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"query": "",
	})
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false for empty query")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestWebSearchExecutorProviderError(t *testing.T) {
	a := NewWebSearchAction(&MockSearchProvider{Err: errors.New("rate limited")})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"query": "test",
	})
	if res.OK {
		t.Errorf("expected OK=false when provider errors")
	}
}

func TestWebSearchExecutorNilProviderDefaultsToMock(t *testing.T) {
	a := NewWebSearchAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"query": "anything",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true with default mock, got %+v", res)
	}
}

func TestWebSearchActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewWebSearchAction(&MockSearchProvider{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(WebSearchActionID) {
		t.Errorf("registry missing %q", WebSearchActionID)
	}
}
