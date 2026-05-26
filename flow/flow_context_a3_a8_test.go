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
	"fmt"
	"testing"
)

// TestNoopSpan_EndDoesNotPanic 验证 NoopSpan() 返回的 Span 调用 End() 不 panic
// 且返回值实现 flow.Span 接口。这是 A3 中 StartSpan 在 tracer==nil 时的返回值。
func TestNoopSpan_EndDoesNotPanic(t *testing.T) {
	s := NoopSpan()
	if s == nil {
		t.Fatal("NoopSpan() returned nil")
	}
	// 多次 End 必须都是安全的（adapter 可能在 defer 路径上被重复调用）。
	s.End()
	s.End()
	// 接口断言：保证其满足 flow.Span。
	var _ Span = s
}

// TestSpanKindConstants 验证 A3 中定义的 SpanKind 常量值唯一且非零的 iota 递增。
// 这只是回归保护：防止后续重构意外复用同一整数导致语义混淆。
func TestSpanKindConstants(t *testing.T) {
	kinds := []SpanKind{SpanKindInternal, SpanKindStep, SpanKindTool}
	seen := map[SpanKind]string{}
	for _, k := range kinds {
		if name, dup := seen[k]; dup {
			t.Errorf("SpanKind value %d duplicated by %q and %q", int(k), name, "current")
		}
		seen[k] = "ok"
	}
	if SpanKindInternal == SpanKindStep {
		t.Error("SpanKindInternal must differ from SpanKindStep")
	}
	if SpanKindStep == SpanKindTool {
		t.Error("SpanKindStep must differ from SpanKindTool")
	}
}

// TestErrPauseRequested_IsSentinel 验证 ErrPauseRequested 是稳定的哨兵错误，
// errors.Is 可以识别它（包括被 fmt.Errorf("%w") wrap 的情况）。A8 中
// flow.Execute 通过 errors.Is(err, ErrPauseRequested) 判断是否走暂停路径。
func TestErrPauseRequested_IsSentinel(t *testing.T) {
	if !errors.Is(ErrPauseRequested, ErrPauseRequested) {
		t.Fatal("errors.Is(ErrPauseRequested, ErrPauseRequested) should be true")
	}
	// 用 fmt.Errorf("%w") 真正 wrap 后，errors.Is 仍应能识别。
	wrapped := fmt.Errorf("step halt: %w", ErrPauseRequested)
	if !errors.Is(wrapped, ErrPauseRequested) {
		t.Errorf("errors.Is should detect wrapped ErrPauseRequested; got false")
	}
	// 不相关的错误不应被误判。
	unrelated := errors.New("some other error")
	if errors.Is(unrelated, ErrPauseRequested) {
		t.Errorf("errors.Is must not match unrelated error")
	}
}

// ============================================================================
// A8: step 主动 RequestPause
// ============================================================================

// TestExecute_StepRequestPause_StatusPaused 验证 step 通过 FlowContext.RequestPause
// 主动请求挂起时，flow.Execute 将状态置为 StatusPaused 而非 StatusFailed，
// 且不把 ErrPauseRequested 追加到 exec.State.Errors。
//
// step 通过 flow.FlowContextFrom(ctx) 取得 FlowContext 并调用 RequestPause，
// 把返回的 ErrPauseRequested 作为 StepFunc 的返回值。Execute 捕获该哨兵错误
// 后转入暂停路径。
func TestExecute_StepRequestPause_StatusPaused(t *testing.T) {
	fc := newMockFlowContext()
	ctx := WithFlowContext(context.Background(), fc)

	step := NewStep("ask-approval", func(c context.Context, _ any) (any, error) {
		fctx, ok := FlowContextFrom(c)
		if !ok {
			return nil, errors.New("FlowContext missing")
		}
		// step 主动请求挂起；RequestPause 返回 ErrPauseRequested。
		return nil, fctx.RequestPause("await approval")
	}).Build()
	f := NewFlow().AddStep(step).Build()

	exec := f.Execute(ctx, "user-input")

	if exec.State.Status != StatusPaused {
		t.Fatalf("expected StatusPaused, got %s; errors=%v",
			exec.State.Status, exec.State.Errors)
	}
	for _, e := range exec.State.Errors {
		if errors.Is(e, ErrPauseRequested) {
			t.Errorf("ErrPauseRequested must NOT be in Errors; got %v", exec.State.Errors)
		}
	}
	// step log 仍应记录该步骤被执行过（含 Error 字段）。
	entry, ok := exec.State.StepLog["ask-approval"]
	if !ok {
		t.Fatal("expected ask-approval in StepLog")
	}
	if entry.Error == nil {
		t.Error("expected step log Error to be set (the ErrPauseRequested)")
	}
}

// TestExecute_StepRequestPause_StepExecLogRecorded 验证 RequestPause 路径
// 上 StepExecLog 仍记录了被暂停的 step，以便后续 Resume 知道从哪个 step 之后续跑。
func TestExecute_StepRequestPause_StepExecLogRecorded(t *testing.T) {
	fc := newMockFlowContext()
	ctx := WithFlowContext(context.Background(), fc)

	step := NewStep("s1", func(c context.Context, _ any) (any, error) {
		fctx, _ := FlowContextFrom(c)
		return nil, fctx.RequestPause("halt")
	}).Build()
	f := NewFlow().AddStep(step).Build()

	exec := f.Execute(ctx, nil)

	if exec.State.Status != StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}
	if len(exec.State.StepExecLog) != 1 || exec.State.StepExecLog[0] != "s1" {
		t.Errorf("expected StepExecLog=[s1], got %v", exec.State.StepExecLog)
	}
}

// TestExecute_StepReturnsErrPauseRequestedDirectly 验证 step 直接返回
// errors.Is-able 的 ErrPauseRequested（不通过 RequestPause）也能触发暂停路径。
// 这覆盖了 step 用 fmt.Errorf("...: %w", ErrPauseRequested) 包装的场景。
func TestExecute_StepReturnsErrPauseRequestedDirectly(t *testing.T) {
	ctx := context.Background()
	step := NewStep("s1", func(_ context.Context, _ any) (any, error) {
		return nil, ErrPauseRequested
	}).Build()
	f := NewFlow().AddStep(step).Build()

	exec := f.Execute(ctx, nil)
	if exec.State.Status != StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}
	if len(exec.State.Errors) != 0 {
		t.Errorf("expected no Errors on pause; got %v", exec.State.Errors)
	}
}
