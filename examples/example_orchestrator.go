//go:build ignore

// 示例：如何使用 orchestrator 模块组装 Agent 并执行 PLAN→EXECUTE 循环
//
// 运行: go run example_orchestrator.go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/actionruntime"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// mockLLM 是一个假的 ModelRequester，用于在不依赖真实 LLM 的情况下演示
// orchestrator 的 PLAN→EXECUTE 循环。它直接返回一段固定的 Decision JSON，
// 让 Engine 在第一轮就拿到 next_action="response" 并终止循环。
type mockLLM struct{}

func (m *mockLLM) Name() string { return "mock-llm" }

// GenerateRequestData 把 ModelRequest 转换成 RequestData。
// 这里我们仅透传聊天历史，并标注模型名以便调试。
func (m *mockLLM) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{
		Model:    "mock-model",
		Messages: req.ChatHistory,
	}, nil
}

// RequestModel 返回一个流式 channel，向其中写入一段 Decision JSON 后关闭。
// Engine.executeLoop 会把 Delta 拼接起来，交给 actionruntime.ParseDecision 解析。
func (m *mockLLM) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{
		Delta:  `{"next_action":"response","final_response":"Hello from mock LLM! Agent orchestration works end-to-end."}`,
		IsDone: true,
	}
	close(ch)
	return ch, nil
}

// BroadcastResponse 在 Engine 路径中不会被调用，返回 nil 即可。
func (m *mockLLM) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

