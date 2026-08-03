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

package flow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ExecutionStatus represents the status of a flow execution
type ExecutionStatus string

const (
	// StatusCreated indicates a flow execution has been created but not yet started.
	StatusCreated ExecutionStatus = "created"
	// StatusRunning indicates a flow execution is currently running.
	StatusRunning ExecutionStatus = "running"
	// StatusCompleted indicates a flow execution finished successfully.
	StatusCompleted ExecutionStatus = "completed"
	// StatusFailed indicates a flow execution terminated with an error.
	StatusFailed ExecutionStatus = "failed"
	// StatusPaused indicates a flow execution is paused and may be resumed.
	StatusPaused ExecutionStatus = "paused"
)

// StepLogEntry records execution details for a single step
type StepLogEntry struct {
	StepName string
	Input    any
	Output   any
	Duration time.Duration
	Error    error
}

// ExecutionState holds the current state of a flow execution
type ExecutionState struct {
	Status  ExecutionStatus
	Result  any
	Errors  []error
	StepLog map[string]*StepLogEntry
	// StepExecLog records step names in execution order. Used by Pause to
	// identify the LAST executed step deterministically (StepLog is a map
	// and cannot be relied on for ordering).
	StepExecLog []string
}

// Execution holds the result of a flow execution
type Execution struct {
	State ExecutionState
}

// Execute runs the flow from the starting step (no incoming edges) through all connected steps
//
// F-MEDIUM-12: 加读锁保护 f.steps / f.edges / f.branches / f.startStep 的并发读。
// 读锁允许多个 Execute 并发执行；与 FlowBuilder 的写锁互斥。
func (f *Flow) Execute(ctx context.Context, input any) *Execution {
	f.mu.RLock()
	defer f.mu.RUnlock()
	exec := &Execution{
		State: ExecutionState{
			Status:  StatusRunning,
			StepLog: make(map[string]*StepLogEntry),
		},
	}

	// Find starting step (no incoming edges)
	startStep := f.findStartStep()
	if startStep == nil {
		exec.State.Status = StatusFailed
		exec.State.Errors = append(exec.State.Errors, fmt.Errorf("no starting step found"))
		return exec
	}

	// Track visited steps to handle the linear chain
	visited := map[string]bool{}
	currentInput := input
	currentStepName := startStep.Name

	for {
		// A1: 在每个步骤执行前检查暂停信号。当 context 中注入了 pause channel
		// 且该 channel 已被关闭（或写入）时，立即将状态置为 StatusPaused 并返回，
		// 不执行当前步骤。非阻塞 select 保证未暂停时零开销。
		if pauseCh, ok := PauseSignalFrom(ctx); ok && pauseCh != nil {
			select {
			case <-pauseCh:
				exec.State.Status = StatusPaused
				return exec
			default:
			}
		}

		step, ok := f.steps[currentStepName]
		if !ok {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, fmt.Errorf("step %s not found", currentStepName))
			return exec
		}

		if visited[currentStepName] {
			exec.State.Result = currentInput
			exec.State.Status = StatusCompleted
			return exec
		}
		visited[currentStepName] = true

		// Execute the step
		start := time.Now()
		output, err := step.Func(ctx, currentInput)
		duration := time.Since(start)

		// Record step log
		exec.State.StepLog[step.Name] = &StepLogEntry{
			StepName: step.Name,
			Input:    currentInput,
			Output:   output,
			Duration: duration,
			Error:    err,
		}
		exec.State.StepExecLog = append(exec.State.StepExecLog, step.Name)

		// A8: step 主动请求挂起。当 step.Func 返回 ErrPauseRequested 时
		// （通常通过 Context.RequestPause 触发），将状态置为 StatusPaused
		// 而非 StatusFailed，且不把 ErrPauseRequested 追加到 Errors。这样
		// 上层 executeFlow 走暂停路径（f.Pause + 返回 exec 句柄），
		// daemon 可随后通过 ResumeFlow 续跑。
		if err != nil && errors.Is(err, ErrPauseRequested) {
			exec.State.Status = StatusPaused
			return exec
		}

		if err != nil {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, err)
			return exec
		}

		// L4 Step Schema validation: when step.Schema is set, validate the
		// output before passing it to the next step. This兑现 README 承诺 of
		// "Step 执行后自动验证输出是否符合 Schema 定义".
		if step.Schema != nil {
			if validateErr := validateStepOutput(output, step.Schema); validateErr != nil {
				exec.State.Status = StatusFailed
				exec.State.Errors = append(exec.State.Errors,
					fmt.Errorf("step %q output validation failed: %w", step.Name, validateErr))
				return exec
			}
		}

		// After executing, check for conditional branches from this step
		output, branchStepName := f.handleBranches(exec, currentStepName, output, ctx)
		if exec.State.Status == StatusFailed {
			return exec
		}
		if branchStepName != "" {
			// Branch was already executed inside handleBranches. Skip
			// re-execution of the branch step and continue from the NEXT
			// step after it (following the edges). Setting currentStepName
			// to branchStepName would re-execute the branch step on the
			// next loop iteration.
			nextAfterBranch := f.findNextStep(branchStepName)
			if nextAfterBranch == "" {
				// No further steps — execution complete.
				exec.State.Result = output
				exec.State.Status = StatusCompleted
				return exec
			}
			currentInput = output
			currentStepName = nextAfterBranch
			continue
		}

		// Find next step via edges
		nextStepName := f.findNextStep(currentStepName)
		if nextStepName == "" {
			// No more steps - execution complete
			exec.State.Result = output
			exec.State.Status = StatusCompleted
			return exec
		}

		currentInput = output
		currentStepName = nextStepName
	}
}

