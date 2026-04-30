package flow

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

func TestIfConditionTrue(t *testing.T) {
	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": true}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
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

	ctx := context.Background()
	exec := flow.Execute(ctx, nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if exec.State.Result != "approved" {
		t.Fatalf("expected 'approved', got %v", exec.State.Result)
	}
	// Verify falseStep was NOT in StepLog
	if _, ok := exec.State.StepLog["falseStep"]; ok {
		t.Error("falseStep should not be in StepLog when condition is true")
	}
}

func TestIfConditionFalse(t *testing.T) {
	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": false}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
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

	ctx := context.Background()
	exec := flow.Execute(ctx, nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if exec.State.Result != "denied" {
		t.Fatalf("expected 'denied', got %v", exec.State.Result)
	}
	// Verify trueStep was NOT in StepLog
	if _, ok := exec.State.StepLog["trueStep"]; ok {
		t.Error("trueStep should not be in StepLog when condition is false")
	}
}

func TestBranchResultMerges(t *testing.T) {
	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": true}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"action": "approve", "value": 42}, nil
	}).Build()

	falseStep := NewStep("falseStep", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"action": "deny", "value": 0}, nil
	}).Build()

	capturedInput := make(chan any, 1)
	nextStep := NewStep("next", func(ctx context.Context, input any) (any, error) {
		capturedInput <- input
		return fmt.Sprintf("received: %v", input), nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	fb.If(func(input any) bool {
		m := input.(map[string]any)
		return m["valid"] == true
	}, trueStep, falseStep)
	fb.To(nextStep)
	flow := fb.Build()

	ctx := context.Background()
	exec := flow.Execute(ctx, nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if exec.State.Result != "received: map[action:approve value:42]" {
		t.Fatalf("expected 'received: map[action:approve value:42]', got %v", exec.State.Result)
	}
	// Verify nextStep received the trueStep's output
	select {
	case input := <-capturedInput:
		m := input.(map[string]any)
		if m["action"] != "approve" {
			t.Errorf("expected action=approve, got %v", m["action"])
		}
	default:
		t.Fatal("nextStep did not receive input")
	}
}

func TestSkippedBranchNotExecuted(t *testing.T) {
	callCount := int32(0)

	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": true}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		return "approved", nil
	}).Build()

	falseStep := NewStep("falseStep", func(ctx context.Context, input any) (any, error) {
		atomic.AddInt32(&callCount, 1)
		return "denied", nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	fb.If(func(input any) bool {
		m := input.(map[string]any)
		return m["valid"] == true
	}, trueStep, falseStep)
	flow := fb.Build()

	ctx := context.Background()
	exec := flow.Execute(ctx, nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", exec.State.Status)
	}
	if exec.State.Result != "approved" {
		t.Fatalf("expected 'approved', got %v", exec.State.Result)
	}
	// Verify falseStep Func was never called
	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("falseStep Func should not have been called, but was called %d times", callCount)
	}
}
