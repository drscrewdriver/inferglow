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
	"strings"
	"testing"
)

// mockFlowContext 是 FlowContext 接口的测试替身。
// 记录 ExecuteAction 被调用时的 name 和 params，便于断言；
// 其余方法返回零值或写入 values map。
type mockFlowContext struct {
	actionName   string
	actionParams map[string]any
	actionResult any
	actionErr    error

	agentResult string
	agentErr    error

	values map[string]any
}

func newMockFlowContext() *mockFlowContext {
	return &mockFlowContext{
		values: make(map[string]any),
	}
}

func (m *mockFlowContext) ExecuteAction(_ context.Context, name string, params map[string]any) (any, error) {
	m.actionName = name
	// 拷贝 params，避免引用外部 map 引发后续修改干扰断言
	m.actionParams = make(map[string]any, len(params))
	for k, v := range params {
		m.actionParams[k] = v
	}
	return m.actionResult, m.actionErr
}

func (m *mockFlowContext) GenerateModel(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}

func (m *mockFlowContext) SessionHistory() []map[string]any { return nil }

func (m *mockFlowContext) AppendSession(_ string, _ any) {}

func (m *mockFlowContext) AuditAppend(_, _ string, _, _ any) {}

func (m *mockFlowContext) SetValue(key string, value any) {
	if m.values == nil {
		m.values = make(map[string]any)
	}
	m.values[key] = value
}

func (m *mockFlowContext) GetValue(key string) (any, bool) {
	v, ok := m.values[key]
	return v, ok
}

// StartSpan 在 mock 上返回 no-op Span（mockFlowContext 不持有 tracer）。
func (m *mockFlowContext) StartSpan(ctx context.Context, _ SpanKind, _ string) (context.Context, Span) {
	return ctx, noopSpan{}
}

// MaskInput 在 mock 上不做任何脱敏。
func (m *mockFlowContext) MaskInput(input string) string { return input }

// CheckOutput 在 mock 上不做任何校验。
func (m *mockFlowContext) CheckOutput(_ string) error { return nil }

// RequestPause 在 mock 上返回 ErrPauseRequested，与真实 flowContextImpl 一致。
func (m *mockFlowContext) RequestPause(_ string) error { return ErrPauseRequested }

// RunAgent 在 mock 上返回预设的 agentResult / agentErr。
func (m *mockFlowContext) RunAgent(_ context.Context, _ string, _ string, _ *AgentRunOptions) (string, error) {
	return m.agentResult, m.agentErr
}

// RunAgentParallel 在 mock 上顺序调用 RunAgent，返回与 agents 等长的结果。
func (m *mockFlowContext) RunAgentParallel(_ context.Context, agents []AgentSubTask) ([]string, error) {
	results := make([]string, len(agents))
	for i := range agents {
		results[i] = m.agentResult
		if m.agentErr != nil {
			return nil, m.agentErr
		}
	}
	return results, nil
}

// ============================================================================
// FlowContext context 注入 / 提取
// ============================================================================

// TestWithFlowContext_Roundtrip 验证 WithFlowContext 注入后 FlowContextFrom
// 能取回同一个 FlowContext 实例（指针相等）。
func TestWithFlowContext_Roundtrip(t *testing.T) {
	fc := newMockFlowContext()
	ctx := WithFlowContext(context.Background(), fc)

	got, ok := FlowContextFrom(ctx)
	if !ok {
		t.Fatal("FlowContextFrom returned ok=false; expected true")
	}
	if got != fc {
		t.Errorf("extracted FlowContext != injected; got %p, want %p", got, fc)
	}
}

// TestFlowContextFrom_Empty 验证未注入时 FlowContextFrom 返回 (nil, false)。
func TestFlowContextFrom_Empty(t *testing.T) {
	got, ok := FlowContextFrom(context.Background())
	if ok {
		t.Errorf("expected ok=false on plain context.Background(), got true")
	}
	if got != nil {
		t.Errorf("expected nil FlowContext on plain context.Background(), got %v", got)
	}
}

