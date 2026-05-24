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

//go:build with_sandbox

// 示例：沙箱模式 —— 完整功能 Agent + SandboxExecutor
//
// 本示例需要使用 -tags with_sandbox 编译：
//
//   go build -tags with_sandbox example_sandbox_enabled.go
//   go run -tags with_sandbox example_sandbox_enabled.go
//
// 演示内容：
//   1. SandboxExecutor 通过 sandbox.Manager 执行隔离命令
//   2. 沙箱模式下的完整 Agent（安全钩子 + 沙箱执行）
//   3. 默认模式下此文件不编译（使用 stub SandboxExecutor）
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/sandbox"
	"github.com/inferglow/session"
)

// mockLLM 返回固定的 Decision JSON，让 Engine 在第一轮就拿到
// next_action="response" 并终止循环。
type mockLLM struct{}

func (m *mockLLM) Name() string { return "mock-llm" }

func (m *mockLLM) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{
		Model:    "mock-model",
		Messages: req.ChatHistory,
	}, nil
}

func (m *mockLLM) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{
		Delta:  `{"next_action":"response","final_response":"Hello from sandbox-enabled agent!"}`,
		IsDone: true,
	}
	close(ch)
	return ch, nil
}

func (m *mockLLM) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

func main() {
	ctx := context.Background()

	fmt.Println("=== 沙箱模式示例（with_sandbox build tag） ===")
	fmt.Println()

	// ============================================================
	// 1. 创建 SandboxExecutor（仅 with_sandbox 模式可用）
	// ============================================================
	fmt.Println("--- 1. 创建 SandboxExecutor ---")

	// 1.1 创建 sandbox.Manager 并注册后端
	mgr := sandbox.NewManager()
	_ = mgr.Register(sandbox.NewTrustedLocalProvider())
	fmt.Println("  ✓ sandbox.Manager 已创建，已注册 TrustedLocalProvider")

	// 1.2 创建 SandboxExecutor
	//     这个类型在默认模式下是 stub（返回错误），
	//     在 with_sandbox 模式下才是真实实现。
	sandboxExec := action.NewSandboxExecutor(action.SandboxExecutorConfig{
		Manager:     mgr,
		DefaultMode: sandbox.ModeTrustedLocal,
	})
	fmt.Println("  ✓ SandboxExecutor 已创建（真实实现，非 stub）")

	// 1.3 直接调用 SandboxExecutor.Execute 演示沙箱执行
	result, _ := sandboxExec.Execute(ctx, map[string]any{
		"argv":    []string{"echo", "hello from sandbox"},
		"timeout": "5s",
	})
	fmt.Printf("  沙箱执行结果: status=%s, ok=%v\n", result.Status, result.OK)
	if result.OK {
		if r, ok := result.Result.(map[string]any); ok {
			fmt.Printf("    stdout: %s\n", r["stdout"])
		}
	}
	fmt.Println()

	// ============================================================
	// 2. 将 SandboxExecutor 注册为 Action
	// ============================================================
	fmt.Println("--- 2. 注册沙箱 Action ---")

	// 创建一个使用 SandboxExecutor 的 Action
	sandboxAction, err := action.New("run_sandboxed", "Run a command in the sandbox", func(ctx context.Context, req map[string]any) (*action.ActionResult, error) {
		return sandboxExec.Execute(ctx, req)
	})
	if err != nil {
		fmt.Printf("  注册失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ Action %q 已注册（使用 SandboxExecutor）\n\n", sandboxAction.Name)

	// ============================================================
	// 3. 组装完整功能的 Agent
	// ============================================================
	fmt.Println("--- 3. 组装完整功能 Agent ---")

	// 3.1 Session
	sess := session.NewSession("sandbox-demo", 4000)

	// 3.2 ActionExtension + 注册 Action
	actExt := agent.NewActionExtension()
	_ = actExt.Register(sandboxAction)
	fmt.Println("  ✓ Session + ActionExtension 已创建")

	// 3.3 Agent（在沙箱模式下，Agent 可以使用沙箱执行）
	//     安全特性可通过接口注入按需启用（见 example_pluggable.go）
	llm := &mockLLM{}
	ag := agent.New(sess, actExt, llm,
		agent.WithMaxRounds(5),
		agent.WithSystemPrompt("You are a sandbox-enabled assistant."),
	)
	fmt.Println("  ✓ Agent 已组装（maxRounds=5）")
	fmt.Println()

	// ============================================================
	// 4. 运行 Agent
	// ============================================================
	fmt.Println("--- 4. 运行 Agent ---")
	resp, err := ag.Run(ctx, "Hello, please greet me.")
	fmt.Printf("  响应: %s\n", resp)
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	}
	fmt.Println()

	// ============================================================
	// 5. 总结
	// ============================================================
	fmt.Println("=== 沙箱模式总结 ===")
	fmt.Println("  1. with_sandbox tag 启用真实 SandboxExecutor（非 stub）")
	fmt.Println("  2. SandboxExecutor 通过 sandbox.Manager 调用后端 Provider")
	fmt.Println("  3. 默认模式下此文件不编译，SandboxExecutor 为 stub")
	fmt.Println("  4. 安全特性与沙箱独立，可同时启用（见 example_pluggable.go）")

	_ = time.Second // 防止 time 未使用（仅用于演示 timeout 配置）
}
