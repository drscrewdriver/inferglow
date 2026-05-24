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

// 示例：可插拔架构 —— 默认模式（无沙箱）+ 接口注入安全特性
//
// 本示例演示 v2 可插拔架构的核心用法：
//   1. 默认编译模式（无需 -tags with_sandbox），不引入 sandbox 依赖
//   2. 通过接口注入启用安全特性（sessionhook + agenthook），orchestrator/session 不直接依赖 security
//   3. 不注入安全钩子时的零开销路径
//
// 运行: go run example_pluggable.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/security/agenthook"
	"github.com/inferglow/security/pii"
	promptinjection "github.com/inferglow/security/prompt_injection"
	"github.com/inferglow/security/sessionhook"
	"github.com/inferglow/session"
)

// mockLLM 返回固定的 Decision JSON，让 Engine 在第一轮就拿到
// next_action="response" 并终止循环。无需真实模型 API Key。
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
		Delta:  `{"next_action":"response","final_response":"Hello from pluggable architecture demo!"}`,
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
	llm := &mockLLM{}

	// ============================================================
	// 模式 A：零开销模式（无安全钩子、无沙箱）
	// ============================================================
	// 这是 v2 默认行为：session 和 orchestrator/agent 都不引入
	// security 依赖。securityHook / outputHook / piiMasker 均为 nil，
	// AddMessageChecked 和 Agent.Run 跳过对应检查，零开销。
	fmt.Println("=== 模式 A：零开销模式（无安全钩子） ===")

	sessA := session.NewSession("zero-overhead", 4000)
	actExtA := agent.NewActionExtension()
	greetA, _ := action.New("greet", "Greet", func(ctx context.Context, req map[string]any) (string, error) {
		return "Hello!", nil
	})
	_ = actExtA.Register(greetA)

	agA := agent.New(sessA, actExtA, llm, agent.WithMaxRounds(3))
	respA, errA := agA.Run(ctx, "Hi")
	fmt.Printf("  响应: %s (err=%v)\n\n", respA, errA)

	// ============================================================
	// 模式 B：接口注入安全特性（sessionhook + agenthook）
	// ============================================================
	// 通过接口注入启用安全特性，无需特殊编译标签。
	// session 和 orchestrator/agent 仅保留接口契约，
	// 实现在 security/sessionhook 和 security/agenthook 子包。
	fmt.Println("=== 模式 B：接口注入安全特性 ===")

	// B1. 输入侧注入检测：sessionhook.SecurityHook 实现 session.MessageHook
	secHook := sessionhook.NewSecurityHook(promptinjection.NewDefaultConfig())
	sessB := session.NewSessionWithOptions("secured", 4000, session.WithSecurityHook(secHook))
	fmt.Println("  ✓ 输入侧注入检测已注入 (sessionhook.SecurityHook → session.MessageHook)")

	// B2. Action 注册
	actExtB := agent.NewActionExtension()
	greetB, _ := action.New("greet", "Greet", func(ctx context.Context, req map[string]any) (string, error) {
		return "Hello!", nil
	})
	_ = actExtB.Register(greetB)

	// B3. 输出侧注入检测：agenthook.OutputInjectionHook 实现 agent.OutputSecurityHook
	outHook := agenthook.NewOutputInjectionHook(promptinjection.NewDefaultConfig())
	fmt.Println("  ✓ 输出侧注入检测已注入 (agenthook.OutputInjectionHook → agent.OutputSecurityHook)")

	// B4. PII 脱敏：agenthook.PIIMasker 适配 pii.Masker，实现 agent.PIIMasker
	piiMasker := agenthook.NewPIIMasker(pii.NewMasker(pii.MaskConfig{}))
	fmt.Println("  ✓ PII 脱敏已注入 (agenthook.PIIMasker → agent.PIIMasker)")

	// B5. 组装 Agent：三个安全接口相互独立，按需注入
	agB := agent.New(sessB, actExtB, llm,
		agent.WithMaxRounds(3),
		agent.WithOutputSecurityHook(outHook),
		agent.WithPIIMasker(piiMasker),
	)
	fmt.Println("  Agent 已组装（maxRounds=3 + outputHook + piiMasker）")

	respB, errB := agB.Run(ctx, "Hi, my email is test@example.com")
	fmt.Printf("  响应: %s (err=%v)\n\n", respB, errB)

	// ============================================================
	// 模式 C：仅启用 PII 脱敏（不启用注入检测）
	// ============================================================
	// 演示三个安全接口的独立性：可以只注入部分特性
	fmt.Println("=== 模式 C：仅启用 PII 脱敏 ===")

	sessC := session.NewSession("pii-only", 4000)
	actExtC := agent.NewActionExtension()
	greetC, _ := action.New("greet", "Greet", func(ctx context.Context, req map[string]any) (string, error) {
		return "Hello!", nil
	})
	_ = actExtC.Register(greetC)

	// 只注入 PII 脱敏，不注入注入检测
	agC := agent.New(sessC, actExtC, llm,
		agent.WithMaxRounds(3),
		agent.WithPIIMasker(agenthook.NewPIIMasker(pii.NewMasker(pii.MaskConfig{}))),
	)
	fmt.Println("  Agent 已组装（仅 piiMasker，无 outputHook / securityHook）")

	respC, errC := agC.Run(ctx, "Hi")
	fmt.Printf("  响应: %s (err=%v)\n\n", respC, errC)

	// ============================================================
	// 总结
	// ============================================================
	fmt.Println("=== 可插拔架构总结 ===")
	fmt.Println("  1. 默认模式（go build ./...）：不引入 sandbox，零安全开销")
	fmt.Println("  2. 安全特性通过接口注入（sessionhook / agenthook），无需特殊编译")
	fmt.Println("  3. 三个安全接口（MessageHook / OutputSecurityHook / PIIMasker）相互独立")
	fmt.Println("  4. 沙箱需 -tags with_sandbox 编译（见 example_sandbox_enabled.go）")
}
