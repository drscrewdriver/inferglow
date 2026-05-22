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
	"sync/atomic"
	"testing"
)

// TestBranchStepExecutedExactlyOnce verifies that when a branch is taken,
// the chosen branch step (TrueStep or FalseStep) is executed EXACTLY once.
// Regression for F-CRITICAL-1: handleBranches executed the branch step's
// Func and returned its name; the main loop then set currentStepName to
// the branch step name and continued, causing the branch step to be
// re-executed on the next iteration.
func TestBranchStepExecutedExactlyOnce(t *testing.T) {
	var trueStepCount int32

	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": true}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		atomic.AddInt32(&trueStepCount, 1)
		return "approved", nil
	}).Build()

	falseStep := NewStep("falseStep", func(ctx context.Context, input any) (any, error) {
		return "denied", nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	fb.If(func(input any) bool {
		m := input.(map[string]any)
		return m["valid"] == true
	}, trueStep, falseStep)
	flow := fb.Build()

	exec := flow.Execute(context.Background(), nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if got := atomic.LoadInt32(&trueStepCount); got != 1 {
		t.Errorf("trueStep should be executed exactly once, got %d (re-execution bug)", got)
	}
}

// TestBranchStepExecutedExactlyOnceFalseBranch verifies the same for the
// false branch.
func TestBranchStepExecutedExactlyOnceFalseBranch(t *testing.T) {
	var falseStepCount int32

	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": false}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		return "approved", nil
	}).Build()

	falseStep := NewStep("falseStep", func(ctx context.Context, input any) (any, error) {
		atomic.AddInt32(&falseStepCount, 1)
		return "denied", nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	fb.If(func(input any) bool {
		m := input.(map[string]any)
		return m["valid"] == true
	}, trueStep, falseStep)
	flow := fb.Build()

	exec := flow.Execute(context.Background(), nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if got := atomic.LoadInt32(&falseStepCount); got != 1 {
		t.Errorf("falseStep should be executed exactly once, got %d (re-execution bug)", got)
	}
}

// TestBranchStepLogEntryNotOverwritten verifies that the StepLog entry for
// a branch step preserves the ORIGINAL input (from the previous step's
// output), not the re-execution input.
func TestBranchStepLogEntryNotOverwritten(t *testing.T) {
	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return "input_output", nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		return "true_output", nil
	}).Build()

	falseStep := NewStep("falseStep", func(ctx context.Context, input any) (any, error) {
		return "false_output", nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	fb.If(func(input any) bool {
		return true
	}, trueStep, falseStep)
	flow := fb.Build()

	exec := flow.Execute(context.Background(), nil)

	entry, ok := exec.State.StepLog["trueStep"]
	if !ok {
		t.Fatal("expected StepLog entry for trueStep")
	}
	// trueStep was originally called with input "input_output" (from inputStep).
	// Without the fix, the entry is overwritten by the re-execution where
	// input is "true_output" (trueStep's own output).
	if entry.Input != "input_output" {
		t.Errorf("StepLog[trueStep].Input = %v, want %q (entry was overwritten by re-execution)", entry.Input, "input_output")
	}
}
