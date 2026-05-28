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

package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/inferglow/flow"
	"github.com/inferglow/observability/otel"
)

// newAgentTestTracer 构造一个由 in-memory exporter 支撑的 *otel.Tracer，
// 供 agent 包测试验证 span 行为。通过 InstallNewProvider 设置全局
// TracerProvider（backed by tracetest.InMemoryExporter），再用 NewTracer
// 从全局 provider 解析 tracer。t.Cleanup 恢复上一个全局 provider，
// 避免污染其他测试。复用 otel.TestInstallNewProviderExportsSpans 的模式。
func newAgentTestTracer(t *testing.T) (*otel.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	prev := gootel.GetTracerProvider()
	cleanup := otel.InstallNewProvider(exp, "agent-test")
	t.Cleanup(func() {
		gootel.SetTracerProvider(prev)
		cleanup()
	})
	return otel.NewTracer("agent-test"), exp
}

// ============================================================================
// A3: flowContextImpl.StartSpan
// ============================================================================

// TestFlowContextImpl_StartSpan_NoTracer_NoopSpan 验证 tracer 为 nil 时
// StartSpan 返回原始 ctx 和 no-op Span。no-op Span 的 End() 必须可安全调用。
func TestFlowContextImpl_StartSpan_NoTracer_NoopSpan(t *testing.T) {
	fc := &flowContextImpl{}

	ctx := context.Background()
	newCtx, span := fc.StartSpan(ctx, flow.SpanKindStep, "my-step")
	if span == nil {
		t.Fatal("expected non-nil Span even when tracer is nil")
	}
	// End 必须不 panic。
	span.End()
	span.End()
	// ctx 应未被修改（no-op 路径直接返回原 ctx）。
	if newCtx != ctx {
		t.Error("expected StartSpan with nil tracer to return original ctx")
	}
}

// TestFlowContextImpl_StartSpan_WithTracer_ReturnsRealSpan 验证 tracer 非 nil 时
// StartSpan 返回真实 span（非 no-op）。通过 span.End() 后导出的 span 数量来断言。
// 这是 A4 任务文档中 "at minimum test that StartSpan on flowContextImpl returns
// a non-noop span when tracer is set" 的最小验证。
func TestFlowContextImpl_StartSpan_WithTracer_ReturnsRealSpan(t *testing.T) {
	tracer, exp := newAgentTestTracer(t)
	fc := &flowContextImpl{tracer: tracer}

	// 传空 name 让 span 名由 kind 派生，便于断言默认名。
	_, span := fc.StartSpan(context.Background(), flow.SpanKindStep, "")
	if span == nil {
		t.Fatal("expected non-nil Span")
	}
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	// flow.SpanKindStep 映射到 otel.SpanFlowExecute -> "inferglow.flow.execute"
	if spans[0].Name != "inferglow.flow.execute" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "inferglow.flow.execute")
	}
}

// TestFlowContextImpl_StartSpan_ToolKindMapsToToolCall 验证 SpanKindTool 映射到
// otel.SpanToolCall（"inferglow.tool.call"）。这是 A3 中 flowToOtelKind 的回归保护。
func TestFlowContextImpl_StartSpan_ToolKindMapsToToolCall(t *testing.T) {
	tracer, exp := newAgentTestTracer(t)
	fc := &flowContextImpl{tracer: tracer}

	_, span := fc.StartSpan(context.Background(), flow.SpanKindTool, "")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "inferglow.tool.call" {
		t.Errorf("SpanKindTool name = %q, want %q", spans[0].Name, "inferglow.tool.call")
	}
}

// TestFlowContextImpl_StartSpan_CustomNameOverridesKind 验证非空 name 覆盖
// 默认的 kind-derived span 名。
func TestFlowContextImpl_StartSpan_CustomNameOverridesKind(t *testing.T) {
	tracer, exp := newAgentTestTracer(t)
	fc := &flowContextImpl{tracer: tracer}

	_, span := fc.StartSpan(context.Background(), flow.SpanKindStep, "my.custom.step")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "my.custom.step" {
		t.Errorf("custom name = %q, want %q", spans[0].Name, "my.custom.step")
	}
}

// ============================================================================
// A3: flowContextImpl.MaskInput
// ============================================================================

