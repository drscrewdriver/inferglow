package flow

import (
	"context"
	"testing"
)

// TestHandleBranches_NilFalseStep verifies that handleBranches does not panic
// when the condition is false and FalseStep is nil. It should skip the false
// branch and return the original output without executing any branch step.
//
// BUG-12 / F-HIGH-1: handleBranches 在 FalseStep 为 nil 时 panic.
// 修复：nil 检查后跳过 false 分支执行.
func TestHandleBranches_NilFalseStep(t *testing.T) {
	// 构造一个 Flow，其 branch 的 FalseStep 为 nil
	inputStep := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": false}, nil
	}).Build()

	trueStep := NewStep("trueStep", func(ctx context.Context, input any) (any, error) {
		return "approved", nil
	}).Build()

	fb := NewFlow().AddStep(inputStep)
	// 直接构造 branches，FalseStep 为 nil
	fb.flow.branches = append(fb.flow.branches, Branch{
		From:      "input",
		Cond:      func(input any) bool { return false }, // 条件为 false
		TrueStep:  trueStep,
		FalseStep: nil, // 故意设为 nil
	})
	flow := fb.Build()

	ctx := context.Background()

	// 不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleBranches panicked on nil FalseStep: %v", r)
		}
	}()

	exec := flow.Execute(ctx, nil)

	// 期望执行完成（不因 nil FalseStep 失败）
	if exec.State.Status == StatusFailed {
		t.Fatalf("expected non-failed status, got %s, errors: %v", exec.State.Status, exec.State.Errors)
	}

	// TrueStep 不应被调用（条件为 false）
	if _, ok := exec.State.StepLog["trueStep"]; ok {
		t.Error("trueStep should NOT be in StepLog when condition is false")
	}
}