// handleBranches evaluates conditional branches from the given step.
// If a branch exists, evaluate the condition and execute the appropriate branch step.
// Returns the updated output from the branch step and the name of the executed branch step, or "" if no branch.
func (f *Flow) handleBranches(exec *Execution, fromStepName string, output any, ctx context.Context) (any, string) {
	for _, branch := range f.branches {
		if branch.From != fromStepName {
			continue
		}
		if branch.Cond(output) {
			// Execute true branch
			start := time.Now()
			trueOutput, err := branch.TrueStep.Func(ctx, output)
			duration := time.Since(start)
			exec.State.StepLog[branch.TrueStep.Name] = &StepLogEntry{
				StepName: branch.TrueStep.Name,
				Input:    output,
				Output:   trueOutput,
				Duration: duration,
				Error:    err,
			}
			exec.State.StepExecLog = append(exec.State.StepExecLog, branch.TrueStep.Name)
			if err != nil {
				exec.State.Status = StatusFailed
				exec.State.Errors = append(exec.State.Errors, err)
				return nil, ""
			}
			return trueOutput, branch.TrueStep.Name
		}
		// Execute false branch (skip if FalseStep is nil)
		if branch.FalseStep == nil {
			// No false branch defined: skip execution and return original output.
			return output, ""
		}
		start := time.Now()
		falseOutput, err := branch.FalseStep.Func(ctx, output)
		duration := time.Since(start)
		exec.State.StepLog[branch.FalseStep.Name] = &StepLogEntry{
			StepName: branch.FalseStep.Name,
			Input:    output,
			Output:   falseOutput,
			Duration: duration,
			Error:    err,
		}
		exec.State.StepExecLog = append(exec.State.StepExecLog, branch.FalseStep.Name)
		if err != nil {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, err)
			return nil, ""
		}
		return falseOutput, branch.FalseStep.Name
	}
	return output, ""
}

// findStartStep returns the first step in the chain (a step with no incoming edges)
func (f *Flow) findStartStep() *Step {
	// If a start step was explicitly set (by AddStep), use it
	if f.startStep != nil {
		return f.startStep
	}
	// Collect all "To" targets
	targets := make(map[string]bool)
	for _, e := range f.edges {
		targets[e.To] = true
	}
	// BUG-15/F-MEDIUM-1: 收集所有候选 start step（非任何 edge 的 target），
	// 按 Name 排序后取首个。原实现直接遍历 map，Go map 迭代顺序随机，
	// 导致多候选时返回结果不确定。
	var candidates []string
	for name := range f.steps {
		if !targets[name] {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) > 0 {
		sort.Strings(candidates)
		return f.steps[candidates[0]]
	}
	// Fallback: 所有 step 都是 target（循环图等），按 Name 排序取首个
	candidates = candidates[:0]
	for name := range f.steps {
		candidates = append(candidates, name)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Strings(candidates)
	return f.steps[candidates[0]]
}

// findNextStep returns the step name that the given step connects to
func (f *Flow) findNextStep(from string) string {
	for _, e := range f.edges {
		if e.From == from {
			return e.To
		}
	}
	return ""
}
