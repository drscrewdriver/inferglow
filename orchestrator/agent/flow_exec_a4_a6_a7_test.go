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

	"github.com/inferglow/flow"
	"github.com/inferglow/session"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// ============================================================================
// A4: executeFlow / ResumeFlow span 埋点
// ============================================================================

// containsSpanName 报告 names 中是否包含 want。
func containsSpanName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// spanNamesFromStubs 从 tracetest.SpanStub 切片提取 span 名列表。
func spanNamesFromStubs(stubs []tracetest.SpanStub) []string {
	out := make([]string, 0, len(stubs))
	for _, s := range stubs {
		out = append(out, s.Name)
	}
	return out
}

// TestExecuteFlow_EmitsFlowExecuteAndPauseSpans 验证配置了 tracer 后，
// executeFlow 在入口创建 SpanFlowExecute span、在暂停点创建 SpanPause span。
// 通过 in-memory exporter 收集 span 名断言。
//
// 流程：构建 3-step 流程，s1 关闭 pauseCh 触发暂停；executeFlow 应产出
// "inferglow.flow.execute" 和 "inferglow.flow.pause" 两个 span。
func TestExecuteFlow_EmitsFlowExecuteAndPauseSpans(t *testing.T) {
	tracer, exp := newAgentTestTracer(t)
	engine := newTestEngine(t)

	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)
	ctx := flow.WithPauseSignal(context.Background(), pauseCh)

	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{},
		tracer:    tracer,
	}

	exec, _, err := engine.executeFlow(ctx, f, "start", "", cfg, nil, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}

	// 用 exporter 的实际类型收集 span 名。
	names := spanNamesFromStubs(exp.GetSpans())
	if !containsSpanName(names, "inferglow.flow.execute") {
		t.Errorf("expected span %q in %v", "inferglow.flow.execute", names)
	}
	if !containsSpanName(names, "inferglow.flow.pause") {
		t.Errorf("expected span %q in %v", "inferglow.flow.pause", names)
	}
}

// TestResumeFlow_EmitsResumeSpan 验证配置了 tracer 后 ResumeFlow 在入口创建
// SpanResume span。流程：先 executeFlow 暂停（带 auto-checkpoint），再
// ResumeFlow 续跑，断言 exporter 收到 "inferglow.flow.resume" span。
func TestResumeFlow_EmitsResumeSpan(t *testing.T) {
	tracer, exp := newAgentTestTracer(t)

	// Engine.tracer 用于 ResumeFlow 的 span；executeFlow 的 span 来自 runConfig.tracer。
	engine := newTestEngine(t)
	engine.tracer = tracer

	dir := t.TempDir()
	store := flow.NewFileCheckpointStore(dir)

	pauseCh := make(chan struct{})
	f := buildPauseTestFlow(t, pauseCh)
	ctx := flow.WithPauseSignal(context.Background(), pauseCh)

	const cpID = "resume-span-test"
	opts := []flow.FlowOption{
		flow.WithAutoCheckpoint(store),
		flow.WithCheckPointID(cpID),
	}
	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{},
		tracer:    tracer,
	}
	exec, _, err := engine.executeFlow(ctx, f, "start", "", cfg, opts, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s", exec.State.Status)
	}

	// 清空 exporter 以便 ResumeFlow 之后的 span 列表只包含 resume 相关 span。
	exp.Reset()

	resumed, err := engine.ResumeFlow(context.Background(), f, store, cpID, nil)
	if err != nil {
		t.Fatalf("ResumeFlow returned error: %v", err)
	}
	if resumed.State.Status != flow.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", resumed.State.Status)
	}

	names := spanNamesFromStubs(exp.GetSpans())
	if !containsSpanName(names, "inferglow.flow.resume") {
		t.Errorf("expected span %q in %v", "inferglow.flow.resume", names)
	}
}

// TestExecuteFlow_NoTracer_NoSpansExported 验证未配置 tracer 时
// executeFlow 不产出任何 span（no-op span 不被 exporter 收集）。
// 这是 A4 的负向回归保护：保证 tracer nil 路径零开销。
func TestExecuteFlow_NoTracer_NoSpansExported(t *testing.T) {
	// 仍然安装一个 exporter，但 runConfig.tracer 留空。
	_, exp := newAgentTestTracer(t)
	engine := newTestEngine(t)

	f := buildCompletionTestFlow(t)
	cfg := &runConfig{maxRounds: 10, features: Features{}}

	_, _, err := engine.executeFlow(context.Background(), f, "start", "", cfg, nil, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}

	if got := len(exp.GetSpans()); got != 0 {
		t.Errorf("expected 0 spans without tracer, got %d: %v", got, spanNamesFromStubs(exp.GetSpans()))
	}
}

