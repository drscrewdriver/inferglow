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
	"fmt"
	"testing"

	"github.com/inferglow/flow"
)

// stubFlowContext 是 agent_step 测试用的 FlowContext 替身。
// RunAgent 返回预设的 stubResponse / stubErr；
// RunAgentParallel 按 stubResponses 索引返回（长度不足时用 stubResponse 填充）。
type stubFlowContext struct {
	stubResponse  string
	stubResponses []string // 按索引返回，为空时用 stubResponse
	stubErr       error
	calls         []stubCall
}

type stubCall struct {
	userMessage  string
	systemPrompt string
	maxRounds    int
}

func (s *stubFlowContext) ExecuteAction(_ context.Context, _ string, _ map[string]any) (any, error) {
	return nil, nil
}
func (s *stubFlowContext) GenerateModel(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}
func (s *stubFlowContext) SessionHistory() []map[string]any { return nil }
func (s *stubFlowContext) AppendSession(_ string, _ any)    {}
func (s *stubFlowContext) AuditAppend(_, _ string, _, _ any) {}
func (s *stubFlowContext) SetValue(_ string, _ any)         {}
func (s *stubFlowContext) GetValue(_ string) (any, bool)    { return nil, false }
func (s *stubFlowContext) StartSpan(ctx context.Context, _ flow.SpanKind, _ string) (context.Context, flow.Span) {
	return ctx, flow.NoopSpan()
}
func (s *stubFlowContext) MaskInput(input string) string  { return input }
func (s *stubFlowContext) CheckOutput(_ string) error     { return nil }
func (s *stubFlowContext) RequestPause(_ string) error    { return flow.ErrPauseRequested }

func (s *stubFlowContext) RunAgent(_ context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
	mr := 10
	if opts != nil && opts.MaxRounds > 0 {
		mr = opts.MaxRounds
	}
	s.calls = append(s.calls, stubCall{userMessage: userMessage, systemPrompt: systemPrompt, maxRounds: mr})
	if s.stubErr != nil {
		return "", s.stubErr
	}
	return s.stubResponse, nil
}

func (s *stubFlowContext) RunAgentParallel(ctx context.Context, agents []flow.AgentSubTask) ([]string, error) {
	results := make([]string, len(agents))
	for i, sub := range agents {
		r, err := s.RunAgent(ctx, sub.UserMessage, sub.SystemPrompt, &flow.AgentRunOptions{
			MaxRounds:        sub.MaxRounds,
			SessionIsolation: true,
		})
		if err != nil {
			return nil, fmt.Errorf("parallel agent %q (index %d): %w", sub.Label, i, err)
		}
		if i < len(s.stubResponses) {
			results[i] = s.stubResponses[i]
		} else {
			results[i] = r
		}
	}
	return results, nil
}

// injectFlowContext 将 stubFlowContext 注入到 context 中。
func injectFlowContext(stub *stubFlowContext) context.Context {
	return flow.WithFlowContext(context.Background(), stub)
}

// ============================================================================
// extractInputString 单元测试
// ============================================================================

func TestExtractInputString_WithKey(t *testing.T) {
	input := map[string]any{"task": "fix bug", "extra": "data"}
	got := extractInputString(input, "task")
	if got != "fix bug" {
		t.Errorf("extractInputString(map, \"task\") = %q, want \"fix bug\"", got)
	}
}

func TestExtractInputString_NoKey(t *testing.T) {
	input := map[string]any{"task": "fix bug"}
	got := extractInputString(input, "")
	// 无 key 时 fmt.Sprint 整个 map
	if got == "" {
		t.Error("extractInputString(map, \"\") returned empty string")
	}
}

func TestExtractInputString_PlainString(t *testing.T) {
	got := extractInputString("hello world", "key")
	if got != "hello world" {
		t.Errorf("extractInputString(string, \"key\") = %q, want \"hello world\"", got)
	}
}

func TestExtractInputString_MissingKey(t *testing.T) {
	input := map[string]any{"task": "fix bug"}
	got := extractInputString(input, "missing_key")
	// 键不存在时回退到 fmt.Sprint(input)
	if got == "" {
		t.Error("extractInputString(map, \"missing_key\") returned empty string")
	}
}

// ============================================================================
// wrapOutput / copyInputMap 单元测试
// ============================================================================

func TestWrapOutput_EmptyKey(t *testing.T) {
	got := wrapOutput(map[string]any{"a": 1}, "", "result")
	if got != "result" {
		t.Errorf("wrapOutput(_, \"\", _) = %v, want \"result\"", got)
	}
}

func TestWrapOutput_WithKey(t *testing.T) {
	input := map[string]any{"a": 1, "b": 2}
	got := wrapOutput(input, "result", "value")
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("wrapOutput returned %T, want map[string]any", got)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Errorf("wrapOutput lost original keys: %v", m)
	}
	if m["result"] != "value" {
		t.Errorf("wrapOutput result = %v, want \"value\"", m["result"])
	}
}

