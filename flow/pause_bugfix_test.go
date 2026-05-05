package flow

import (
	"context"
	"testing"
	"time"
)

// TestResume_HandleBranches verifies that Resume evaluates conditional branches
// after executing each step, just like Execute does.
//
// F-HIGH-8: Resume 不处理分支逻辑，handleBranches 未被调用.
// 修复：Resume 在每个 step 执行后调用 handleBranches.
//
// 场景：
//  1. Flow: stepA -> stepB (branch from stepB: cond 检查 output == "trigger")
//     - trueStep: stepTrue (输出 "<input>_true")
//  2. 在 stepA 处 Pause（模拟 stepA 执行后、stepB 执行前暂停）
//  3. Resume 从 stepB 开始执行，stepB 输出 "trigger"
//  4. 修复前：Resume 不调用 handleBranches，stepTrue 不执行
//  5. 修复后：Resume 调用 handleBranches，stepTrue 执行
func TestResume_HandleBranches(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		// 输出 "trigger" 以触发分支条件
		return "trigger", nil
	}).Build()

	trueStep := NewStep("stepTrue", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_true", nil
	}).Build()

	falseStep := NewStep("stepFalse", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_false", nil
	}).Build()

	fb := NewFlow().AddStep(stepA).To(stepB)
	fb.flow.branches = append(fb.flow.branches, Branch{
		From:      "stepB",
		Cond:      func(output any) bool { return output == "trigger" },
		TrueStep:  trueStep,
		FalseStep: falseStep,
	})
	flow := fb.Build()

	// 在 stepA 处暂停（stepA 已执行，stepB 未执行）
	pp := &PausePoint{
		StepName:  "stepA",
		Input:     "start",
		Timestamp: time.Now(),
	}

	// Resume 从 stepB 开始执行
	newExec := flow.Resume(context.Background(), pp, "resumed")

	if newExec.State.Status == StatusFailed {
		t.Fatalf("Resume failed: %v", newExec.State.Errors)
	}

	// stepB 应被执行
	if _, ok := newExec.State.StepLog["stepB"]; !ok {
		t.Error("StepLog should contain entry for stepB")
	}

	// stepTrue 应被执行（分支条件为 true）
	if _, ok := newExec.State.StepLog["stepTrue"]; !ok {
		t.Error("StepLog should contain entry for stepTrue (branch not evaluated in Resume)")
	}

	// stepFalse 不应被执行
	if _, ok := newExec.State.StepLog["stepFalse"]; ok {
		t.Error("StepLog should NOT contain entry for stepFalse (condition was true)")
	}
}

// TestResume_HandleBranches_FalseBranch verifies that Resume evaluates the false
// branch when the condition is false.
func TestResume_HandleBranches_FalseBranch(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		// 输出 "no_trigger" 不触发分支条件
		return "no_trigger", nil
	}).Build()

	trueStep := NewStep("stepTrue", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_true", nil
	}).Build()

	falseStep := NewStep("stepFalse", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_false", nil
	}).Build()

	fb := NewFlow().AddStep(stepA).To(stepB)
	fb.flow.branches = append(fb.flow.branches, Branch{
		From:      "stepB",
		Cond:      func(output any) bool { return output == "trigger" },
		TrueStep:  trueStep,
		FalseStep: falseStep,
	})
	flow := fb.Build()

	pp := &PausePoint{
		StepName:  "stepA",
		Input:     "start",
		Timestamp: time.Now(),
	}

	newExec := flow.Resume(context.Background(), pp, "resumed")

	if newExec.State.Status == StatusFailed {
		t.Fatalf("Resume failed: %v", newExec.State.Errors)
	}

	// stepFalse 应被执行（分支条件为 false）
	if _, ok := newExec.State.StepLog["stepFalse"]; !ok {
		t.Error("StepLog should contain entry for stepFalse (false branch not evaluated in Resume)")
	}

	// stepTrue 不应被执行
	if _, ok := newExec.State.StepLog["stepTrue"]; ok {
		t.Error("StepLog should NOT contain entry for stepTrue (condition was false)")
	}
}

