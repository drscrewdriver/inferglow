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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package actions

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// GrepActionID is the registered Action name for grep searches.
const GrepActionID = "grep"

// GrepRequest is the strongly-typed payload handed to a GrepRunner.
type GrepRequest struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
	MaxResult int    `json:"max_results,omitempty"`
}

// GrepMatch is a single match result from a grep search.
type GrepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// GrepResult is the structured outcome returned by grep.
type GrepResult struct {
	Pattern string      `json:"pattern"`
	Matches []GrepMatch `json:"matches"`
	Count   int         `json:"count"`
}

// GrepRunner is the abstraction that actually performs grep searches.
// Concrete implementations are injected by the caller — this package
// deliberately does NOT execute system commands itself.
type GrepRunner interface {
	// Run executes a grep search and returns matches.
	Run(ctx context.Context, req GrepRequest) ([]GrepMatch, error)
}

// grepExecutor wraps an injected GrepRunner behind the ActionExecutor contract.
type grepExecutor struct {
	runner GrepRunner
}

// NewGrepAction builds an Action that performs grep searches via the
// injected runner.
func NewGrepAction(runner GrepRunner) *action.Action {
	return &action.Action{
		Name:        GrepActionID,
		Description: "Search for a pattern in files using grep. Returns matching lines with file and line number.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":     map[string]any{"type": "string"},
				"path":        map[string]any{"type": "string"},
				"recursive":   map[string]any{"type": "boolean"},
				"max_results": map[string]any{"type": "integer"},
			},
			"required": []string{"pattern", "path"},
		},
		Executor: &grepExecutor{runner: runner},
		Tags:     []string{"search", "read", "builtin"},
	}
}

// Execute performs the grep search via the injected runner.
func (e *grepExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if e == nil || e.runner == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "grep: no runner injected",
		}, nil
	}

	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "grep: pattern is required"}, nil
	}
	path, _ := input["path"].(string)
	if path == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "grep: path is required"}, nil
	}

	req := GrepRequest{
		Pattern:   pattern,
		Path:      path,
		Recursive: input["recursive"] == true,
	}
	if maxR, ok := input["max_results"].(float64); ok && maxR > 0 {
		req.MaxResult = int(maxR)
	}

	matches, err := e.runner.Run(ctx, req)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("grep: %s", err),
		}, nil
	}

	result := GrepResult{
		Pattern: pattern,
		Matches: matches,
		Count:   len(matches),
	}

	return &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: result,
	}, nil
}