func TestCopyInputMap_NonMap(t *testing.T) {
	got := copyInputMap("not a map")
	if got == nil {
		t.Fatal("copyInputMap(string) returned nil")
	}
	if len(got) != 0 {
		t.Errorf("copyInputMap(string) = %v, want empty map", got)
	}
}

// ============================================================================
// NewAgentStepFunc 集成测试
// ============================================================================

func TestNewAgentStepFunc_Basic(t *testing.T) {
	stub := &stubFlowContext{stubResponse: "code modified"}
	ctx := injectFlowContext(stub)

	fn := NewAgentStepFunc(AgentStepConfig{
		SystemPrompt: "You are a coder",
		MaxRounds:    5,
		InputKey:     "task",
		OutputKey:    "code",
	})

	input := map[string]any{"task": "fix the bug in foo.go"}
	out, err := fn(ctx, input)
	if err != nil {
		t.Fatalf("NewAgentStepFunc returned error: %v", err)
	}

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output is %T, want map[string]any", out)
	}
	if m["code"] != "code modified" {
		t.Errorf("output[\"code\"] = %v, want \"code modified\"", m["code"])
	}
	if m["task"] != "fix the bug in foo.go" {
		t.Errorf("output[\"task\"] = %v, want original input preserved", m["task"])
	}

	if len(stub.calls) != 1 {
		t.Fatalf("RunAgent called %d times, want 1", len(stub.calls))
	}
	if stub.calls[0].userMessage != "fix the bug in foo.go" {
		t.Errorf("RunAgent userMessage = %q, want \"fix the bug in foo.go\"", stub.calls[0].userMessage)
	}
	if stub.calls[0].maxRounds != 5 {
		t.Errorf("RunAgent maxRounds = %d, want 5", stub.calls[0].maxRounds)
	}
}

func TestNewAgentStepFunc_NoOutputKey(t *testing.T) {
	stub := &stubFlowContext{stubResponse: "raw result"}
	ctx := injectFlowContext(stub)

	fn := NewAgentStepFunc(AgentStepConfig{
		SystemPrompt: "Analyze",
		MaxRounds:    3,
	})

	out, err := fn(ctx, "analyze this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output is %T, want string", out)
	}
	if s != "raw result" {
		t.Errorf("output = %q, want \"raw result\"", s)
	}
}

func TestNewAgentStepFunc_NoFlowContext(t *testing.T) {
	fn := NewAgentStepFunc(AgentStepConfig{SystemPrompt: "test"})
	_, err := fn(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error when FlowContext missing")
	}
}

func TestNewAgentStepFunc_AgentError(t *testing.T) {
	stub := &stubFlowContext{stubErr: fmt.Errorf("model timeout")}
	ctx := injectFlowContext(stub)

	fn := NewAgentStepFunc(AgentStepConfig{SystemPrompt: "test"})
	_, err := fn(ctx, "input")
	if err == nil {
		t.Fatal("expected error when RunAgent fails")
	}
}

// ============================================================================
// NewParallelAgentStepFunc 集成测试
// ============================================================================

func TestNewParallelAgentStepFunc_Basic(t *testing.T) {
	stub := &stubFlowContext{
		stubResponses: []string{"review: LGTM", "test: all passed"},
	}
	ctx := injectFlowContext(stub)

	fn := NewParallelAgentStepFunc(ParallelAgentStepConfig{
		SubTasks: []SubTaskSpec{
			{Label: "reviewer", SystemPrompt: "Review", MaxRounds: 3, InputKey: "code", OutputKey: "review"},
			{Label: "tester", SystemPrompt: "Test", MaxRounds: 2, InputKey: "code", OutputKey: "test"},
		},
	})

	input := map[string]any{"code": "func foo() { return 42 }"}
	out, err := fn(ctx, input)
	if err != nil {
		t.Fatalf("NewParallelAgentStepFunc returned error: %v", err)
	}

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output is %T, want map[string]any", out)
	}
	if m["review"] != "review: LGTM" {
		t.Errorf("output[\"review\"] = %v, want \"review: LGTM\"", m["review"])
	}
	if m["test"] != "test: all passed" {
		t.Errorf("output[\"test\"] = %v, want \"test: all passed\"", m["test"])
	}
	if m["code"] != "func foo() { return 42 }" {
		t.Errorf("original input key \"code\" not preserved: %v", m["code"])
	}

	// 验证 RunAgent 被调用了 2 次（顺序降级执行）
	if len(stub.calls) != 2 {
		t.Errorf("RunAgent called %d times, want 2", len(stub.calls))
	}
}

func TestNewParallelAgentStepFunc_NoFlowContext(t *testing.T) {
	fn := NewParallelAgentStepFunc(ParallelAgentStepConfig{
		SubTasks: []SubTaskSpec{{Label: "x", SystemPrompt: "test"}},
	})
	_, err := fn(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error when FlowContext missing")
	}
}
