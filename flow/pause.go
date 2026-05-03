package flow

import (
	"context"
	"time"
)

// PausePoint records the state at which a flow was paused
type PausePoint struct {
	StepName  string
	Input     any
	Timestamp time.Time
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
func (f *Flow) Resume(ctx context.Context, pp *PausePoint, resumeInput any) *Execution {
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

	// Execute from next step
	currentInput := resumeInput
	currentStepName := nextStepName

	for {
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

		if err != nil {
			exec.State.Status = StatusFailed
			exec.State.Errors = append(exec.State.Errors, err)
			return exec
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
