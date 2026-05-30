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

package actionruntime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestDAGActionFlowRun(t *testing.T) {
	var count atomic.Int32
	exec := func(_ context.Context, call ActionCall) (*ActionResult, error) {
		count.Add(1)
		return &ActionResult{ActionName: call.Name, Result: "ok"}, nil
	}

	flow := NewDAGActionFlow(exec)
	calls := []ActionCall{
		{Name: "a", Params: map[string]any{"x": 1}},
		{Name: "b", Params: map[string]any{"x": 2}},
		{Name: "c", Params: map[string]any{"x": 3}},
	}

	results, err := flow.Run(context.Background(), calls)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if count.Load() != 3 {
		t.Errorf("expected 3 executions, got %d", count.Load())
	}
}

func TestDAGActionFlowEmpty(t *testing.T) {
	flow := NewDAGActionFlow(func(_ context.Context, _ ActionCall) (*ActionResult, error) {
		return nil, nil
	})
	_, err := flow.Run(context.Background(), nil)
	if !errors.Is(err, ErrDAGNoCalls) {
		t.Fatalf("expected ErrDAGNoCalls, got %v", err)
	}
}

func TestDAGActionFlowError(t *testing.T) {
	exec := func(_ context.Context, call ActionCall) (*ActionResult, error) {
		if call.Name == "fail" {
			return nil, errors.New("boom")
		}
		return &ActionResult{ActionName: call.Name, Result: "ok"}, nil
	}

	flow := NewDAGActionFlow(exec)
	calls := []ActionCall{
		{Name: "ok"},
		{Name: "fail"},
	}

	results, err := flow.Run(context.Background(), calls)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestDAGActionFlowConcurrency(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	exec := func(_ context.Context, call ActionCall) (*ActionResult, error) {
		c := current.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		// Simulate work.
		for i := 0; i < 1000; i++ {
			_ = i
		}
		current.Add(-1)
		return &ActionResult{ActionName: call.Name}, nil
	}

	flow := NewDAGActionFlow(exec)
	flow.SetConcurrency(2)

	calls := make([]ActionCall, 6)
	for i := range calls {
		calls[i] = ActionCall{Name: "task"}
	}

	_, err := flow.Run(context.Background(), calls)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if maxConcurrent.Load() > 2 {
		t.Errorf("max concurrent = %d, expected <= 2", maxConcurrent.Load())
	}
}
