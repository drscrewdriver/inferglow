package flow

import (
	"context"
	"testing"
	"time"
)

// TestRun_SignalNetNotNil verifies that TriggerFlow.Run creates a SignalNet
// and passes it to operator handlers via OperatorContext.
//
// F-HIGH-5: TriggerFlow.Run 不传递 SignalNet 到 OperatorContext.
// 修复：Run 时创建 SignalNet 并注入 OperatorContext.SignalNet.
func TestRun_SignalNetNotNil(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	// SignalGateHandler 在 oc.SignalNet == nil 时返回 "nil signal net" 错误.
	// 不设置 required_signals, 这样 SignalNet 非 nil 时直接透传 input.
	f.AddOperator(&Operator{ID: "op-1", Kind: OpSignalGate, Name: "gate"})
	out, err := f.Run("input_data")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if out != "input_data" {
		t.Errorf("output = %v, want input_data", out)
	}
}

// TestRun_EmitSignalNotNil verifies that TriggerFlow.Run creates an EmitSignal
// function and passes it to operator handlers via OperatorContext.
//
// F-HIGH-5: TriggerFlow.Run 不传递 EmitSignal 到 OperatorContext.
// 修复：Run 时创建 EmitSignal (将信号接受到 SignalNet) 并注入 OperatorContext.EmitSignal.
//
// 流程：
//   1. OpChunk (name "c1")：发射 "Chunk[c1]" 信号
//   2. OpSignalGate (required_signals: ["Chunk[c1]"])：要求 "Chunk[c1]" 已被接受
//
// 修复前：EmitSignal == nil，ChunkHandler 不发射信号，SignalGate 看不到信号，
//         返回 (nil, nil)，最终 output == ""
// 修复后：EmitSignal != nil，ChunkHandler 发射并接受信号，SignalGate 看到信号已接受，
//         透传 input，最终 output == "input_data"
func TestRun_EmitSignalNotNil(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})
	f.AddOperator(&Operator{
		ID:   "op-2",
		Kind: OpSignalGate,
		Name: "gate",
		Options: map[string]any{
			"required_signals": []string{"Chunk[c1]"},
		},
	})
	out, err := f.Run("input_data")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if out != "input_data" {
		t.Errorf("output = %v, want input_data (EmitSignal should accept Chunk[c1] signal)", out)
	}
}

// TestWrapOperatorHandlerAsHandler_PassesContext verifies that the wrapped
// handler receives an OperatorContext with the outer context, SignalNet, and
// EmitSignal propagated from TriggerFlowRuntimeData, instead of a hardcoded
// context.Background().
//
// F-HIGH-4 / BUG-17: wrapOperatorHandlerAsHandler 使用 context.Background(),
// 不透传外层 context 和 SignalNet.
// 修复：从 TriggerFlowRuntimeData 读取 Ctx/SignalNet/EmitSignal 并注入 OperatorContext.
func TestWrapOperatorHandlerAsHandler_PassesContext(t *testing.T) {
	// 使用一个自定义 OperatorHandler 来捕获 OperatorContext
	captured := &ctxCapturingHandler{}
	op := &Operator{ID: "op-x", Kind: OperatorKind("capturing"), Name: "cap"}

	wrapped := wrapOperatorHandlerAsHandler(captured, op)

	// 准备外层 context 和 SignalNet
	outerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outerSignalNet := NewSignalNet()
	emitCalled := false
	emitFn := func(s Signal) { emitCalled = true }

	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{},
		FlowData:    map[string]any{},
		Signal:      &Signal{ID: "outer", Value: "outer-input"},
		Result:      "result-value",
		Ctx:         outerCtx,
		SignalNet:   outerSignalNet,
		EmitSignal:  emitFn,
	}

	_, err := wrapped(rd)
	if err != nil {
		t.Fatalf("wrapped handler returned error: %v", err)
	}

	if captured.ctx == nil {
		t.Fatal("captured OperatorContext.Ctx is nil")
	}
	// 修复前：ctx == context.Background()，不会响应取消
	// 修复后：ctx 应响应外层取消
	select {
	case <-captured.ctx.Done():
		t.Error("captured ctx should not be cancelled when outer ctx is not cancelled")
	default:
		// expected
	}

	// 取消外层 ctx，验证 captured.ctx 能感知到
	cancel()
	select {
	case <-captured.ctx.Done():
		// expected: captured ctx should be cancelled when outer ctx is cancelled
	case <-time.After(100 * time.Millisecond):
		t.Error("captured ctx should be cancelled when outer ctx is cancelled (context not propagated)")
	}

	if captured.signalNet == nil {
		t.Error("captured OperatorContext.SignalNet is nil (should be propagated from TriggerFlowRuntimeData)")
	} else if captured.signalNet != outerSignalNet {
		t.Error("captured OperatorContext.SignalNet should be the same instance as outerSignalNet")
	}

	if captured.emitSignal == nil {
		t.Error("captured OperatorContext.EmitSignal is nil (should be propagated from TriggerFlowRuntimeData)")
	} else {
		captured.emitSignal(Signal{ID: "test"})
		if !emitCalled {
			t.Error("captured EmitSignal should call outer emitFn when invoked")
		}
	}
}

// TestWrapOperatorHandlerAsHandler_DefaultsWhenNoContext verifies that the
// wrapped handler falls back to context.Background() when TriggerFlowRuntimeData
// does not carry Ctx/SignalNet/EmitSignal. This preserves backward compat.
func TestWrapOperatorHandlerAsHandler_DefaultsWhenNoContext(t *testing.T) {
	captured := &ctxCapturingHandler{}
	op := &Operator{ID: "op-y", Kind: OperatorKind("capturing"), Name: "cap2"}

	wrapped := wrapOperatorHandlerAsHandler(captured, op)

	rd := &TriggerFlowRuntimeData{
		RuntimeData: map[string]any{},
		FlowData:    map[string]any{},
		Signal:      &Signal{ID: "s", Value: "v"},
	}

	_, err := wrapped(rd)
	if err != nil {
		t.Fatalf("wrapped handler returned error: %v", err)
	}
	if captured.ctx == nil {
		t.Error("captured ctx should default to context.Background() when rd.Ctx is nil")
	}
}

// ctxCapturingHandler 捕获 OperatorContext 用于断言.
type ctxCapturingHandler struct {
	ctx        context.Context
	signalNet  *SignalNet
	emitSignal func(Signal)
}

func (h *ctxCapturingHandler) Kind() OperatorKind { return OperatorKind("capturing") }

func (h *ctxCapturingHandler) Execute(oc *OperatorContext) (any, error) {
	if oc != nil {
		h.ctx = oc.Ctx
		h.signalNet = oc.SignalNet
		h.emitSignal = oc.EmitSignal
	}
	return oc.Input, nil
}
