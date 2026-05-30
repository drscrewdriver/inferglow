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

	"github.com/inferglow/flow"
)

// Compile compiles a TaskDAG into a flow.Flow. Each task node becomes
// a step in the flow, with edges representing dependencies.
func Compile(dag *TaskDAG) (*flow.Flow, error) {
	if err := Validate(dag); err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	sorted, err := TopoSort(dag)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	builder := flow.NewFlow()
	nodeSteps := make(map[string]*flow.Step, len(dag.Tasks))

	for _, node := range sorted {
		stepName := fmt.Sprintf("task_%s", node.ID)
		// Capture node for closure.
		n := node
		step := &flow.Step{
			Name: stepName,
			Func: func(ctx context.Context, input any) (any, error) {
				return map[string]any{
					"task_id": n.ID,
					"kind":    n.Kind,
					"binding": n.Binding,
					"input":   n.Input,
				}, nil
			},
		}
		nodeSteps[node.ID] = step
	}

	// Add the first step, then chain the rest via To.
	if len(sorted) > 0 {
		firstStep := nodeSteps[sorted[0].ID]
		builder.AddStep(firstStep)
		for i := 1; i < len(sorted); i++ {
			builder.To(nodeSteps[sorted[i].ID])
		}
	}

	return builder.Build(), nil
}