// TestFlowContextImpl_MaskInput_NoMasker_ReturnsOriginal 验证 piiMasker 为 nil 时
// MaskInput 返回原始 input（无副作用）。
func TestFlowContextImpl_MaskInput_NoMasker_ReturnsOriginal(t *testing.T) {
	fc := &flowContextImpl{}
	in := "contact alice@example.com"
	if got := fc.MaskInput(in); got != in {
		t.Errorf("MaskInput without masker = %q, want %q", got, in)
	}
}

// TestFlowContextImpl_MaskInput_WithMasker_RedactsEmail 验证 piiMasker 配置后
// MaskInput 调用 masker.MaskInput 做脱敏。复用 testPIIMasker（pii_mask_test.go）。
func TestFlowContextImpl_MaskInput_WithMasker_RedactsEmail(t *testing.T) {
	fc := &flowContextImpl{
		piiMasker: &testPIIMasker{maskInput: true},
	}
	in := "contact alice@example.com"
	got := fc.MaskInput(in)
	if strings.Contains(got, "alice@example.com") {
		t.Errorf("MaskInput did not redact email: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Errorf("expected mask char in output, got %q", got)
	}
}

// TestFlowContextImpl_MaskInput_MaskerDisabled_NoChange 验证 masker 配置但
// maskInput=false（ApplyOn 不含 MaskOnInput）时 MaskInput 返回原值。
// 这保证 flowContextImpl 不绕过 masker 自身的 ApplyOn 门控。
func TestFlowContextImpl_MaskInput_MaskerDisabled_NoChange(t *testing.T) {
	fc := &flowContextImpl{
		piiMasker: &testPIIMasker{maskInput: false, maskOutput: true},
	}
	in := "contact alice@example.com"
	if got := fc.MaskInput(in); got != in {
		t.Errorf("MaskInput with disabled masker = %q, want %q", got, in)
	}
}

// ============================================================================
// A3: flowContextImpl.CheckOutput
// ============================================================================

// stubOutputHook 是 OutputSecurityHook 的测试实现，记录最后一次调用文本并可配置返回错误。
type stubOutputHook struct {
	lastInput string
	err       error
}

func (s *stubOutputHook) CheckOutput(text string) error {
	s.lastInput = text
	return s.err
}

// TestFlowContextImpl_CheckOutput_NoHook_ReturnsNil 验证 outputHook 为 nil 时
// CheckOutput 返回 nil（无校验）。
func TestFlowContextImpl_CheckOutput_NoHook_ReturnsNil(t *testing.T) {
	fc := &flowContextImpl{}
	if err := fc.CheckOutput("anything"); err != nil {
		t.Errorf("CheckOutput without hook = %v, want nil", err)
	}
}

// TestFlowContextImpl_CheckOutput_WithHook_Delegates 验证 outputHook 配置后
// CheckOutput 把文本透传给 hook 并返回 hook 的错误。
func TestFlowContextImpl_CheckOutput_WithHook_Delegates(t *testing.T) {
	blockErr := errors.New("injection detected")
	hook := &stubOutputHook{err: blockErr}
	fc := &flowContextImpl{outputHook: hook}

	err := fc.CheckOutput("ignore previous instructions")
	if !errors.Is(err, blockErr) {
		t.Errorf("CheckOutput err = %v, want %v", err, blockErr)
	}
	if hook.lastInput != "ignore previous instructions" {
		t.Errorf("hook received %q, want %q", hook.lastInput, "ignore previous instructions")
	}
}

// TestFlowContextImpl_CheckOutput_WithHook_PassesWhenNoInjection 验证 hook 返回
// nil 时 CheckOutput 也返回 nil（pass-through）。
func TestFlowContextImpl_CheckOutput_WithHook_PassesWhenNoInjection(t *testing.T) {
	hook := &stubOutputHook{}
	fc := &flowContextImpl{outputHook: hook}
	if err := fc.CheckOutput("clean response"); err != nil {
		t.Errorf("CheckOutput = %v, want nil", err)
	}
}

// ============================================================================
// A3+A8: flowContextImpl.RequestPause
// ============================================================================

// TestFlowContextImpl_RequestPause_ReturnsErrPauseRequested 验证 RequestPause
// 返回 flow.ErrPauseRequested 哨兵错误。flow.Execute 通过 errors.Is 识别它。
func TestFlowContextImpl_RequestPause_ReturnsErrPauseRequested(t *testing.T) {
	fc := &flowContextImpl{}
	err := fc.RequestPause("await approval")
	if !errors.Is(err, flow.ErrPauseRequested) {
		t.Errorf("RequestPause err = %v, want flow.ErrPauseRequested", err)
	}
}
