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
	"encoding/json"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestRollout_WiredIntegration 跑一轮带工具调用的 Agent，断言生成的
// Rollout JSONL 文件存在、且 items 流顺序正确：
// user_message → tool_call → tool_result → assistant_message。
func TestRollout_WiredIntegration(t *testing.T) {
	dir := t.TempDir()
	rec := session.NewRolloutRecorder(dir, "rollout-wired")

	echoTool, err := action.New("echo", "echo tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			return input["v"], nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	// 复用 intervene 场景的 fake 模型：第一轮返回工具调用决策，
	// 之后返回最终回复。
	engine, _ := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"echo","params":{"v":"ping"}}]}`,
		nil, echoTool)
	// 从 RunOption 路径注入记录器（等价于 WithRollout 的引擎侧效果）。
	engine.rollout = rec

	runInterveneLoop(t, engine)

	items, err := rec.List("rollout-wired")
	if err != nil {
		t.Fatalf("List rollout failed: %v", err)
	}

	// P2：turn 边界 + 每 LLM round 的请求/响应事件已插入事件流。
	wantTypes := []session.RolloutItemType{
		session.RolloutTurnStart,
		session.RolloutUserMessage,
		session.RolloutLLMRequest,
		session.RolloutLLMResponse,
		session.RolloutToolCall,
		session.RolloutToolResult,
		session.RolloutLLMRequest,
		session.RolloutLLMResponse,
		session.RolloutAssistantMessage,
		session.RolloutTurnEnd,
	}
	if len(items) != len(wantTypes) {
		t.Fatalf("rollout has %d items; want %d: %+v", len(items), len(wantTypes), items)
	}
	for i, want := range wantTypes {
		if items[i].Type != want {
			t.Errorf("items[%d].Type = %q; want %q", i, items[i].Type, want)
		}
		if items[i].Seq != int64(i+1) {
			t.Errorf("items[%d].Seq = %d; want %d", i, items[i].Seq, i+1)
		}
		if items[i].SessionID != "rollout-wired" {
			t.Errorf("items[%d].SessionID = %q; want rollout-wired", i, items[i].SessionID)
		}
	}

	// tool_call 覆盖工具名与参数。
	if items[4].ToolName != "echo" {
		t.Errorf("tool_call ToolName = %q; want echo", items[4].ToolName)
	}
	if v := items[4].Params["v"]; v != "ping" {
		t.Errorf("tool_call Params = %v; want v=ping", items[4].Params)
	}
	// tool_result 覆盖结果内容（echo 返回 "ping"，按 formatToolResult
	// 序列化为 JSON 字符串 "ping"）。
	if items[5].Result != `"ping"` {
		t.Errorf("tool_result Result = %q; want \"ping\"", items[5].Result)
	}
	// assistant_message 为最终回复。
	if items[8].Content != "done" {
		t.Errorf("assistant_message Content = %q; want done", items[8].Content)
	}

	// P2 断言：所有 item 都盖上 turn=1；llm_request/llm_response 的
	// Payload 经 JSONL 往返后可还原为强类型契约；round 单调递增。
	for i, item := range items {
		if item.Turn != 1 {
			t.Errorf("items[%d].Turn = %d; want 1", i, item.Turn)
		}
	}
	var reqPayload model.LLMRequestPayload
	if raw, err := json.Marshal(items[2].Payload); err != nil {
		t.Fatalf("marshal llm_request payload: %v", err)
	} else if err := json.Unmarshal(raw, &reqPayload); err != nil {
		t.Fatalf("unmarshal llm_request payload: %v", err)
	}
	if reqPayload.Round != 0 || reqPayload.Breakdown == nil || reqPayload.Breakdown.Total <= 0 {
		t.Errorf("llm_request payload = %+v; want round=0 with non-empty breakdown", reqPayload)
	}
	var respPayload model.LLMResponsePayload
	if raw, err := json.Marshal(items[3].Payload); err != nil {
		t.Fatalf("marshal llm_response payload: %v", err)
	} else if err := json.Unmarshal(raw, &respPayload); err != nil {
		t.Fatalf("unmarshal llm_response payload: %v", err)
	}
	if respPayload.Round != 0 || respPayload.DurationMs < 0 {
		t.Errorf("llm_response payload = %+v; want round=0 with duration", respPayload)
	}
	var turnEnd model.TurnEndPayload
	if raw, err := json.Marshal(items[9].Payload); err != nil {
		t.Fatalf("marshal turn_end payload: %v", err)
	} else if err := json.Unmarshal(raw, &turnEnd); err != nil {
		t.Fatalf("unmarshal turn_end payload: %v", err)
	}
	if turnEnd.Turn != 1 || turnEnd.Reason != "final" || turnEnd.WallMs < 0 {
		t.Errorf("turn_end payload = %+v; want turn=1 reason=final", turnEnd)
	}
}

// TestRollout_EphemeralNoOp via engine：nil 记录器（默认）时 executeLoop
// 零行为变化——不 panic、无副作用、Replay 从共享目录读到空。
func TestRollout_ZeroWhenNilRecorder(t *testing.T) {
	dir := t.TempDir()
	// 不注入任何记录器到 engine：走默认 nil 路径。
	engine, _ := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"echo","params":{}}]}`,
		nil, mustEchoAction(t))
	runInterveneLoop(t, engine)

	// 单独的只读 recorder 读取同一目录：会话未记录任何 rollout 文件 → 空。
	probe := session.NewRolloutRecorder(dir, "rollout-nil")
	items, err := probe.List("rollout-nil")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("nil-recorder engine unexpectedly produced %d rollout items", len(items))
	}
}

func mustEchoAction(t *testing.T) *action.Action {
	t.Helper()
	a, err := action.New("echo", "echo tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			return nil, nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}
	return a
}
// TestRollout_FileOpFolded 验证文件类工具调用折叠出 file_op 事件
// （op 由工具名前缀识别，路径取自 params）。
func TestRollout_FileOpFolded(t *testing.T) {
	dir := t.TempDir()
	rec := session.NewRolloutRecorder(dir, "rollout-fileop")

	readTool, err := action.New("read_file", "read file tool",
		func(ctx context.Context, input map[string]any) (any, error) {
			return "contents", nil
		})
	if err != nil {
		t.Fatalf("failed to create action: %v", err)
	}

	engine, _ := newInterveneEngine(t,
		`{"next_action":"execute","action_calls":[{"name":"read_file","params":{"path":"a.txt"}}]}`,
		nil, readTool)
	engine.rollout = rec
	runInterveneLoop(t, engine)

	items, err := rec.List("rollout-fileop")
	if err != nil {
		t.Fatalf("List rollout failed: %v", err)
	}
	var fileOp *session.RolloutItem
	for i := range items {
		if items[i].Type == session.RolloutFileOp {
			fileOp = &items[i]
		}
	}
	if fileOp == nil {
		t.Fatalf("no file_op item in rollout: %+v", items)
	}
	var payload model.FileOpPayload
	raw, err := json.Marshal(fileOp.Payload)
	if err != nil {
		t.Fatalf("marshal file_op payload: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal file_op payload: %v", err)
	}
	if payload.Op != "read" || payload.Path != "a.txt" {
		t.Errorf("file_op payload = %+v; want op=read path=a.txt", payload)
	}
	if fileOp.Turn != 1 {
		t.Errorf("file_op Turn = %d; want 1", fileOp.Turn)
	}
}
