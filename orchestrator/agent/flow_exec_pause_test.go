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
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/flow"
	"github.com/inferglow/session"
)

// buildPauseTestFlow constructs a 3-step linear flow (s1 -> s2 -> s3) where
// each step appends its letter to the input string. step1 closes pauseCh so
// that the pause-signal check at the start of step2's iteration observes a
// closed channel and pauses execution after exactly one step.
func buildPauseTestFlow(t *testing.T, pauseCh chan struct{}) *flow.Flow {
	t.Helper()
	step1 := flow.NewStep("s1", func(ctx context.Context, input any) (any, error) {
		close(pauseCh)
		return input.(string) + "A", nil
	}).Build()
	step2 := flow.NewStep("s2", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()
	step3 := flow.NewStep("s3", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()
	return flow.NewFlow().AddStep(step1).To(step2).To(step3).Build()
}

// buildCompletionTestFlow constructs a 3-step linear flow without any pause
// signaling. Each step appends its letter to the input string so the final
// result is "<input>ABC".
func buildCompletionTestFlow(t *testing.T) *flow.Flow {
	t.Helper()
	step1 := flow.NewStep("s1", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()
	step2 := flow.NewStep("s2", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()
	step3 := flow.NewStep("s3", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()
	return flow.NewFlow().AddStep(step1).To(step2).To(step3).Build()
}

// newTestEngine builds an Engine backed by a fresh session and a zero-value
// mock requester, suitable for flow-execution tests that never call the LLM.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	return NewEngine(NewSessionExtension(sess), actExt, &mockModelRequester{})
}

// zeroRunConfig returns a runConfig with features disabled so no output hooks
// or PII maskers are consulted on the completed path.
func zeroRunConfig() *runConfig {
	return &runConfig{maxRounds: 10, features: Features{}}
}

// TestExecuteFlow_PauseSignal verifies that a closed pause channel causes
// executeFlow to return a *Execution with StatusPaused after exactly one step.
func TestExecuteFlow_PauseSignal(t *testing.T) {
	engine := newTestEngine(t)
	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)

	ctx := flow.WithPauseSignal(context.Background(), pauseCh)
	exec, resp, err := engine.executeFlow(ctx, f, "start", "", zeroRunConfig(), nil, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec == nil {
		t.Fatal("expected non-nil Execution")
	}
	if exec.State.Status != flow.StatusPaused {
		t.Errorf("expected StatusPaused, got %s", exec.State.Status)
	}
	if resp != "" {
		t.Errorf("expected empty response on pause, got %q", resp)
	}
	if len(exec.State.StepExecLog) != 1 {
		t.Errorf("expected exactly 1 step executed, got %d: %v", len(exec.State.StepExecLog), exec.State.StepExecLog)
	}
	if exec.State.StepExecLog[0] != "s1" {
		t.Errorf("expected first executed step to be s1, got %q", exec.State.StepExecLog[0])
	}
}

// TestExecuteFlow_NoPause_NormalCompletion verifies that without a pause
// signal the flow runs all 3 steps to completion and returns the response.
func TestExecuteFlow_NoPause_NormalCompletion(t *testing.T) {
	engine := newTestEngine(t)
	f := buildCompletionTestFlow(t)

	exec, resp, err := engine.executeFlow(context.Background(), f, "start", "", zeroRunConfig(), nil, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec == nil {
		t.Fatal("expected non-nil Execution")
	}
	if exec.State.Status != flow.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", exec.State.Status)
	}
	if resp != "startABC" {
		t.Errorf("expected response 'startABC', got %q", resp)
	}
	if len(exec.State.StepLog) != 3 {
		t.Errorf("expected 3 step log entries, got %d", len(exec.State.StepLog))
	}
}

// TestExecuteFlow_AutoCheckpoint verifies that auto-checkpoint options applied
// via opts cause a snapshot file to be persisted under the configured
// CheckPointID when the flow pauses mid-execution.
func TestExecuteFlow_AutoCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store := flow.NewFileCheckpointStore(dir)
	engine := newTestEngine(t)

	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)
	ctx := flow.WithPauseSignal(context.Background(), pauseCh)

	opts := []flow.FlowOption{
		flow.WithAutoCheckpoint(store),
		flow.WithCheckPointID("run-test"),
	}
	exec, _, err := engine.executeFlow(ctx, f, "start", "", zeroRunConfig(), opts, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}

	path := filepath.Join(dir, "run-test.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected checkpoint file at %s: %v", path, err)
	}
}

// TestExecuteFlow_RunMeta verifies that RunMeta fields (RunID/OwnerID/LeaseTTL)
// are propagated into the persisted snapshot via a WithStateModifier injected
// from meta. With no CheckPointID configured, the snapshot is saved under the
// RunID so the stateModifier's ExecutionID survives SaveCheckpoint.
func TestExecuteFlow_RunMeta(t *testing.T) {
	dir := t.TempDir()
	store := flow.NewFileCheckpointStore(dir)
	engine := newTestEngine(t)

	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)
	ctx := flow.WithPauseSignal(context.Background(), pauseCh)

	opts := []flow.FlowOption{
		flow.WithAutoCheckpoint(store),
	}
	meta := RunMeta{RunID: "run-1", OwnerID: "user-123", LeaseTTL: 300}
	exec, _, err := engine.executeFlow(ctx, f, "start", "", zeroRunConfig(), opts, meta)
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}

	loaded, err := store.Load("run-1")
	if err != nil {
		t.Fatalf("Load checkpoint 'run-1': %v", err)
	}
	if loaded.ExecutionID != "run-1" {
		t.Errorf("ExecutionID = %q, want 'run-1'", loaded.ExecutionID)
	}
	if loaded.OwnerID != "user-123" {
		t.Errorf("OwnerID = %q, want 'user-123'", loaded.OwnerID)
	}
	if loaded.LeaseTTL != 300 {
		t.Errorf("LeaseTTL = %d, want 300", loaded.LeaseTTL)
	}
}

// TestResumeFlow verifies that after a pause with auto-checkpoint, ResumeFlow
// loads the snapshot and resumes execution to completion with the correct
// final result.
func TestResumeFlow(t *testing.T) {
	dir := t.TempDir()
	store := flow.NewFileCheckpointStore(dir)
	engine := newTestEngine(t)

	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)
	ctx := flow.WithPauseSignal(context.Background(), pauseCh)

	const cpID = "resume-test"
	opts := []flow.FlowOption{
		flow.WithAutoCheckpoint(store),
		flow.WithCheckPointID(cpID),
	}
	exec, _, err := engine.executeFlow(ctx, f, "start", "", zeroRunConfig(), opts, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused after executeFlow, got %s", exec.State.Status)
	}

	resumed, err := engine.ResumeFlow(context.Background(), f, store, cpID, nil)
	if err != nil {
		t.Fatalf("ResumeFlow returned error: %v", err)
	}
	if resumed == nil {
		t.Fatal("ResumeFlow returned nil Execution")
	}
	if resumed.State.Status != flow.StatusCompleted {
		t.Errorf("expected StatusCompleted after resume, got %s", resumed.State.Status)
	}
	if resumed.State.Result != "startABC" {
		t.Errorf("expected result 'startABC', got %v", resumed.State.Result)
	}
	if _, ok := resumed.State.StepLog["s2"]; !ok {
		t.Error("expected s2 in StepLog after resume (remaining step executed)")
	}
	if _, ok := resumed.State.StepLog["s3"]; !ok {
		t.Error("expected s3 in StepLog after resume (remaining step executed)")
	}
}
