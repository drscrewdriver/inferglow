package flow

import (
	"context"
	"testing"
)

// TestPauseReturnsLastExecutedStep verifies that Pause returns the LAST
// executed step, not a random one due to map iteration.
// Regression for BUG-4: Pause used `for name := range StepLog` which has
// random iteration order, so PausePoint could point to the wrong step.
func TestPauseReturnsLastExecutedStep(t *testing.T) {
	// Build and run a real flow: stepA -> stepB -> stepC
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

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if len(exec.State.StepLog) != 3 {
		t.Fatalf("expected 3 step log entries, got %d", len(exec.State.StepLog))
	}

	// Call Pause many times — each call iterates the StepLog map. Before the
	// fix, the "last" step is random per iteration; we expect it to always
	// be stepC (the actual last-executed step).
	for i := 0; i < 50; i++ {
		pp := exec.Pause("check")
		if pp.StepName != "stepC" {
			t.Errorf("iteration %d: Pause().StepName = %q, want %q (random map iteration bug)", i, pp.StepName, "stepC")
			break
		}
	}
}

// TestPauseInputFromLastExecutedStep verifies that Pause's Input field
// comes from the LAST executed step, not a random one.
func TestPauseInputFromLastExecutedStep(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return "stepA_output", nil
	}).Build()
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return "stepB_output", nil
	}).Build()
	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return "stepC_output", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()
	exec := flow.Execute(context.Background(), "start")

	// stepC's Input should be stepB's output ("stepB_output").
	for i := 0; i < 50; i++ {
		pp := exec.Pause("check")
		if pp.Input != "stepB_output" {
			t.Errorf("iteration %d: Pause().Input = %v, want %q (random map iteration bug)", i, pp.Input, "stepB_output")
			break
		}
	}
}
