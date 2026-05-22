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
	"strings"
	"testing"
)

// TestExecuteSequentialSteps verifies basic sequential execution
func TestExecuteSequentialSteps(t *testing.T) {
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

	exec := flow.Execute(context.Background(), "start")

	if exec.State.Result != "startABC" {
		t.Errorf("expected result 'startABC', got %v", exec.State.Result)
	}

	if len(exec.State.StepLog) != 3 {
		t.Errorf("expected 3 step log entries, got %d", len(exec.State.StepLog))
	}
	for _, name := range []string{"stepA", "stepB", "stepC"} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("expected step log entry for %s", name)
		}
	}
}

// TestExecutePassesIntermediateResults verifies that Step N output becomes Step N+1 input
func TestExecutePassesIntermediateResults(t *testing.T) {
	// Step A outputs a map
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"value": 10, "label": "initial"}, nil
	}).Build()

	// Step B receives the map and returns a new map with doubled value
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		m := input.(map[string]any)
		return map[string]any{
			"value": m["value"].(int) * 2,
			"label": "doubled",
		}, nil
	}).Build()

	// Step C reads the doubled value
	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		m := input.(map[string]any)
		return m["value"].(int), nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()

	exec := flow.Execute(context.Background(), "ignored")

	if exec.State.Result != 20 {
		t.Errorf("expected result 20, got %v", exec.State.Result)
	}

	// Verify Step B received Step A's output correctly
	stepBLog := exec.State.StepLog["stepB"]
	if stepBLog == nil {
		t.Fatal("missing stepB log entry")
	}
	inputMap := stepBLog.Input.(map[string]any)
	if inputMap["value"] != 10 || inputMap["label"] != "initial" {
		t.Errorf("stepB did not receive stepA's correct output, got %v", inputMap)
	}
}

// TestExecuteStopsOnFailure verifies that a failing step stops execution and doesn't run subsequent steps
func TestExecuteStopsOnFailure(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	testErr := errors.New("stepB failed")
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return nil, testErr
	}).Build()

	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()

	exec := flow.Execute(context.Background(), "start")

	// Verify status is failed
	if exec.State.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", exec.State.Status)
	}

	// Verify Errors list contains the error
	if len(exec.State.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(exec.State.Errors))
	}
	if exec.State.Errors[0] != testErr {
		t.Errorf("expected error to be testErr, got %v", exec.State.Errors[0])
	}

	// Verify StepLog has entry for stepB with Error field set
	stepBLog := exec.State.StepLog["stepB"]
	if stepBLog == nil {
		t.Fatal("missing stepB log entry")
	}
	if stepBLog.Error != testErr {
		t.Errorf("expected stepB log error to be testErr, got %v", stepBLog.Error)
	}

	// Verify step C was NOT executed (no entry in StepLog)
	if _, ok := exec.State.StepLog["stepC"]; ok {
		t.Error("stepC should not have been executed")
	}
}

// TestExecuteRecordsStepLog verifies that each step's log entry contains correct data
func TestExecuteRecordsStepLog(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		// Do some work to ensure measurable duration
		s := input.(string)
		for i := 0; i < 10000; i++ {
			s += "x"
		}
		return s, nil
	}).Build()

	flow := NewFlow().AddStep(stepA).Build()

	exec := flow.Execute(context.Background(), "hello")

	// Verify StepLog entry has correct StepName
	entry := exec.State.StepLog["stepA"]
	if entry == nil {
		t.Fatal("missing stepA log entry")
	}
	if entry.StepName != "stepA" {
		t.Errorf("expected StepName 'stepA', got %s", entry.StepName)
	}

	// Verify Input
	if entry.Input != "hello" {
		t.Errorf("expected Input 'hello', got %v", entry.Input)
	}

	// Verify Output contains the expected data (with appended 'x's)
	if !isStringWithXs(entry.Output, "hello") {
		t.Errorf("expected Output to start with 'hello' and contain 'x's, got %v", entry.Output)
	}

	// Verify Duration > 0
	if entry.Duration <= 0 {
		t.Errorf("expected Duration > 0, got %v", entry.Duration)
	}
}

// TestExecuteStatusTransitions verifies status transitions during execution
func TestExecuteStatusTransitions(t *testing.T) {
	// Test successful execution status
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + " done", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).Build()

	exec := flow.Execute(context.Background(), "test")

	// After successful Execute: Status should be StatusCompleted
	if exec.State.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", exec.State.Status)
	}
}

// TestExecuteStatusTransitionOnFailure verifies status is failed on error
func TestExecuteStatusTransitionOnFailure(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return nil, errors.New("failed")
	}).Build()

	flow := NewFlow().AddStep(stepA).Build()

	exec := flow.Execute(context.Background(), "test")

	// After failed Execute: Status should be StatusFailed
	if exec.State.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", exec.State.Status)
	}
}

func isStringWithXs(s any, prefix string) bool {
	str, ok := s.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(str, prefix) && strings.Contains(str, "x")
}