// TestFlowContext_InStepFunc 验证 Flow.Execute 会把注入的 FlowContext
// 透传到 StepFunc 中：StepFunc 内调用 FlowContextFrom 应能取回同一个 fc。
func TestFlowContext_InStepFunc(t *testing.T) {
	fc := newMockFlowContext()
	ctx := WithFlowContext(context.Background(), fc)

	var seen FlowContext
	var seenOK bool
	step := NewStep("probe", func(c context.Context, _ any) (any, error) {
		seen, seenOK = FlowContextFrom(c)
		return "done", nil
	}).Build()

	flow := NewFlow().AddStep(step).Build()
	exec := flow.Execute(ctx, nil)

	if exec.State.Status != StatusCompleted {
		t.Fatalf("flow status = %q, want %q; errors=%v",
			exec.State.Status, StatusCompleted, exec.State.Errors)
	}
	if !seenOK {
		t.Fatal("StepFunc did not observe FlowContext (ok=false)")
	}
	if seen != fc {
		t.Errorf("StepFunc observed different FlowContext; got %p, want %p", seen, fc)
	}
}

// ============================================================================
// ActionOperatorHandler
// ============================================================================

// TestActionOperator_NoFlowContext 验证 OperatorContext 中未注入 FlowContext
// 时 ActionOperatorHandler.Execute 返回包含 "requires FlowContext" 的错误。
func TestActionOperator_NoFlowContext(t *testing.T) {
	oc := &OperatorContext{
		Ctx: context.Background(), // 无 FlowContext
		Operator: &Operator{
			ID:   "op-no-fc",
			Kind: OpAction,
			Options: map[string]any{
				"action_name": "search",
			},
		},
		Input: nil,
	}

	h := &ActionOperatorHandler{}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error when FlowContext missing, got nil")
	}
	if !strings.Contains(err.Error(), "requires FlowContext") {
		t.Errorf("error message %q does not contain \"requires FlowContext\"", err.Error())
	}
}

// TestActionOperator_Success 验证 ActionOperatorHandler 在 FlowContext 已注入
// 时能正确调用 Action：mock 记录到的 name 应等于配置的 action_name。
func TestActionOperator_Success(t *testing.T) {
	fc := newMockFlowContext()
	fc.actionResult = "ok-result"

	ctx := WithFlowContext(context.Background(), fc)

	oc := &OperatorContext{
		Ctx: ctx,
		Operator: &Operator{
			ID:   "op-ok",
			Kind: OpAction,
			Options: map[string]any{
				"action_name":   "search",
				"action_params": nil,
			},
		},
		Input: nil,
	}

	h := &ActionOperatorHandler{}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if out != fc.actionResult {
		t.Errorf("output = %v, want %v", out, fc.actionResult)
	}
	if fc.actionName != "search" {
		t.Errorf("mock recorded actionName = %q, want \"search\"", fc.actionName)
	}
}

// TestActionOperator_ParamsMerge 验证 action_params（静态）与 Input（动态）合并逻辑：
// Input 中的同名键应覆盖静态参数，最终 mock 收到合并后的 params。
func TestActionOperator_ParamsMerge(t *testing.T) {
	fc := newMockFlowContext()
	ctx := WithFlowContext(context.Background(), fc)

	oc := &OperatorContext{
		Ctx: ctx,
		Operator: &Operator{
			ID:   "op-merge",
			Kind: OpAction,
			Options: map[string]any{
				"action_name": "search",
				"action_params": map[string]any{
					"limit": 10,
					"lang":  "en",
				},
			},
		},
		// lang 应覆盖为 "zh"；query 为 Input 独有键。
		Input: map[string]any{
			"query": "hello",
			"lang":  "zh",
		},
	}

	h := &ActionOperatorHandler{}
	_, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	want := map[string]any{
		"limit": 10,
		"lang":  "zh",
		"query": "hello",
	}
	if fc.actionParams == nil {
		t.Fatal("mock did not record any params")
	}
	if len(fc.actionParams) != len(want) {
		t.Errorf("recorded params size = %d, want %d (got=%v)",
			len(fc.actionParams), len(want), fc.actionParams)
	}
	for k, wantV := range want {
		gotV, ok := fc.actionParams[k]
		if !ok {
			t.Errorf("missing key %q in recorded params; got=%v", k, fc.actionParams)
			continue
		}
		if gotV != wantV {
			t.Errorf("params[%q] = %v, want %v", k, gotV, wantV)
		}
	}
}
