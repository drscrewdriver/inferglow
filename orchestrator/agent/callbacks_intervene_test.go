// Copyright 2026 Inferglow Authors
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
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// ---- 可干预钩子测试的共享辅助 ----

// interveneRecorder 线程安全地记录 fake 工具执行器收到的参数与调用次数。
// 工具在 dispatcher 的并发 goroutine 中执行，需要互斥保护。
type interveneRecorder struct {
	mu     sync.Mutex
	calls  int
	params map[string]any
}

func (r *interveneRecorder) record(params map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.params = params
}

func (r *interveneRecorder) snapshot() (int, map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.params
}

// historyText 汇总第 idx 轮模型请求的完整对话历史文本，用于断言工具结果
// 与附加上下文是否进入后续 LLM 输入。idx 越界时返回空串。
func historyText(reqs []*model.ModelRequest, idx int) string {
	if idx < 0 || idx >= len(reqs) {
		return ""
	}
	var b strings.Builder
	for _, m := range reqs[idx].ChatHistory {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// newInterveneEngine 构造最小 Engine：第一轮 LLM 返回 firstDelta 指定的
// 工具调用决策，之后每轮返回固定 response；requestFn 捕获每轮模型请求。
// 返回 engine 与捕获到的请求列表（按轮次）。
func newInterveneEngine(t *testing.T, firstDelta string, cb *AgentCallbacks, actions ...*action.Action) (*Engine, *[]*model.ModelRequest) {
	t.Helper()
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()
	for _, a := range actions {
		if err := actExt.Register(a); err != nil {
			t.Fatalf("failed to register action %q: %v", a.Name, err)
		}
	}

	var captured []*model.ModelRequest
	callCount := 0
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			cp := *req
			captured = append(captured, &cp)
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			callCount++
			if callCount == 1 {
				ch <- &model.StreamChunk{Delta: firstDelta, IsDone: true}
			} else {
				ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"done"}`, IsDone: true}
			}
			close(ch)
			return ch, nil
		},
	}

	engine := &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mockReq,
		callbacks: cb,
	}
	return engine, &captured
}

// runInterveneLoop 执行一次 executeLoop 并断言其以最终回复收尾。
func runInterveneLoop(t *testing.T, e *Engine) {
	t.Helper()
	decision, err := e.executeLoop(context.Background(), "Hi", 5, "")
	if err != nil {
		t.Fatalf("executeLoop returned error: %v", err)
	}
	if decision == nil {
		t.Fatal("executeLoop returned nil decision")
	}
	if decision.FinalResponse != "done" {
		t.Fatalf("FinalResponse = %q, want %q", decision.FinalResponse, "done")
	}
}

// ---- 场景一：PreToolCall 返回 Block ----

// TestIntervene_PreToolCallBlock 验证 PreToolCall 返回 Block 时：
//  1. 被阻断的工具不执行；
//  2. 该调用产出与 approval 拦截同形的 blocked 结果（OK=false、
//     Status="blocked"、Error=阻断原因），并触发 OnApprovalRequired；
//  3. 阻断原因随工具结果进入下一轮模型输入（模型可读）；
//  4. 同批其余工具照常执行。
func TestIntervene_PreToolCallBlock(t *testing.T) {
	var dangerous, safe interveneRecorder
	dangerousTool, err := action.New("dangerous", "a dangerous tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			dangerous.record(input)
			return "boom", nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}
	safeTool, err := action.New("safe", "a safe tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			safe.record(input)
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	var mu sync.Mutex
	var approvalEvents []string
	var postResults []*action.ActionResult
	cb := &AgentCallbacks{
		PreToolCall: func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
			if toolName == "dangerous" {
				// 阻断并给出模型可读的原因。
				return &ToolCallDecision{Block: true, BlockReason: "forbidden by policy"}
			}
			return nil // safe 工具放行
		},
		OnApprovalRequired: func(ctx context.Context, toolName, recordID string) {
			mu.Lock()
			approvalEvents = append(approvalEvents, toolName+":"+recordID)
			mu.Unlock()
		},
		PostToolCall: func(ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback {
			mu.Lock()
			postResults = append(postResults, result)
			mu.Unlock()
			return nil
		},
	}

	engine, captured := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"dangerous","params":{}},{"name":"safe","params":{}}]}`,
		cb, dangerousTool, safeTool)
	runInterveneLoop(t, engine)

	// 1. 被阻断的工具不执行。
	if n, _ := dangerous.snapshot(); n != 0 {
		t.Errorf("blocked tool executed %d times; want 0", n)
	}
	// 4. 同批其余工具照常执行。
	if n, _ := safe.snapshot(); n != 1 {
		t.Errorf("safe tool executed %d times; want 1", n)
	}

	// 2. 产出与 approval 拦截同形的 blocked 结果。
	mu.Lock()
	blockedCount := 0
	for _, res := range postResults {
		if res != nil && res.Status == "blocked" && !res.OK && res.Error == "forbidden by policy" {
			blockedCount++
		}
	}
	approvals := append([]string(nil), approvalEvents...)
	mu.Unlock()
	if blockedCount != 1 {
		t.Errorf("expected exactly 1 blocked ActionResult (OK=false, Status=blocked, Error=reason), got %d in %v",
			blockedCount, postResults)
	}
	if len(approvals) != 1 || approvals[0] != "dangerous:forbidden by policy" {
		t.Errorf("OnApprovalRequired events = %v; want [dangerous:forbidden by policy]", approvals)
	}

	// 3. 阻断原因进入下一轮模型输入（模型可读阻断原因）。
	second := historyText(*captured, 1)
	if !strings.Contains(second, "forbidden by policy") {
		t.Errorf("second-round model input missing block reason; history:\n%s", second)
	}
	if !strings.Contains(second, `Action "safe" executed: status=success`) {
		t.Errorf("second-round model input missing safe tool result; history:\n%s", second)
	}
}

// ---- 场景二：PreToolCall 返回 RewriteParams ----

// TestIntervene_PreToolCallRewriteParams 验证 PreToolCall 返回
// RewriteParams 时：执行器收到改写后的参数，而钩子本身观察到原始参数。
func TestIntervene_PreToolCallRewriteParams(t *testing.T) {
	var got interveneRecorder
	weatherTool, err := action.New("weather", "weather lookup",
		func(ctx context.Context, input map[string]any) (any, error) {
			got.record(input)
			return "sunny", nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	var preSeenParams map[string]any
	cb := &AgentCallbacks{
		PreToolCall: func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
			preSeenParams = params
			// 把 NYC 改写为 Beijing。
			return &ToolCallDecision{RewriteParams: map[string]any{"city": "Beijing"}}
		},
	}

	engine, _ := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"weather","params":{"city":"NYC"}}]}`,
		cb, weatherTool)
	runInterveneLoop(t, engine)

	// 执行器收到改写后的参数。
	if _, params := got.snapshot(); params == nil || params["city"] != "Beijing" {
		t.Errorf("executor received params %v; want city=Beijing", params)
	}
	// 钩子观察到的是模型原始参数。
	if preSeenParams == nil || preSeenParams["city"] != "NYC" {
		t.Errorf("PreToolCall observed params %v; want original city=NYC", preSeenParams)
	}
}

// ---- 场景三：Pre/Post 返回 AppendContext ----

// TestIntervene_AppendContext 验证 Pre 与 Post 返回 AppendContext 时：
// 附加上下文拼接进工具结果内容，随 session 进入下一轮 LLM 输入；
// 工具本身仍正常执行。
func TestIntervene_AppendContext(t *testing.T) {
	var got interveneRecorder
	echoTool, err := action.New("echo", "echo tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			got.record(input)
			return 42, nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	cb := &AgentCallbacks{
		PreToolCall: func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
			return &ToolCallDecision{AppendContext: "PRE-CTX-MARKER-42"}
		},
		PostToolCall: func(ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback {
			return &ToolCallFeedback{AppendContext: "POST-CTX-MARKER-43"}
		},
	}

	engine, captured := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"echo","params":{"k":"v"}}]}`,
		cb, echoTool)
	runInterveneLoop(t, engine)

	// 工具正常执行。
	if n, _ := got.snapshot(); n != 1 {
		t.Errorf("echo tool executed %d times; want 1", n)
	}
	// Pre/Post 附加上下文均进入下一轮模型输入。
	second := historyText(*captured, 1)
	if !strings.Contains(second, "PRE-CTX-MARKER-42") {
		t.Errorf("second-round model input missing Pre AppendContext; history:\n%s", second)
	}
	if !strings.Contains(second, "POST-CTX-MARKER-43") {
		t.Errorf("second-round model input missing Post AppendContext; history:\n%s", second)
	}
	// 原始结果内容仍在。
	if !strings.Contains(second, "42") {
		t.Errorf("second-round model input missing original tool result; history:\n%s", second)
	}
}

