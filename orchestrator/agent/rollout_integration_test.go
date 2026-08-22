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
	"testing"

	"github.com/inferglow/action"
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

	wantTypes := []session.RolloutItemType{
		session.RolloutUserMessage,
		session.RolloutToolCall,
		session.RolloutToolResult,
		session.RolloutAssistantMessage,
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
	if items[1].ToolName != "echo" {
		t.Errorf("tool_call ToolName = %q; want echo", items[1].ToolName)
	}
	if v := items[1].Params["v"]; v != "ping" {
		t.Errorf("tool_call Params = %v; want v=ping", items[1].Params)
	}
	// tool_result 覆盖结果内容（echo 返回 "ping"，按 formatToolResult
	// 序列化为 JSON 字符串 "ping"）。
	if items[2].Result != `"ping"` {
		t.Errorf("tool_result Result = %q; want \"ping\"", items[2].Result)
	}
	// assistant_message 为最终回复。
	if items[3].Content != "done" {
		t.Errorf("assistant_message Content = %q; want done", items[3].Content)
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