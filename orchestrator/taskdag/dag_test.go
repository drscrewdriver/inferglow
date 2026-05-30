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

package taskdag

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// echoHandler returns the node's input as its result.
type echoHandler struct{}

func (echoHandler) Execute(_ context.Context, tctx *TaskDAGContext) (any, error) {
	return tctx.CurrentNode.Input, nil
}

// failHandler always fails.
type failHandler struct{}

func (failHandler) Execute(context.Context, *TaskDAGContext) (any, error) {
	return nil, errors.New("intentional failure")
}

func TestValidateOK(t *testing.T) {
	dag := &TaskDAG{
		ID: "test",
		Tasks: []TaskNode{
			{ID: "a"},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: []string{"a", "b"}},
		},
	}
	if err := Validate(dag); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCycle(t *testing.T) {
	dag := &TaskDAG{
		ID: "cycle",
		Tasks: []TaskNode{
			{ID: "a", DependsOn: []string{"b"}},
			{ID: "b", DependsOn: []string{"a"}},
		},
	}
	if err := Validate(dag); !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateDuplicate(t *testing.T) {
	dag := &TaskDAG{
		ID: "dup",
		Tasks: []TaskNode{
			{ID: "a"},
			{ID: "a"},
		},
	}
	if err := Validate(dag); !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("expected ErrDuplicateNode, got %v", err)
	}
}

func TestValidateMissingDep(t *testing.T) {
	dag := &TaskDAG{
		ID: "missing",
		Tasks: []TaskNode{
			{ID: "a", DependsOn: []string{"nonexistent"}},
		},
	}
	if err := Validate(dag); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestTopoSort(t *testing.T) {
	dag := &TaskDAG{
		ID: "sort",
		Tasks: []TaskNode{
			{ID: "c", DependsOn: []string{"a", "b"}},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "a"},
		},
	}
	sorted, err := TopoSort(dag)
	if err != nil {
		t.Fatalf("TopoSort: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(sorted))
	}
	// a must come before b and c.
	pos := make(map[string]int)
	for i, n := range sorted {
		pos[n.ID] = i
	}
	if pos["a"] >= pos["b"] || pos["a"] >= pos["c"] {
		t.Errorf("a should come first: %v", pos)
	}
}

func TestExecutorRun(t *testing.T) {
	dag := &TaskDAG{
		ID: "exec",
		Tasks: []TaskNode{
			{ID: "a", Kind: "echo", Input: map[string]any{"value": "hello"}},
			{ID: "b", Kind: "echo", DependsOn: []string{"a"}, Input: map[string]any{"value": "world"}},
		},
	}

	resolver := NewStaticResolver()
	resolver.Register("echo", echoHandler{})

	exec := NewTaskDAGExecutor(resolver)
	results, err := exec.Run(context.Background(), dag, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestExecutorOptionalSkip(t *testing.T) {
	dag := &TaskDAG{
		ID: "opt",
		Tasks: []TaskNode{
			{ID: "a", Kind: "fail", Optional: true},
			{ID: "b", Kind: "echo", DependsOn: []string{"a"}, Optional: true, Input: map[string]any{"value": "ok"}},
		},
	}

	resolver := NewStaticResolver()
	resolver.Register("echo", echoHandler{})
	resolver.Register("fail", failHandler{})

	exec := NewTaskDAGExecutor(resolver)
	results, err := exec.Run(context.Background(), dag, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// a failed, b is optional and should be skipped.
	if _, ok := results["b"]; ok {
		t.Error("optional node b should have been skipped")
	}
}

func TestExecutorNonOptionalFail(t *testing.T) {
	dag := &TaskDAG{
		ID: "fail",
		Tasks: []TaskNode{
			{ID: "a", Kind: "fail"},
		},
	}

	resolver := NewStaticResolver()
	resolver.Register("fail", failHandler{})

	exec := NewTaskDAGExecutor(resolver)
	_, err := exec.Run(context.Background(), dag, nil)
	if !errors.Is(err, ErrNodeFailed) {
		t.Fatalf("expected ErrNodeFailed, got %v", err)
	}
}

func TestStaticResolverUnknown(t *testing.T) {
	resolver := NewStaticResolver()
	node := &TaskNode{ID: "x", Kind: "unknown"}
	_, err := resolver.Resolve(node)
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("expected ErrHandlerNotFound, got %v", err)
	}
}

func TestTaskDAGContext(t *testing.T) {
	tctx := &TaskDAGContext{
		NodeResults: make(map[string]any),
	}
	tctx.SetNodeResult("a", 42)
	v, ok := tctx.GetNodeResult("a")
	if !ok || v != 42 {
		t.Errorf("expected 42, got %v", v)
	}
	_, ok = tctx.GetNodeResult("missing")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestExecutorDiamondDAG(t *testing.T) {
	// Diamond: a → b, a → c, b+c → d
	dag := &TaskDAG{
		ID: "diamond",
		Tasks: []TaskNode{
			{ID: "a", Kind: "echo", Input: map[string]any{"v": 1}},
			{ID: "b", Kind: "echo", DependsOn: []string{"a"}, Input: map[string]any{"v": 2}},
			{ID: "c", Kind: "echo", DependsOn: []string{"a"}, Input: map[string]any{"v": 3}},
			{ID: "d", Kind: "echo", DependsOn: []string{"b", "c"}, Input: map[string]any{"v": 4}},
		},
	}

	resolver := NewStaticResolver()
	resolver.Register("echo", echoHandler{})

	exec := NewTaskDAGExecutor(resolver)
	results, err := exec.Run(context.Background(), dag, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, ok := results[id]; !ok {
			t.Errorf("missing result for %s", id)
		}
	}
	_ = fmt.Sprintf("test")
}