// ---- 场景四：nil/零值决策 → 行为与现状一致 ----

// TestIntervene_ZeroValueDecisionPassThrough 验证零值决策
// （&ToolCallDecision{} / &ToolCallFeedback{}）等价于放行：执行器收到
// 原始参数、结果内容不含任何附加上下文拼接，工具结果消息与未安装
// 钩子时的格式完全一致。
func TestIntervene_ZeroValueDecisionPassThrough(t *testing.T) {
	var got interveneRecorder
	echoTool, err := action.New("echo", "echo tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			got.record(input)
			return 42, nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	cb := &AgentCallbacks{
		PreToolCall: func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
			// 零值决策 = 放行。
			return &ToolCallDecision{}
		},
		PostToolCall: func(ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback {
			// 零值反馈 = 不干预。
			return &ToolCallFeedback{}
		},
	}

	engine, captured := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"echo","params":{"city":"NYC"}}]}`,
		cb, echoTool)
	runInterveneLoop(t, engine)

	// 执行器收到原始参数（未被改写）。
	if _, params := got.snapshot(); params == nil || params["city"] != "NYC" {
		t.Errorf("executor received params %v; want original city=NYC", params)
	}
	// 工具结果消息与无钩子现状完全一致：标准 executed 消息、无上下文拼接。
	second := historyText(*captured, 1)
	want := `Action "echo" executed: status=success, result=42`
	if !strings.Contains(second, want+"\n") {
		t.Errorf("second-round model input missing canonical result message %q; history:\n%s", want, second)
	}
}

// TestIntervene_NilDecisionPassThrough 验证 PreToolCall 返回 nil（放行）
// 且 PostToolCall 返回 nil（不干预）时，工具照常执行、结果保持原样。
func TestIntervene_NilDecisionPassThrough(t *testing.T) {
	var got interveneRecorder
	echoTool, err := action.New("echo", "echo tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			got.record(input)
			return 42, nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	cb := &AgentCallbacks{
		PreToolCall: func(ctx context.Context, toolName string, params map[string]any) *ToolCallDecision {
			return nil // nil 决策 = 放行
		},
		PostToolCall: func(ctx context.Context, toolName string, result *action.ActionResult) *ToolCallFeedback {
			return nil // nil 反馈 = 不干预
		},
	}

	engine, captured := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"echo","params":{"city":"NYC"}}]}`,
		cb, echoTool)
	runInterveneLoop(t, engine)

	if n, _ := got.snapshot(); n != 1 {
		t.Errorf("echo tool executed %d times; want 1", n)
	}
	second := historyText(*captured, 1)
	want := `Action "echo" executed: status=success, result=42`
	if !strings.Contains(second, want+"\n") {
		t.Errorf("second-round model input missing canonical result message %q; history:\n%s", want, second)
	}
}