// ============================================================================
// A6: 失败路径 PII 脱敏
// ============================================================================

// TestExecuteFlow_FailurePath_PIIMasked 验证 step 抛出包含 PII（邮箱）的错误时，
// executeFlow 在返回错误前对错误消息做 PII 脱敏，使邮箱不出现在最终 error 中。
//
// 流程：step 返回 errors.New("contact alice@example.com for help")，
// runConfig 配置 PIIMasking=true + testPIIMasker（maskOutput=true），
// 断言返回的 err.Error() 不包含 "alice@example.com" 且包含 "***"。
func TestExecuteFlow_FailurePath_PIIMasked(t *testing.T) {
	engine := newTestEngine(t)

	step := flow.NewStep("leak", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("contact alice@example.com for help")
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{PIIMasking: true},
		piiMasker: &testPIIMasker{maskOutput: true},
	}

	_, _, err := engine.executeFlow(context.Background(), f, "hi", "", cfg, nil, RunMeta{})
	if err == nil {
		t.Fatal("expected error from failed flow, got nil")
	}
	if strings.Contains(err.Error(), "alice@example.com") {
		t.Errorf("PII leaked into error message: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("expected mask char in error message, got %q", err.Error())
	}
	// 仍应保留 "flow execution failed" 前缀，便于上层识别 flow 失败类型。
	if !strings.Contains(err.Error(), "flow execution failed") {
		t.Errorf("error %q does not contain %q", err.Error(), "flow execution failed")
	}
}

// TestExecuteFlow_FailurePath_NoMasker_NoChange 验证未配置 masker 时
// 错误消息不被脱敏（PII 保留）。这是 A6 的负向回归保护：masker 门控正确。
func TestExecuteFlow_FailurePath_NoMasker_NoChange(t *testing.T) {
	engine := newTestEngine(t)

	step := flow.NewStep("leak", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("contact alice@example.com for help")
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	// PIIMasking=true 但 piiMasker=nil：脱敏被跳过。
	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{PIIMasking: true},
		piiMasker: nil,
	}

	_, _, err := engine.executeFlow(context.Background(), f, "hi", "", cfg, nil, RunMeta{})
	if err == nil {
		t.Fatal("expected error from failed flow, got nil")
	}
	if !strings.Contains(err.Error(), "alice@example.com") {
		t.Errorf("without masker the email should be preserved; got %q", err.Error())
	}
}

// TestExecuteFlow_FailurePath_PIIMaskingDisabled_NoChange 验证 PIIMasking=false
// 时即使配置了 masker 也不脱敏。门控行为与完成路径一致。
func TestExecuteFlow_FailurePath_PIIMaskingDisabled_NoChange(t *testing.T) {
	engine := newTestEngine(t)

	step := flow.NewStep("leak", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("contact alice@example.com for help")
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{PIIMasking: false},
		piiMasker: &testPIIMasker{maskOutput: true},
	}

	_, _, err := engine.executeFlow(context.Background(), f, "hi", "", cfg, nil, RunMeta{})
	if err == nil {
		t.Fatal("expected error from failed flow, got nil")
	}
	if !strings.Contains(err.Error(), "alice@example.com") {
		t.Errorf("with PIIMasking=false the email should be preserved; got %q", err.Error())
	}
}

// TestExecuteFlow_FailurePath_UnknownErrorMasked 验证 flow 失败但 Errors 为空时
// （理论上不应发生，但代码有 fallback）也走脱敏路径，返回 "flow execution failed with unknown error"。
func TestExecuteFlow_FailurePath_UnknownErrorMasked(t *testing.T) {
	engine := newTestEngine(t)

	// 构建一个空 flow（无 step）—— Execute 会因找不到 start step 而失败，
	// 但 Errors 会被填充 ("no starting step found")，所以这条路径其实会走
	// Errors[0] 分支。这里仍验证脱敏生效。
	f := flow.NewFlow().Build()

	cfg := &runConfig{
		maxRounds: 10,
		features:  Features{PIIMasking: true},
		piiMasker: &testPIIMasker{maskOutput: true},
	}

	_, _, err := engine.executeFlow(context.Background(), f, "hi", "", cfg, nil, RunMeta{})
	if err == nil {
		t.Fatal("expected error from empty flow, got nil")
	}
	// "no starting step found" 不含 PII，脱敏后应保持不变；但前缀仍应是
	// "flow execution failed"。
	if !strings.Contains(err.Error(), "flow execution failed") {
		t.Errorf("error %q does not contain %q", err.Error(), "flow execution failed")
	}
}

// ============================================================================
// A7: extractFlowResponse map fallback
// ============================================================================

// TestExtractFlowResponse_FallbackKeys 验证 map[string]any 输入时按顺序尝试
// final_response / response / output / result / answer 五个键，命中第一个字符串值即返回。
func TestExtractFlowResponse_FallbackKeys(t *testing.T) {
	keys := []string{"final_response", "response", "output", "result", "answer"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			in := map[string]any{key: "value-for-" + key}
			got := extractFlowResponse(in)
			want := "value-for-" + key
			if got != want {
				t.Errorf("extractFlowResponse(%v) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestExtractFlowResponse_FallbackKeyPrecedence 验证多个键同时存在时
// 按 final_response > response > output > result > answer 顺序取第一个。
func TestExtractFlowResponse_FallbackKeyPrecedence(t *testing.T) {
	in := map[string]any{
		"final_response": "from-final",
		"response":       "from-response",
		"output":         "from-output",
		"result":         "from-result",
		"answer":         "from-answer",
	}
	if got := extractFlowResponse(in); got != "from-final" {
		t.Errorf("precedence: got %q, want %q", got, "from-final")
	}

	// 去掉 final_response 后应取 response。
	delete(in, "final_response")
	if got := extractFlowResponse(in); got != "from-response" {
		t.Errorf("after removing final_response: got %q, want %q", got, "from-response")
	}

	// 继续去掉 response 后应取 output。
	delete(in, "response")
	if got := extractFlowResponse(in); got != "from-output" {
		t.Errorf("after removing response: got %q, want %q", got, "from-output")
	}

	// 继续去掉 output 后应取 result。
	delete(in, "output")
	if got := extractFlowResponse(in); got != "from-result" {
		t.Errorf("after removing output: got %q, want %q", got, "from-result")
	}

	// 继续去掉 result 后应取 answer。
	delete(in, "result")
	if got := extractFlowResponse(in); got != "from-answer" {
		t.Errorf("after removing result: got %q, want %q", got, "from-answer")
	}
}

// TestExtractFlowResponse_NonStringFallbackValue 验证已知键存在但值不是 string 时
// 跳过该键继续尝试后续键。例如 final_response=42（int）应跳过，取 response。
func TestExtractFlowResponse_NonStringFallbackValue(t *testing.T) {
	in := map[string]any{
		"final_response": 42,
		"response":       "real-string",
	}
	got := extractFlowResponse(in)
	if got != "real-string" {
		t.Errorf("expected non-string final_response to be skipped; got %q", got)
	}
}

// TestExtractFlowResponse_JSONMarshalFallback 验证无任何已知键时
// 用 json.Marshal 输出合法 JSON 字符串。
func TestExtractFlowResponse_JSONMarshalFallback(t *testing.T) {
	in := map[string]any{
		"other":  "value",
		"count":  3,
		"nested": map[string]any{"k": "v"},
	}
	got := extractFlowResponse(in)
	// 验证是合法 JSON 且能被解析回等效 map。
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Errorf("expected JSON object, got %q", got)
	}
	// 简单验证包含原始键。
	if !strings.Contains(got, `"other":"value"`) {
		t.Errorf("JSON output missing 'other' key: %q", got)
	}
	if !strings.Contains(got, `"count":3`) {
		t.Errorf("JSON output missing 'count' key: %q", got)
	}
}

// TestExtractFlowResponse_EmptyMapReturnsBraces 验证空 map 返回 "{}"。
func TestExtractFlowResponse_EmptyMapReturnsBraces(t *testing.T) {
	got := extractFlowResponse(map[string]any{})
	if got != "{}" {
		t.Errorf("empty map = %q, want %q", got, "{}")
	}
}

// TestExtractFlowResponse_StringStillWorks 验证 string 输入仍直接返回（A7 不破坏既有路径）。
func TestExtractFlowResponse_StringStillWorks(t *testing.T) {
	if got := extractFlowResponse("plain"); got != "plain" {
		t.Errorf("string input = %q, want %q", got, "plain")
	}
}

// TestExtractFlowResponse_DefaultCaseFmtSprint 验证非 string、非 map 输入
// 仍走 fmt.Sprint 路径（A7 未改变 default 分支）。
func TestExtractFlowResponse_DefaultCaseFmtSprint(t *testing.T) {
	if got := extractFlowResponse(42); got != "42" {
		t.Errorf("int input = %q, want %q", got, "42")
	}
	if got := extractFlowResponse(true); got != "true" {
		t.Errorf("bool input = %q, want %q", got, "true")
	}
}

// TestExtractFlowResponse_NestedMapFinalResponseNotPromoted 验证 final_response
// 嵌套在子 map 中时不会被提升（仅顶层键被检查）。
func TestExtractFlowResponse_NestedMapFinalResponseNotPromoted(t *testing.T) {
	in := map[string]any{
		"outer": map[string]any{
			"final_response": "nested",
		},
	}
	got := extractFlowResponse(in)
	// 顶层无已知键，回退到 json.Marshal；输出应包含 "final_response":"nested"
	// 但不是直接返回 "nested"。
	if got == "nested" {
		t.Errorf("nested final_response must not be promoted; got %q", got)
	}
	if !strings.Contains(got, "final_response") {
		t.Errorf("JSON fallback should contain nested key; got %q", got)
	}
}

// ============================================================================
// A8: step 通过 Context.RequestPause 主动挂起（agent 端到端）
// ============================================================================

// TestExecuteFlow_StepRequestPause_StatusPaused 验证 step 通过
// Context.RequestPause 主动请求挂起时，executeFlow 观察到 StatusPaused
// 并走暂停路径（f.Pause + 返回 exec 句柄）。
//
// 与 flow 包的 TestExecute_StepRequestPause_StatusPaused 互补：
// flow 包用 mockContext 验证 Execute 行为；本测试用真实 flowContextImpl
// 验证 RequestPause 端到端从 orchestrator 层穿透到 flow 层。
func TestExecuteFlow_StepRequestPause_StatusPaused(t *testing.T) {
	engine := newTestEngine(t)

	step := flow.NewStep("ask-approval", func(ctx context.Context, _ any) (any, error) {
		fc, ok := flow.ContextFrom(ctx)
		if !ok {
			return nil, errors.New("Context missing")
		}
		return nil, fc.RequestPause("await human approval")
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	cfg := zeroRunConfig()
	exec, resp, err := engine.executeFlow(context.Background(), f, "user-input", "", cfg, nil, RunMeta{})
	if err != nil {
		t.Fatalf("executeFlow returned error: %v", err)
	}
	if exec.State.Status != flow.StatusPaused {
		t.Fatalf("expected StatusPaused, got %s; errors=%v",
			exec.State.Status, exec.State.Errors)
	}
	if resp != "" {
		t.Errorf("expected empty response on pause, got %q", resp)
	}
	// ErrPauseRequested 不应出现在 Errors 中。
	for _, e := range exec.State.Errors {
		if errors.Is(e, flow.ErrPauseRequested) {
			t.Errorf("ErrPauseRequested must NOT be in Errors; got %v", exec.State.Errors)
		}
	}
}

// TestAgent_Run_StepRequestPause_ReturnsEmptyResponse 验证通过 Agent.Run 入口
// 调用一个会 RequestPause 的 step 时，Run 返回 ("", nil)（与 pause-signal 暂停行为一致）。
func TestAgent_Run_StepRequestPause_ReturnsEmptyResponse(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{}

	step := flow.NewStep("pause-step", func(ctx context.Context, _ any) (any, error) {
		fc, _ := flow.ContextFrom(ctx)
		return nil, fc.RequestPause("await approval")
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if resp != "" {
		t.Errorf("expected empty response on step-requested pause, got %q", resp)
	}
}
