// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/flowdef"
)

// FlowContextFactory creates a FlowContext for a run. This allows the server
// to remain independent of orchestrator while still supporting LLM-backed
// stage functions.
type FlowContextFactory func(ctx context.Context) flow.FlowContext

// SessionEndHook is called after a flow run completes successfully.
// It allows external components (e.g. LongMemPromoter) to perform
// session-end tasks such as promoting qualifying facts to long-term memory.
// The context package's LongMemPromoter.OnSessionEnd satisfies this interface.
type SessionEndHook func(ctx context.Context) error

// RunStatus represents the lifecycle state of a Run.
type RunStatus string

const (
	RunStatusPending  RunStatus = "pending"
	RunStatusRunning  RunStatus = "running"
	RunStatusPaused   RunStatus = "paused"
	RunStatusDone     RunStatus = "done"
	RunStatusFailed   RunStatus = "failed"
	RunStatusCanceled RunStatus = "canceled"
)

// RunEvent represents a single lifecycle event emitted during a run.
type RunEvent struct {
	Type      string    `json:"type"`
	Step      string    `json:"step,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data,omitempty"`
}

// RunHandle is the control handle for a single flow execution.
type RunHandle struct {
	ID         string                   `json:"run_id"`
	FlowName   string                   `json:"flow"`
	Status     RunStatus                `json:"status"`
	Inputs     map[string]any           `json:"inputs,omitempty"`
	Output     map[string]any           `json:"output,omitempty"`
	Error      string                   `json:"error,omitempty"`
	Owner      string                   `json:"owner,omitempty"`
	StartedAt  time.Time                `json:"started_at"`
	FinishedAt *time.Time               `json:"finished_at,omitempty"`
	Events     chan RunEvent            `json:"-"`
	ExecState  *flow.ExecutionState     `json:"-"` // read-only execution state snapshot
	cancel     context.CancelFunc
	mu         sync.Mutex
}

// RunManager manages the lifecycle of flow runs.
type RunManager struct {
	mu             sync.RWMutex
	runs           map[string]*RunHandle
	store          *FlowStore
	seq            int
	fctxFactory    FlowContextFactory
	sessionEndHook SessionEndHook
}

// NewRunManager creates a RunManager backed by the given FlowStore.
func NewRunManager(store *FlowStore) *RunManager {
	return &RunManager{
		runs:  make(map[string]*RunHandle),
		store: store,
	}
}

// SetFlowContextFactory sets the factory used to create FlowContext for runs.
// This allows stage functions to call LLM via fctx.GenerateModel().
func (rm *RunManager) SetFlowContextFactory(factory FlowContextFactory) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.fctxFactory = factory
}

// SetSessionEndHook sets a hook that is called after each successful flow run.
// Typical use: inject LongMemPromoter.OnSessionEnd to auto-promote facts.
func (rm *RunManager) SetSessionEndHook(hook SessionEndHook) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.sessionEndHook = hook
}

// Start creates and executes a new run for the named flow.
func (rm *RunManager) Start(flowName string, inputs map[string]any, owner string) (*RunHandle, error) {
	def, ok := rm.store.Get(flowName)
	if !ok {
		return nil, fmt.Errorf("flow %q not found", flowName)
	}

	rm.mu.Lock()
	rm.seq++
	id := fmt.Sprintf("run-%04d", rm.seq)

	ctx, cancel := context.WithCancel(context.Background())
	handle := &RunHandle{
		ID:        id,
		FlowName:  flowName,
		Status:    RunStatusPending,
		Inputs:    inputs,
		Owner:     owner,
		StartedAt: time.Now(),
		Events:    make(chan RunEvent, 64),
		cancel:    cancel,
	}
	rm.runs[id] = handle
	rm.mu.Unlock()

	// Execute in background goroutine.
	go rm.execute(ctx, handle, def, inputs)

	return handle, nil
}

