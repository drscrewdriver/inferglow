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
	"sync"

	"github.com/inferglow/action"
)

// Sentinel errors for DAG action flow.
var (
	ErrDAGNoCalls    = errors.New("dag: no action calls provided")
	ErrDAGActionMiss = errors.New("dag: action not found in registry")
)

// ActionResult holds the outcome of a single action execution.
type ActionResult struct {
	// ActionName is the name of the executed action.
	ActionName string `json:"action_name"`
	// Result is the action's output.
	Result any `json:"result,omitempty"`
	// Error is set if the action failed.
	Error string `json:"error,omitempty"`
}

// ActionExecutor is the function signature for executing a single action.
// This abstraction allows the DAG flow to work with any execution backend.
type ActionExecutor func(ctx context.Context, call ActionCall) (*ActionResult, error)

// DAGActionFlow upgrades action execution from serial to DAG-parallel.
// It analyzes ActionCalls for dependencies, builds an implicit execution
// DAG, and runs independent actions concurrently.
type DAGActionFlow struct {
	// executor runs individual action calls.
	executor ActionExecutor
	// maxConcurrency limits parallel action execution.
	maxConcurrency int
}

// NewDAGActionFlow creates a DAG flow with the given executor.
func NewDAGActionFlow(executor ActionExecutor) *DAGActionFlow {
	return &DAGActionFlow{
		executor:       executor,
		maxConcurrency: 4,
	}
}

// SetConcurrency sets the maximum parallelism.
func (f *DAGActionFlow) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	f.maxConcurrency = n
}

// Run executes all action calls, potentially in parallel when no
// data dependencies exist between them. In this implementation,
// all calls are treated as independent (no inter-call dependencies)
// and executed concurrently up to maxConcurrency.
func (f *DAGActionFlow) Run(ctx context.Context, calls []ActionCall) ([]*ActionResult, error) {
	if len(calls) == 0 {
		return nil, ErrDAGNoCalls
	}

	results := make([]*ActionResult, len(calls))
	var mu sync.Mutex
	sem := make(chan struct{}, f.maxConcurrency)
	var wg sync.WaitGroup
	var firstErr error

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c ActionCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r, err := f.executor(ctx, c)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if r != nil {
				results[idx] = r
			} else if err != nil {
				results[idx] = &ActionResult{
					ActionName: c.Name,
					Error:      err.Error(),
				}
			}
		}(i, call)
	}

	wg.Wait()
	return results, firstErr
}

// DefaultActionExecutor creates an ActionExecutor that dispatches through
// an action.ActionRegistry. This is a convenience adapter.
func DefaultActionExecutor(reg *action.ActionRegistry) ActionExecutor {
	return func(ctx context.Context, call ActionCall) (*ActionResult, error) {
		regResult, err := reg.Execute(ctx, call.Name, call.Params)
		if err != nil {
			return &ActionResult{ActionName: call.Name, Error: err.Error()}, err
		}
		return &ActionResult{ActionName: call.Name, Result: regResult}, nil
	}
}