// ============================================================================
// BUG-16 / F-MEDIUM-11: Resume 检查 ctx.Done
//
// 现状（修复前）：Resume 的循环不检查 ctx.Done()，即使父 context 已被取消，
// Resume 仍会继续执行所有剩余 step。这在长时间运行的 flow 中会导致无法
// 取消 Resume（例如客户端超时、用户手动取消等场景）。
//
// 修复要求：Resume 在每次循环迭代开始时检查 ctx.Done()，若已取消则立即
// 返回当前 Execution（状态为 StatusFailed，错误包含 ctx.Err()）。
// ============================================================================

// TestResume_ChecksContextCancellation 验证 Resume 在父 context 取消时
// 立即停止执行后续 step。修复前，即使 ctx 已取消，Resume 仍会执行所有 step。
func TestResume_ChecksContextCancellation(t *testing.T) {
	// 构造一个 flow：stepA -> stepB -> stepC
	// stepB 执行 50ms，stepC 执行 50ms
	// ctx 在 25ms 后取消（stepB 执行期间），stepC 迭代开始时应看到 ctx.Done
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()

	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		time.Sleep(50 * time.Millisecond)
		return input.(string) + "B", nil
	}).Build()

	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()

	// PausePoint.StepName = "stepA" → Resume 从 findNextStep("stepA") = "stepB" 开始
	pp := &PausePoint{
		StepName:  "stepA",
		Input:     "start",
		Timestamp: time.Now(),
	}

	// ctx 在 25ms 后取消（stepB 执行期间：stepB 0-50ms，ctx 25ms 取消）
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	start := time.Now()
	exec := flow.Resume(ctx, pp, "resumed")
	elapsed := time.Since(start)

	// 修复前：Resume 会执行 stepB (50ms) + stepC (0ms) = 50ms，返回 completed
	// 修复后：stepB 执行完后 (50ms)，下次循环检查 ctx.Done() → 已取消 → 立即返回
	// 总耗时 ~50ms（stepB 完成） + 立即返回
	if elapsed > 80*time.Millisecond {
		t.Errorf("Resume took %v to complete, expected < 80ms (should respect ctx cancellation)", elapsed)
	}

	// stepC 不应被执行（ctx 在 stepB 期间取消，stepC 迭代开始时 ctx.Done）
	if _, ok := exec.State.StepLog["stepC"]; ok {
		t.Error("stepC should NOT be in StepLog (ctx was cancelled before stepC iteration)")
	}

	// Resume 应该返回非 completed 状态（因为 ctx 被取消）
	if exec.State.Status == StatusCompleted {
		t.Error("Resume should not return StatusCompleted when ctx was cancelled")
	}
}

// TestResume_RespectsAlreadyCancelledContext 验证 Resume 在 ctx 已取消时
// 立即返回，不执行任何 step。
func TestResume_RespectsAlreadyCancelledContext(t *testing.T) {
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

	// 创建一个已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec := flow.Resume(ctx, pp, "resumed")

	// 不应执行任何 step
	if _, ok := exec.State.StepLog["stepB"]; ok {
		t.Error("stepB should NOT be in StepLog (ctx was already cancelled)")
	}

	// 状态不应是 completed
	if exec.State.Status == StatusCompleted {
		t.Error("Resume should not return StatusCompleted when ctx was already cancelled")
	}
}

// TestResume_ContextCancellationReturnsError 验证 Resume 在 ctx 取消后
// 返回的 Execution 包含错误信息。
func TestResume_ContextCancellationReturnsError(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec := flow.Resume(ctx, pp, "resumed")

	// 应该有错误记录
	if len(exec.State.Errors) == 0 {
		t.Error("Resume should record error when ctx is cancelled")
	}
}
