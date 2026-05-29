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

package agent

import (
	"context"

	"github.com/inferglow/orchestrator/taskdag"
)

// ExecutionStrategy defines how an agent executes a turn.
type ExecutionStrategy interface {
	// Name returns the strategy identifier.
	Name() string
	// Execute runs one execution cycle and returns the result.
	Execute(ctx context.Context, agent *Agent, input any) (any, error)
}

// DirectStrategy is the default PLAN→EXECUTE loop strategy.
type DirectStrategy struct{}

// Name returns "direct".
func (DirectStrategy) Name() string { return "direct" }

// Execute delegates to the agent's existing run loop.
func (DirectStrategy) Execute(ctx context.Context, agent *Agent, input any) (any, error) {
	// The direct strategy uses the existing PLAN→EXECUTE loop in engine.go.
	// This is a pass-through that calls the agent's Run method.
	return agent.Run(ctx, input.(string))
}

// TaskDAGStrategy uses a model-generated task DAG for execution.
type TaskDAGStrategy struct {
	// Resolver maps task kinds to handlers.
	Resolver taskdag.HandlerResolver
}

// Name returns "task_dag".
func (TaskDAGStrategy) Name() string { return "task_dag" }

// Execute compiles the input into a DAG and runs it.
func (s TaskDAGStrategy) Execute(ctx context.Context, _ *Agent, input any) (any, error) {
	dag, ok := input.(*taskdag.TaskDAG)
	if !ok {
		return nil, ErrInvalidDAGInput
	}
	exec := taskdag.NewTaskDAGExecutor(s.Resolver)
	return exec.Run(ctx, dag, nil)
}
