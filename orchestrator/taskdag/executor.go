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
	"fmt"
)

// TaskDAGExecutor runs a TaskDAG using a HandlerResolver.
type TaskDAGExecutor struct {
	resolver    HandlerResolver
	concurrency int
}

// NewTaskDAGExecutor creates an executor with the given resolver.
func NewTaskDAGExecutor(resolver HandlerResolver) *TaskDAGExecutor {
	return &TaskDAGExecutor{resolver: resolver, concurrency: 4}
}

// SetConcurrency sets the maximum number of parallel tasks.
func (e *TaskDAGExecutor) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	e.concurrency = n
}

// Run executes the DAG with the given input and returns node outputs.
func (e *TaskDAGExecutor) Run(ctx context.Context, dag *TaskDAG, input any) (map[string]any, error) {
	if err := Validate(dag); err != nil {
		return nil, err
	}

	sorted, err := TopoSort(dag)
	if err != nil {
		return nil, err
	}

	tctx := &TaskDAGContext{
		DAG:               dag,
		DAGInput:          input,
		DependencyResults: make(map[string]any),
		NodeResults:       make(map[string]any),
	}

	// Execute in topological order (sequential for correctness).
	for _, node := range sorted {
		tctx.CurrentNode = &node

		// Gather dependency results.
		deps := make(map[string]any, len(node.DependsOn))
		skipped := false
		for _, dep := range node.DependsOn {
			r, ok := tctx.GetNodeResult(dep)
			if !ok {
				if node.Optional {
					skipped = true
					break
				}
				return nil, fmt.Errorf("%w: dependency %s not resolved for %s", ErrNodeNotFound, dep, node.ID)
			}
			deps[dep] = r
		}
		if skipped {
			continue
		}
		tctx.DependencyResults = deps

		handler, err := e.resolver.Resolve(&node)
		if err != nil {
			if node.Optional {
				continue
			}
			return nil, fmt.Errorf("resolve handler for %s: %w", node.ID, err)
		}

		result, err := handler.Execute(ctx, tctx)
		if err != nil {
			if node.Optional {
				continue
			}
			return nil, fmt.Errorf("%w: %s: %v", ErrNodeFailed, node.ID, err)
		}
		tctx.SetNodeResult(node.ID, result)
	}

	return tctx.NodeResults, nil
}
