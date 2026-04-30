package flow

import (
	"context"
	"fmt"
	"time"
)

// ExecutionStatus represents the status of a flow execution
type ExecutionStatus string

const (
	StatusCreated   ExecutionStatus = "created"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusPaused    ExecutionStatus = "paused"
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
}

// Execution holds the result of a flow execution
type Execution struct {
	State ExecutionState
}

// Execute runs the flow from the starting step (no incoming edges) through all connected steps
func (f *Flow) Execute(ctx context.Context, input any) *Execution {
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

		if err != nil {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, err)
			return exec
		}

		// After executing, check for conditional branches from this step
		output, branchStepName := f.handleBranches(exec, currentStepName, output, ctx)
		if exec.State.Status == StatusFailed {
			return exec
		}
		if branchStepName != "" {
			// Branch was executed; continue from the branch step
			// The branch step's output is already in `output`
			currentInput = output
			currentStepName = branchStepName
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
			if err != nil {
				exec.State.Status = StatusFailed
				exec.State.Errors = append(exec.State.Errors, err)
				return nil, ""
			}
			return trueOutput, branch.TrueStep.Name
		}
		// Execute false branch
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
	// Find a step that is not a target of any edge
	for name, step := range f.steps {
		if !targets[name] {
			return step
		}
	}
	// Fallback: return first step
	for _, step := range f.steps {
		return step
	}
	return nil
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
