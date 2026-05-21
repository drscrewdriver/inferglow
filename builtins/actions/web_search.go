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
	"fmt"

	"github.com/inferglow/action"
)

// WebSearchActionID is the registered Action name for the web search.
const WebSearchActionID = "web_search"

// SearchResult represents a single search hit returned by a SearchProvider.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchProvider is the abstraction used by the web_search Action to
// dispatch a query. Concrete implementations may wrap Bing, Google,
// Brave, an internal RAG index, etc. The interface is intentionally
// minimal so callers can inject any backend.
type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// MockSearchProvider is a deterministic in-memory SearchProvider used
// by tests and as a default when no real backend is configured.
type MockSearchProvider struct {
	Results []SearchResult
	Err     error
}

// Search returns the preconfigured results (or error), echoing the
// query back in the first result's snippet when Results is empty so
// callers can verify wiring without crafting fixtures.
func (m *MockSearchProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(m.Results) == 0 {
		return []SearchResult{
			{
				Title:   "Mock result for " + query,
				URL:     "https://example.com/search?q=" + query,
				Snippet: "This is a deterministic mock result for query: " + query,
			},
		}, nil
	}
	out := make([]SearchResult, len(m.Results))
	copy(out, m.Results)
	return out, nil
}

// WebSearchSpec is the ActionSpec for web_search: read-only, no
// approval, no sandbox.
var WebSearchSpec = &action.ActionSpec{
	ActionID:         WebSearchActionID,
	Name:             "WebSearch",
	Description:      "Search the web for a query and return ranked results.",
	SideEffectLevel:  action.SideEffectRead,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       false,
	ExposeToModel:    true,
	Tags:             []string{"web", "search", "builtin"},
	Kwargs: map[string]any{
		"query": map[string]any{"type": "string", "required": true},
	},
	Returns: map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string"},
				"url":     map[string]any{"type": "string"},
				"snippet": map[string]any{"type": "string"},
			},
		},
	},
}

// webSearchExecutor binds a SearchProvider to the ActionExecutor
// contract.
type webSearchExecutor struct {
	provider SearchProvider
}

func (e *webSearchExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if e == nil || e.provider == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "web_search: no search provider configured",
		}, nil
	}
	query, _ := input["query"].(string)
	if query == "" {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "web_search: query is required",
		}, nil
	}
	results, err := e.provider.Search(ctx, query)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("web_search: %s", err.Error()),
		}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: results,
	}, nil
}

// NewWebSearchAction builds an Action that dispatches to provider. If
// provider is nil, a MockSearchProvider is used so the Action is always
// callable (useful for examples and early development).
func NewWebSearchAction(provider SearchProvider) *action.Action {
	if provider == nil {
		provider = &MockSearchProvider{}
	}
	return &action.Action{
		Name:        WebSearchActionID,
		Description: "Search the web for a query.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		},
		Executor: &webSearchExecutor{provider: provider},
		Tags:     []string{"web", "search", "builtin"},
	}
}
