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

// mockGrepRunner is a test double for GrepRunner.
type mockGrepRunner struct {
	matches []GrepMatch
	err     error
	gotReq  GrepRequest
}

func (m *mockGrepRunner) Run(ctx context.Context, req GrepRequest) ([]GrepMatch, error) {
	m.gotReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.matches, nil
}

func TestGrepActionSpec(t *testing.T) {
	a := NewGrepAction(&mockGrepRunner{})
	if a.Name != GrepActionID {
		t.Errorf("Name = %q, want %q", a.Name, GrepActionID)
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

func TestGrepActionRegistry(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewGrepAction(&mockGrepRunner{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(GrepActionID) {
		t.Errorf("registry missing %q", GrepActionID)
	}
}

func TestGrepExecutorSuccess(t *testing.T) {
	matches := []GrepMatch{
		{File: "test.txt", Line: 1, Content: "hello world"},
		{File: "test.txt", Line: 5, Content: "hello again"},
	}
	mock := &mockGrepRunner{matches: matches}
	a := NewGrepAction(mock)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "/tmp",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	result, ok := res.Result.(GrepResult)
	if !ok {
		t.Fatalf("Result not GrepResult: %T", res.Result)
	}
	if result.Pattern != "hello" {
		t.Errorf("Pattern = %q, want hello", result.Pattern)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
	if len(result.Matches) != 2 {
		t.Errorf("len(Matches) = %d, want 2", len(result.Matches))
	}
	if mock.gotReq.Pattern != "hello" {
		t.Errorf("gotReq.Pattern = %q", mock.gotReq.Pattern)
	}
	if mock.gotReq.Path != "/tmp" {
		t.Errorf("gotReq.Path = %q", mock.gotReq.Path)
	}
}

func TestGrepExecutorMissingPattern(t *testing.T) {
	a := NewGrepAction(&mockGrepRunner{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": "/tmp",
	})
	if res.OK {
		t.Errorf("expected OK=false for missing pattern")
	}
	if res.Error != "grep: pattern is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestGrepExecutorMissingPath(t *testing.T) {
	a := NewGrepAction(&mockGrepRunner{})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"pattern": "hello",
	})
	if res.OK {
		t.Errorf("expected OK=false for missing path")
	}
	if res.Error != "grep: path is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestGrepExecutorNilRunner(t *testing.T) {
	a := NewGrepAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "/tmp",
	})
	if res.OK {
		t.Errorf("expected OK=false for nil runner")
	}
	if res.Error != "grep: no runner injected" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestGrepExecutorRunnerError(t *testing.T) {
	mock := &mockGrepRunner{err: errors.New("permission denied")}
	a := NewGrepAction(mock)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "/tmp",
	})
	if res.OK {
		t.Errorf("expected OK=false when runner errors")
	}
	if res.Error != "grep: permission denied" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestGrepExecutorRecursiveFlag(t *testing.T) {
	mock := &mockGrepRunner{matches: []GrepMatch{}}
	a := NewGrepAction(mock)
	_, _ = a.Executor.Execute(context.Background(), map[string]any{
		"pattern":   "hello",
		"path":      "/tmp",
		"recursive": true,
	})
	if !mock.gotReq.Recursive {
		t.Error("expected recursive=true")
	}
}

func TestGrepExecutorMaxResults(t *testing.T) {
	mock := &mockGrepRunner{matches: []GrepMatch{}}
	a := NewGrepAction(mock)
	_, _ = a.Executor.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"path":        "/tmp",
		"max_results": float64(10),
	})
	if mock.gotReq.MaxResult != 10 {
		t.Errorf("MaxResult = %d, want 10", mock.gotReq.MaxResult)
	}
}