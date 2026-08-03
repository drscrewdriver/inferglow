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
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestAgent_Run_NoFlow_UsesExecuteLoop 验证未设置 WithFlow 时 Run 走 executeLoop
// 路径（oneshot 模式）。mock 模型返回结构化 decision，Run 应返回其中的
// final_response。若误走 executeFlow 路径，flow 为空不会调用模型，断言会失败。
func TestAgent_Run_NoFlow_UsesExecuteLoop(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"from-executeloop"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if resp != "from-executeloop" {
		t.Errorf("expected %q (executeLoop path), got %q", "from-executeloop", resp)
	}
}

// TestAgent_Run_WithFlow_UsesExecuteFlow 验证设置 WithFlow 时 Run 走 executeFlow
// 路径。Flow 的单步返回固定字符串 "from-flow"；模型 mock 配置为返回
// "from-model"。若 Run 返回 "from-flow" 则证明走了 flow 路径而非 executeLoop。
func TestAgent_Run_WithFlow_UsesExecuteFlow(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"from-model"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	step := flow.NewStep("single", func(ctx context.Context, input any) (any, error) {
		return "from-flow", nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if resp != "from-flow" {
		t.Errorf("expected %q (executeFlow path), got %q", "from-flow", resp)
	}
}

// TestExecuteFlow_ActionInStep 验证 flow 步骤中通过 Context 调用 Action。
// 注册一个 "echo" action，flow 的 step 通过 ContextFrom(ctx) 取得
// Context 并调用 ExecuteAction，断言 action 被调用且结果正确返回。
func TestExecuteFlow_ActionInStep(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	actionCalled := false
	echoAction, err := action.New("echo", "echoes input msg",
		func(ctx context.Context, input map[string]any) (any, error) {
			actionCalled = true
			return fmt.Sprintf("echo:%v", input["msg"]), nil
		})
	if err != nil {
		t.Fatalf("action.New: %v", err)
	}
	if err := actExt.Register(echoAction); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// flow 路径不调用模型，零值 mock 即可
	mockReq := &mockModelRequester{}

	step := flow.NewStep("call-action", func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.ContextFrom(ctx)
		if !ok {
			return nil, errors.New("Context not found in ctx")
		}
		result, err := fc.ExecuteAction(ctx, "echo", map[string]any{"msg": "hello"})
		if err != nil {
			return nil, fmt.Errorf("ExecuteAction: %w", err)
		}
		return result, nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !actionCalled {
		t.Error("expected echo action to be invoked, but it was not")
	}
	if resp != "echo:hello" {
		t.Errorf("expected %q, got %q", "echo:hello", resp)
	}
}

// TestExecuteFlow_FlowFailed 验证 flow 执行失败时 Run 返回包含
// "flow execution failed" 的错误，且原始 step 错误文本被保留。
//
// A6 改变了错误构造方式：原本用 fmt.Errorf("flow execution failed: %w", err)
// 包装原始 error（因此 errors.Is(err, boom) 成立）；A6 改为先把错误文本
// 格式化成 string、再视情况做 PII 脱敏、最后用 errors.New 返回，以便在
// 失败路径上对错误消息中的敏感数据做脱敏。这使 errors.Is 不再成立，
// 但错误文本（"step boom"）仍保留在返回的 error 中。
func TestExecuteFlow_FlowFailed(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{}

	boom := errors.New("step boom")
	step := flow.NewStep("fail", func(ctx context.Context, input any) (any, error) {
		return nil, boom
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	_, err := agent.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected error from failed flow, got nil")
	}
	if !strings.Contains(err.Error(), "flow execution failed") {
		t.Errorf("error %q does not contain %q", err.Error(), "flow execution failed")
	}
	if !strings.Contains(err.Error(), "step boom") {
		t.Errorf("error %q does not contain original step error text %q", err.Error(), "step boom")
	}
	// A6: errors.Is no longer holds because the error is now constructed via
	// errors.New(maskedString) rather than fmt.Errorf("...: %w", err). This
	// is the deliberate trade-off for being able to mask PII in the message.
	if errors.Is(err, boom) {
		t.Errorf("A6: errors.Is(err, boom) should be false after switching to errors.New; got true")
	}
}

// TestExtractFlowResponse 测试 extractFlowResponse 对三种输入格式的处理。
func TestExtractFlowResponse(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got := extractFlowResponse("hello")
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("map_with_final_response", func(t *testing.T) {
		got := extractFlowResponse(map[string]any{"final_response": "world"})
		if got != "world" {
			t.Errorf("got %q, want %q", got, "world")
		}
	})

	t.Run("int", func(t *testing.T) {
		got := extractFlowResponse(42)
		if got != "42" {
			t.Errorf("got %q, want %q", got, "42")
		}
	})

	t.Run("map_without_final_response", func(t *testing.T) {
		in := map[string]any{"other": "value"}
		got := extractFlowResponse(in)
		// A7: 无已知键时回退到 json.Marshal，输出合法 JSON 字符串。
		want := `{"other":"value"}`
		if got != want {
			t.Errorf("got %q, want %q (JSON marshal fallback)", got, want)
		}
	})
}

// TestExecuteFlow_RunAgentInStep 验证 flow 步骤中通过 Context.RunAgent
// 触发多轮 Agent 循环（executeLoop）。step 从 ctx 提取 Context 并调用
// RunAgent，mock 模型返回 response decision，断言最终响应来自 Agent 循环。
// 这证明 executeFlow 路径中 engine 引用已正确注入到 flowContextImpl。
func TestExecuteFlow_RunAgentInStep(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	agentCalled := false
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			agentCalled = true
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  `{"next_action":"response","final_response":"agent-loop-result"}`,
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	step := flow.NewStep("run-agent", func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.ContextFrom(ctx)
		if !ok {
			return nil, errors.New("Context not found in ctx")
		}
		result, err := fc.RunAgent(ctx, "do-task", "you are a helper", nil)
		if err != nil {
			return nil, fmt.Errorf("RunAgent: %w", err)
		}
		return result, nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	resp, err := agent.Run(context.Background(), "start")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !agentCalled {
		t.Error("expected model to be invoked via RunAgent→executeLoop, but it was not")
	}
	if resp != "agent-loop-result" {
		t.Errorf("expected %q, got %q", "agent-loop-result", resp)
	}
}

// TestExecuteFlow_RunAgentParallelInStep 验证 flow 步骤中通过
// Context.RunAgentParallel 触发多个子 Agent 循环。当前实现为顺序降级，
// mock 模型通过计数器为每个子任务返回不同结果，断言所有结果正确收集。
func TestExecuteFlow_RunAgentParallelInStep(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var callCount atomic.Int32
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			n := callCount.Add(1)
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{
				Delta:  fmt.Sprintf(`{"next_action":"response","final_response":"result-%d"}`, n),
				IsDone: true,
			}
			close(ch)
			return ch, nil
		},
	}

	step := flow.NewStep("parallel-agents", func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.ContextFrom(ctx)
		if !ok {
			return nil, errors.New("Context not found in ctx")
		}
		results, err := fc.RunAgentParallel(ctx, []flow.AgentSubTask{
			{Label: "task-a", UserMessage: "do-a", SystemPrompt: "sys-a", MaxRounds: 5},
			{Label: "task-b", UserMessage: "do-b", SystemPrompt: "sys-b", MaxRounds: 5},
		})
		if err != nil {
			return nil, fmt.Errorf("RunAgentParallel: %w", err)
		}
		// 合并结果，用逗号分隔
		return strings.Join(results, ","), nil
	}).Build()
	f := flow.NewFlow().AddStep(step).Build()

	agent := New(sess, actExt, mockReq, WithFlow(f))
	resp, err := agent.Run(context.Background(), "start")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 model calls (one per subtask), got %d", callCount.Load())
	}
	// Parallel execution does not guarantee order, so accept either permutation.
	if resp != "result-1,result-2" && resp != "result-2,result-1" {
		t.Errorf("expected %q or %q, got %q", "result-1,result-2", "result-2,result-1", resp)
	}
}
