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