// execute compiles the FlowDef and runs it.
func (rm *RunManager) execute(ctx context.Context, handle *RunHandle, def *flowdef.FlowDef, inputs map[string]any) {
	handle.mu.Lock()
	handle.Status = RunStatusRunning
	handle.emit(RunEvent{Type: "run_started", Timestamp: time.Now()})
	handle.mu.Unlock()

	// Inject FlowContext if factory is available.
	rm.mu.RLock()
	factory := rm.fctxFactory
	rm.mu.RUnlock()
	if factory != nil {
		fctx := factory(ctx)
		ctx = flow.WithFlowContext(ctx, fctx)
	}

	// Compile FlowDef into executable Flow.
	f, err := rm.store.Adapter().ToFlow(def)
	if err != nil {
		rm.failRun(handle, fmt.Errorf("compile flow: %w", err))
		return
	}

	// Build initial data map from inputs.
	data := make(map[string]any, len(inputs))
	for k, v := range inputs {
		data[k] = v
	}

	// Execute the flow.
	exec := f.Execute(ctx, data)

	// Save execution state for inspection (read-only).
	handle.mu.Lock()
	handle.ExecState = &exec.State
	handle.mu.Unlock()

	if len(exec.State.Errors) > 0 {
		if ctx.Err() != nil {
			rm.failRun(handle, fmt.Errorf("run canceled"))
			return
		}
		rm.failRun(handle, fmt.Errorf("execute: %v", exec.State.Errors))
		return
	}

	// Extract output.
	output := make(map[string]any)
	if m, ok := exec.State.Result.(map[string]any); ok {
		output = m
	} else if exec.State.Result != nil {
		output["response"] = exec.State.Result
	}

	handle.mu.Lock()
	handle.Status = RunStatusDone
	handle.Output = output
	now := time.Now()
	handle.FinishedAt = &now

	// Emit step-level events for observability.
	for _, stepName := range exec.State.StepExecLog {
		if entry, ok := exec.State.StepLog[stepName]; ok {
			handle.emit(RunEvent{
				Type:      "step_done",
				Step:      stepName,
				Timestamp: time.Now(),
				Data: map[string]any{
					"duration": entry.Duration.String(),
				},
			})
		}
	}

	handle.emit(RunEvent{Type: "run_done", Timestamp: time.Now(), Data: output})
	handle.mu.Unlock()
	close(handle.Events)

	// Call session-end hook (e.g. LongMemPromoter) asynchronously.
	// Errors are logged but do not affect the run's success status.
	rm.mu.RLock()
	hook := rm.sessionEndHook
	rm.mu.RUnlock()
	if hook != nil {
		go func() {
			_ = hook(ctx)
		}()
	}
}

// failRun marks a run as failed.
func (rm *RunManager) failRun(handle *RunHandle, err error) {
	handle.mu.Lock()
	handle.Status = RunStatusFailed
	handle.Error = err.Error()
	now := time.Now()
	handle.FinishedAt = &now
	handle.emit(RunEvent{Type: "error", Timestamp: time.Now(), Data: map[string]string{"error": err.Error()}})
	handle.mu.Unlock()
	close(handle.Events)
}

// GetID returns the run's unique identifier.
func (h *RunHandle) GetID() string { return h.ID }

// GetStatus returns the run's current status as a string.
func (h *RunHandle) GetStatus() string { return string(h.Status) }

// emit sends an event to the run's event channel. Must be called with handle.mu held.
func (h *RunHandle) emit(ev RunEvent) {
	select {
	case h.Events <- ev:
	default:
		// Drop event if channel is full (non-blocking).
	}
}

// Status returns the current status of a run.
func (rm *RunManager) Status(id string) (*RunHandle, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	h, ok := rm.runs[id]
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	return h, nil
}

// List returns all runs, optionally filtered by status.
func (rm *RunManager) List(statusFilter RunStatus) []*RunHandle {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	var result []*RunHandle
	for _, h := range rm.runs {
		if statusFilter == "" || h.Status == statusFilter {
			result = append(result, h)
		}
	}
	return result
}

// Cancel cancels a running or paused run.
func (rm *RunManager) Cancel(id string) error {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Status != RunStatusRunning && h.Status != RunStatusPaused {
		return fmt.Errorf("run %q is not running (status: %s)", id, h.Status)
	}

	h.cancel()
	h.Status = RunStatusCanceled
	now := time.Now()
	h.FinishedAt = &now
	return nil
}

// Subscribe returns the event channel for a run and a cleanup function.
func (rm *RunManager) Subscribe(id string) (<-chan RunEvent, func(), error) {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("run %q not found", id)
	}
	cleanup := func() {}
	return h.Events, cleanup, nil
}

// Pause requests a flow to pause at the next step boundary.
func (rm *RunManager) Pause(id string) error {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Status != RunStatusRunning {
		return fmt.Errorf("run %q is not running (status: %s)", id, h.Status)
	}
	// TODO: integrate with flow.PauseSignal for cooperative pause.
	h.Status = RunStatusPaused
	h.emit(RunEvent{Type: "paused", Timestamp: time.Now()})
	return nil
}

// Resume resumes a paused run.
func (rm *RunManager) Resume(id string, resumeInput map[string]any) error {
	rm.mu.RLock()
	h, ok := rm.runs[id]
	rm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Status != RunStatusPaused {
		return fmt.Errorf("run %q is not paused (status: %s)", id, h.Status)
	}
	// TODO: integrate with flow.PauseSignal for cooperative resume.
	h.Status = RunStatusRunning
	h.emit(RunEvent{Type: "resumed", Timestamp: time.Now()})
	return nil
}

// Flow returns the *flow.Flow for a given FlowDef (compiles it).
func (f *FlowStore) Flow(name string) (*flow.Flow, error) {
	def, ok := f.Get(name)
	if !ok {
		return nil, fmt.Errorf("flow %q not found", name)
	}
	return f.Adapter().ToFlow(def)
}