func main() {
	ctx := context.Background()

	// ============================================================
	// Example 1: 组装 Agent 组件
	// ============================================================
	fmt.Println("=== Example 1: 组装 Agent 组件 ===")

	// 1.1 创建会话：ID 用于标识，4000 是上下文窗口的最大长度
	sess := session.NewSession("orchestrator-demo", 4000)
	fmt.Printf("创建 Session: ID=%s, MaxLength=%d\n", sess.ID, sess.MaxLength)

	// 1.2 创建 ActionExtension，用于管理可被 LLM 调用的 Action
	actExt := agent.NewActionExtension()

	// 1.3 注册一个简单的 greet 动作：函数签名必须为
	//     func(ctx context.Context, req InputT) (OutputT, error)
	//     这里 InputT 使用 map[string]any，OutputT 使用 string，最灵活也最简单。
	greetAction, err := action.New("greet", "Greet a user by name", func(ctx context.Context, req map[string]any) (string, error) {
		name, _ := req["name"].(string)
		if name == "" {
			name = "friend"
		}
		return fmt.Sprintf("Hello, %s! Greeting from inferglow action.", name), nil
	})
	if err != nil {
		fmt.Printf("注册 greet 动作失败: %v\n", err)
		return
	}
	if err := actExt.Register(greetAction); err != nil {
		fmt.Printf("注册 greet 动作到 ActionExtension 失败: %v\n", err)
		return
	}

	// 1.4 创建 Mock LLM（无需真实模型 API Key 即可演示编排逻辑）
	llm := &mockLLM{}
	fmt.Printf("创建 MockLLM: Name=%s\n", llm.Name())

	// 1.5 打印当前注册的 Action 列表
	registry := actExt.GetRegistry()
	fmt.Printf("已注册 Action: %v\n\n", registry.List())

	// ============================================================
	// Example 2: LoopGuard 配置
	// ============================================================
	fmt.Println("=== Example 2: LoopGuard 配置 ===")

	// LoopGuard 用于检测 Agent 的死亡循环：
	//   - RepeatActionWindow: 连续 N 轮重复同样的 ActionCalls → break
	//   - OutputStagnationWindow: 连续 N 轮输出相似度超阈值 → break
	//   - TimeBudget: 整轮 run 超时 → break
	//   - TokenBudget: 累计 token 超限 → break
	// 未指定（零值）的字段会被 NewLoopGuard 替换为默认值。
	guard := agent.NewLoopGuard(agent.LoopGuardConfig{
		RepeatActionWindow: 3,
		TimeBudget:         2 * time.Minute,
		TokenBudget:        50000,
	})
	// 注：LoopGuardConfig 字段未导出读取接口，这里仅打印我们显式设置的值；
	// NewLoopGuard 会自动给下列零值字段填充默认：
	//   OutputStagnationWindow = 3
	//   OutputSimilarityThreshold = 0.9
	//   (Disabled 默认为 false，表示启用守卫)
	fmt.Printf("已配置: RepeatActionWindow=3, TimeBudget=%v, TokenBudget=50000\n", 2*time.Minute)
	fmt.Println("默认应用: OutputStagnationWindow=3, OutputSimilarityThreshold=0.9")

	// 用一个“全新”的状态调用 Check，应该返回 VerdictContinue
	freshState := agent.LoopGuardState{
		Round:       0,
		ActionCalls: nil,
		LastOutput:  "",
		TotalTokens: 0,
		StartedAt:   time.Now(),
	}
	verdict, err := guard.Check(freshState)
	if err != nil {
		fmt.Printf("LoopGuard.Check 返回错误: %v\n", err)
	} else {
		fmt.Printf("LoopGuard.Check(初始状态) = {Action: %s, Reason: %q}\n", verdict.Action, verdict.Reason)
	}

	// Reset 清空内部滑动窗口，可复用于下一次 Run
	guard.Reset()
	fmt.Println("已调用 guard.Reset()，LoopGuard 内部状态已清空")
	fmt.Println()

	// ============================================================
	// Example 3: AuditChain 准备
	// ============================================================
	fmt.Println("=== Example 3: AuditChain 准备 ===")

	// AuditChain 是一个 append-only、哈希链式的审计日志。
	// cfg.Enabled=false 时 Append 是 no-op（零开销），匹配“默认关闭”语义。
	// SignatureKey 非空时会给每条 entry 计算 HMAC-SHA256 签名，便于事后验签。
	auditChain, err := audit.NewAuditChain(audit.AuditConfig{
		Enabled:      true,
		SignatureKey: []byte("demo-key"),
		MaxEntries:   100,
	})
	if err != nil {
		fmt.Printf("创建 AuditChain 失败: %v\n", err)
		return
	}
	fmt.Printf("AuditChain.IsEnabled() = %v\n", auditChain.IsEnabled())
	fmt.Printf("AuditChain.Len()       = %d\n", auditChain.Len())
	fmt.Println("注：*audit.AuditChain 实现了 audit.AuditHook 接口，可直接传给 Engine")
	fmt.Println()

	// ============================================================
	// Example 4: Agent Run（端到端演示 PLAN→EXECUTE 循环）
	// ============================================================
	fmt.Println("=== Example 4: Agent Run ===")

	// agent.New 内部会：
	//   1. 用 NewSessionExtension 包裹 sess
	//   2. 用 NewEngine(...) 构造一个默认 Engine（NoOpHook，无 LoopGuard）
	//   3. 应用传入的 WithMaxRounds / WithSystemPrompt 等 RunOption 作为 Agent 默认
	ag := agent.New(sess, actExt, llm,
		agent.WithMaxRounds(5),
		agent.WithSystemPrompt("You are a demo assistant."),
	)
	fmt.Println("Agent 已创建：maxRounds=5, systemPrompt=\"You are a demo assistant.\"")
	fmt.Println("开始执行 Agent.Run ...")

	result, runErr := ag.Run(ctx, "Hello, please greet me.")
	if runErr != nil {
		fmt.Printf("Agent.Run 返回错误: %v\n", runErr)
	} else {
		fmt.Printf("Agent.Run 返回结果: %s\n", result)
	}
	fmt.Println()

	// ============================================================
	// Example 5: AuditChain 验证（手动追加 + 验签 + 查询）
	// ============================================================
	fmt.Println("=== Example 5: AuditChain 验证 ===")

	// 手动追加一条审计记录。Append 会自动填充 ID/Timestamp/PrevHash/Hash，
	// 并在 SignatureKey 非空时计算 Signature。
	hash, appendErr := auditChain.Append(&audit.AuditEntry{
		Source: "agent",
		Action: "decision",
		Input:  "demo run completed",
		Output: map[string]any{"status": "success"},
	})
	if appendErr != nil {
		fmt.Printf("AuditChain.Append 返回错误: %v\n", appendErr)
	} else {
		fmt.Printf("AuditChain.Append 成功，entry hash = %s\n", hash)
	}
	fmt.Printf("AuditChain.Len() = %d\n", auditChain.Len())

	// VerifyChain 会重新计算每条 entry 的 Hash、检查 PrevHash 链式连续性、
	// 并在配置了 SignatureKey 时校验 HMAC-SHA256 签名。
	if verifyErr := auditChain.VerifyChain(); verifyErr != nil {
		fmt.Printf("AuditChain.VerifyChain() 失败: %v\n", verifyErr)
	} else {
		fmt.Println("AuditChain.VerifyChain() 通过：所有 entry 完整且签名有效")
	}

	// Query 按 Source/Action/时间窗/Metadata 过滤。这里筛 Source="agent"。
	matched, queryErr := auditChain.Query(audit.QueryFilter{Source: "agent"})
	if queryErr != nil {
		fmt.Printf("AuditChain.Query 返回错误: %v\n", queryErr)
	} else {
		fmt.Printf("AuditChain.Query(Source=agent) 命中 %d 条记录\n", len(matched))
		for i, e := range matched {
			fmt.Printf("  [%d] id=%s action=%s hash=%s\n", i, e.ID, e.Action, e.Hash)
		}
	}
	fmt.Println()

	// ============================================================
	// 附加说明：手动组装带 Audit + LoopGuard 的 Engine（高级用法）
	// ============================================================
	// agent.New 内部使用 NewEngine(...)，默认不挂 AuditHook 与 LoopGuard。
	// 当你需要同时启用审计与循环守卫时，可以使用更低层的构造器：
	//
	//   sessExt2 := agent.NewSessionExtension(session.NewSession("advanced", 4000))
	//   actExt2  := agent.NewActionExtension()
	//   engine   := agent.NewEngineWithAuditAndLoopGuard(sessExt2, actExt2, llm, auditChain, guard)
	//
	// 该 Engine 的 executeLoop 是未导出方法，无法在外部直接调用；
	// 其典型用法是嵌入到自定义 Agent 包装器中，或通过反射/测试钩子触发。
	// 日常使用推荐 agent.New + ag.Run，已在 Example 4 中演示。

	// 防止 unused import 警告（actionruntime 在 Examples 中被引用以说明 Decision 结构）
	_ = actionruntime.Decision{}

	fmt.Println("=== All examples completed ===")
}
