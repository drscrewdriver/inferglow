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

//go:build ignore

// 快速入门示例：展示 inferglow 核心概念的最小完整 Agent
//
// 本示例演示：
//  1. 创建 Session（对话记忆）
//  2. 注册 Action（将 Go 函数包装为 LLM 可调用的工具）
//  3. 使用 MockLLM 模拟 LLM 响应（无需真实 API Key）
//  4. 组装 Agent 并执行完整 PLAN→EXECUTE 循环
//
// 运行: go run example_quickstart.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// ---- MockLLM ----
// 无需真实 LLM API Key，直接返回固定 Decision JSON，
// 让 Engine 在第一轮就取到 next_action="response" 并终止循环。

type mockLLM struct{}

func (m *mockLLM) Name() string { return "mock-llm" }

func (m *mockLLM) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{Model: "mock-model", Messages: req.ChatHistory}, nil
}

func (m *mockLLM) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{
		Delta:  `{"next_action":"response","final_response":"Hello from inferglow Agent! Orchestration works end-to-end."}`,
		IsDone: true,
	}
	close(ch)
	return ch, nil
}

func (m *mockLLM) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

// ---- main ----

func main() {
	ctx := context.Background()

	// ============================================================
	// Step 1: 创建 Session（对话记忆）
	// ============================================================
	// Session 管理 Agent 的对话历史。MaxLength 控制上下文窗口上限。
	sess := session.NewSession("quickstart-demo", 4000)
	fmt.Println("✓ Session created: ID =", sess.ID)

	// ============================================================
	// Step 2: 创建 ActionExtension（工具注册表）
	// ============================================================
	// ActionExtension 管理可被 LLM 调用的工具（Action）。
	// 每个 Action 对应一个 Go 函数，自动生成 JSON Schema 供 LLM 理解。
	actExt := agent.NewActionExtension()

	// 注册一个 Action：greet
	// action.New() 自动从函数签名推导 JSON Schema 并创建 LocalFunctionExecutor
	greetAction, err := action.New("greet", "Greet a user by name",
		func(ctx context.Context, req map[string]any) (string, error) {
			name, _ := req["name"].(string)
			if name == "" {
				name = "friend"
			}
			return fmt.Sprintf("Hello, %s! Greeting from inferglow action.", name), nil
		})
	if err != nil {
		fmt.Println("✗ Failed to create greet action:", err)
		return
	}
	if err := actExt.Register(greetAction); err != nil {
		fmt.Println("✗ Failed to register greet action:", err)
		return
	}
	fmt.Println("✓ Action registered: greet")

	// 注册一个 Action：add
	addAction, err := action.New("add", "Add two numbers together",
		func(ctx context.Context, req map[string]any) (int, error) {
			a, _ := req["a"].(float64)
			b, _ := req["b"].(float64)
			return int(a) + int(b), nil
		})
	if err != nil {
		fmt.Println("✗ Failed to create add action:", err)
		return
	}
	if err := actExt.Register(addAction); err != nil {
		fmt.Println("✗ Failed to register add action:", err)
		return
	}
	fmt.Println("✓ Action registered: add")

	// ============================================================
	// Step 3: 创建 MockLLM（用于演示，无需真实 API Key）
	// ============================================================
	llm := &mockLLM{}
	fmt.Println("✓ MockLLM created")

	// ============================================================
	// Step 4: 组装 Agent 并运行
	// ============================================================
	// agent.New 内部会：
	//   1. 用 SessionExtension 包裹 sess
	//   2. 用 NewEngine 构造默认 Engine
	//   3. 应用传入的 WithMaxRounds / WithSystemPrompt 等选项
	ag := agent.New(sess, actExt, llm,
		agent.WithMaxRounds(5),
		agent.WithSystemPrompt("You are a helpful assistant with access to tools."),
	)
	fmt.Println("✓ Agent created: maxRounds=5")

	fmt.Println("\n--- Running Agent ---")
	result, err := ag.Run(ctx, "Hello, please greet me.")
	if err != nil {
		fmt.Println("✗ Agent.Run failed:", err)
		return
	}
	fmt.Println("✓ Agent response:", result)

	// ============================================================
	// Step 5: 验证 Session 已记录对话
	// ============================================================
	fmt.Println("\n--- Session State ---")
	fmt.Printf("  FullContext: %d messages\n", len(sess.FullContext))
	fmt.Printf("  ContextWindow: %d messages\n", len(sess.ContextWindow))
	for i, msg := range sess.ContextWindow {
		content := fmt.Sprintf("%v", msg.Content)
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		fmt.Printf("  [%d] %s: %s\n", i, msg.Role, content)
	}

	fmt.Println("\n✓ Quickstart example completed successfully!")
}