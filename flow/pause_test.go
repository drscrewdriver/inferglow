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
	"testing"
	"time"
)

// Test 1: Pause Creates PausePoint
func TestPauseCreatesPausePoint(t *testing.T) {
	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {
					StepName: "stepA",
					Input:    "start",
					Output:   "startA",
					Duration: 0,
				},
			},
		},
	}

	pp := exec.Pause("need review")

	if pp == nil {
		t.Fatal("PausePoint is nil")
	}
	if pp.StepName != "stepA" {
		t.Errorf("expected StepName 'stepA', got %s", pp.StepName)
	}
	if pp.Input != "start" {
		t.Errorf("expected Input 'start', got %v", pp.Input)
	}
	if pp.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if exec.State.Status != StatusPaused {
		t.Errorf("expected StatusPaused, got %s", exec.State.Status)
	}
}

// Test 2: Resume Creates New Execution
func TestResumeCreatesNewExecution(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).Build()

	pp := &PausePoint{
		StepName:  "stepA",
		Input:     "start",
		Timestamp: time.Now(),
	}

	newExec := flow.Resume(context.Background(), pp, "resumed")

	if newExec == nil {
		t.Fatal("Resume returned nil Execution")
	}
	if newExec.State.Status == StatusPaused || newExec.State.Status == StatusCreated {
		t.Errorf("expected active status, got %s", newExec.State.Status)
	}
}

// Test 3: Resume Continues Execution
func TestResumeContinuesExecution(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()

	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()

	pp := &PausePoint{
		StepName:  "stepA",
		Input:     "start",
		Timestamp: time.Now(),
	}

	newExec := flow.Resume(context.Background(), pp, "resumed")

	expected := "resumedBC"
	if newExec.State.Result != expected {
		t.Errorf("expected result '%s', got %v", expected, newExec.State.Result)
	}
	if _, ok := newExec.State.StepLog["stepB"]; !ok {
		t.Error("StepLog should contain entry for stepB")
	}
	if _, ok := newExec.State.StepLog["stepC"]; !ok {
		t.Error("StepLog should contain entry for stepC")
	}
}
