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
	"time"
)

// PausePoint records the state at which a flow was paused.
// CheckpointID carries the ID under which a checkpoint was persisted when the
// pause was performed through Flow.Pause with auto-checkpointing enabled; it is
// empty for a plain Execution.Pause or when auto-checkpointing is off.
type PausePoint struct {
	StepName     string
	Input        any
	Timestamp    time.Time
	CheckpointID string
}

// Pause captures the current execution state and transitions to paused status
func (e *Execution) Pause(reason string) *PausePoint {
	// Identify the LAST executed step deterministically. StepExecLog records
	// step names in execution order; fall back to map iteration only when
	// StepExecLog is empty (e.g. Execution constructed directly without
	// running Execute — see TestPauseCreatesPausePoint).
	lastStepName := ""
	if n := len(e.State.StepExecLog); n > 0 {
		lastStepName = e.State.StepExecLog[n-1]
	} else {
		for name := range e.State.StepLog {
			lastStepName = name
		}
	}
	// The last step's input is the last entry in StepLog
	lastStepInput := e.State.Result
	if entry := e.State.StepLog[lastStepName]; entry != nil {
		lastStepInput = entry.Input
	}

	pp := &PausePoint{
		StepName:  lastStepName,
		Input:     lastStepInput,
		Timestamp: time.Now(),
	}

	e.State.Status = StatusPaused
	return pp
}

// Resume creates a new Execution starting from the paused step with new input
//
// BUG-16/F-MEDIUM-11: Resume 在每次循环迭代开始时检查 ctx.Done()。
// 若父 context 已取消，立即返回 StatusFailed 并记录 ctx.Err()。
//
// F-MEDIUM-12: 加读锁保护 f.steps / f.edges 的并发读。
func (f *Flow) Resume(ctx context.Context, pp *PausePoint, resumeInput any) *Execution {
	f.mu.RLock()
	defer f.mu.RUnlock()
	// Find the step name that was paused at - we resume from the NEXT step
	pausedStepName := pp.StepName
	// The next step is the one that follows the paused step in the flow
	nextStepName := f.findNextStep(pausedStepName)

	// Create new execution starting from the step after the paused one
	exec := &Execution{
		State: ExecutionState{
			Status:  StatusRunning,
			StepLog: make(map[string]*StepLogEntry),
		},
	}

	// 检查 context 是否已取消（在执行任何 step 之前）
	if err := ctx.Err(); err != nil {
		exec.State.Status = StatusFailed
		exec.State.Errors = append(exec.State.Errors, fmt.Errorf("resume: context cancelled before start: %w", err))
		return exec
	}

	// Execute from next step
	currentInput := resumeInput
	currentStepName := nextStepName

	for {
		// BUG-16/F-MEDIUM-11: 在每次循环迭代开始时检查 ctx.Done()
		select {
		case <-ctx.Done():
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, fmt.Errorf("resume: context cancelled: %w", ctx.Err()))
			return exec
		default:
		}

		step, ok := f.steps[currentStepName]
		if !ok {
			exec.State.Status = StatusCompleted
			exec.State.Result = currentInput
			return exec
		}

		start := time.Now()
		output, err := step.Func(ctx, currentInput)
		duration := time.Since(start)

		exec.State.StepLog[step.Name] = &StepLogEntry{
			StepName: step.Name,
			Input:    currentInput,
			Output:   output,
			Duration: duration,
			Error:    err,
		}
		exec.State.StepExecLog = append(exec.State.StepExecLog, step.Name)

		// A8: 在 Resume 路径同样支持 step 主动 RequestPause。
		if err != nil && errors.Is(err, ErrPauseRequested) {
			exec.State.Status = StatusPaused
			return exec
		}

		if err != nil {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, err)
			return exec
		}

		// After executing, check for conditional branches from this step.
		// This mirrors Execute's behavior so Resume honors branch logic.
		output, branchStepName := f.handleBranches(exec, currentStepName, output, ctx)
		if exec.State.Status == StatusFailed {
			return exec
		}
		if branchStepName != "" {
			// Branch was already executed inside handleBranches. Continue
			// from the NEXT step after the branch step.
			nextAfterBranch := f.findNextStep(branchStepName)
			if nextAfterBranch == "" {
				exec.State.Result = output
				exec.State.Status = StatusCompleted
				return exec
			}
			currentInput = output
			currentStepName = nextAfterBranch
			continue
		}

		nextStepName := f.findNextStep(currentStepName)
		if nextStepName == "" {
			exec.State.Result = output
			exec.State.Status = StatusCompleted
			return exec
		}

		currentInput = output
		currentStepName = nextStepName
	}
}
