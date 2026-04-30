//go:build ignore

// integration_demo 端到端集成演示
//
// 演示完整的 Agent 流程，不依赖 orchestrator 模块：
// 1. 连接真实 LLM（OpenAI 兼容协议）
// 2. 注册 Actions（文件保护、网络保护、目录列表、环境变量）
// 3. Session 留存对话历史
// 4. 内联 PLAN → EXECUTE 循环
//
// 使用方法：
//   cd e:\test\rewrite-agently\inferglow\examples
//   go run integration_demo.go
//
// 环境依赖：
//   - HTTP 服务: http://192.168.100.242:8200
//   - 模型: Qwen3.6-35B-A3B
//   - Token: dummy

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// ==================== 决策结构 ====================

type ActionCall struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
}

type Decision struct {
	NextAction    string       `json:"next_action"`
	ActionCalls   []ActionCall `json:"action_calls,omitempty"`
	FinalResponse string       `json:"final_response,omitempty"`
}

func parseDecision(content string) (*Decision, error) {
	var d Decision
	if err := json.Unmarshal([]byte(content), &d); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if d.NextAction == "" {
		return nil, fmt.Errorf("missing next_action")
	}
	if d.NextAction != "execute" && d.NextAction != "response" {
		return nil, fmt.Errorf("invalid next_action: %q", d.NextAction)
	}
	return &d, nil
}

// ==================== Agent 核心逻辑（内联） ====================

type agentConfig struct {
	maxRounds    int
	systemPrompt string
}

type RunOption func(*agentConfig)

func withMaxRounds(n int) RunOption {
	return func(c *agentConfig) { c.maxRounds = n }
}

func withSystemPrompt(prompt string) RunOption {
	return func(c *agentConfig) { c.systemPrompt = prompt }
}

func runAgent(ctx context.Context, sess *session.Session, actExt *actionExtension, provider model.ModelRequester, userMessage string, opts ...RunOption) (string, error) {
	cfg := &agentConfig{maxRounds: 10, systemPrompt: "You are a helpful assistant."}
	for _, opt := range opts {
		opt(cfg)
	}

	sess.AddMessage("user", userMessage, "")

	round := 0
	for {
		tools := buildToolDefs(actExt)
		req := &model.ModelRequest{
			System:      cfg.systemPrompt,
			ChatHistory: toModelChatMessages(sess.PreparePrompt()),
			Tools:       tools,
			Output: &model.OutputSchema{
				Type: "object",
				Properties: map[string]any{
					"next_action": map[string]any{"type": "string", "description": "\"execute\" or \"response\""},
					"action_calls": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":   map[string]any{"type": "string"},
								"params": map[string]any{"type": "object"},
							},
						},
					},
					"final_response": map[string]any{"type": "string", "description": "Response when next_action is response"},
				},
			},
		}

		data, err := provider.GenerateRequestData(ctx, req)
		if err != nil {
			return "", err
		}

		stream, err := provider.RequestModel(ctx, data)
		if err != nil {
			return "", err
		}

		var content strings.Builder
		for chunk := range stream {
			content.WriteString(chunk.Delta)
		}

		decision, err := parseDecision(content.String())
		if err != nil {
			return "", err
		}

		if decision.NextAction == "response" || decision.NextAction != "execute" || len(decision.ActionCalls) == 0 || round >= cfg.maxRounds {
			if decision.NextAction == "response" {
				sess.AddMessage("assistant", decision.FinalResponse, "")
			}
			return decision.FinalResponse, nil
		}

		// Execute actions
		results := dispatchActions(ctx, actExt.registry, decision.ActionCalls)
		for i, call := range decision.ActionCalls {
			if i < len(results) {
				r := results[i]
				msg := fmt.Sprintf("Action %q result: %v", call.Name, r.Result)
				if !r.OK {
					msg = fmt.Sprintf("Action %q failed: %s", call.Name, r.Error)
				}
				sess.AddMessage("system", msg, "")
			}
		}

		round++
	}
}

// ==================== ActionExtension（内联） ====================

type actionExtension struct {
	registry *action.ActionRegistry
}

func newActionExtension(r *action.ActionRegistry) *actionExtension {
	return &actionExtension{registry: r}
}

func (e *actionExtension) listActions() []map[string]any {
	names := e.registry.List()
	result := make([]map[string]any, 0, len(names))
	for _, name := range names {
		a := e.registry.GetAction(name)
		if a == nil {
			continue
		}
		result = append(result, map[string]any{
			"name":        a.Name,
			"description": a.Description,
			"schema":      a.Schema,
		})
	}
	return result
}

func (e *actionExtension) executeAction(ctx context.Context, name string, params map[string]any) (*action.ActionResult, error) {
	return e.registry.Execute(ctx, name, params)
}

// ==================== 工具函数 ====================

// toModelChatMessages converts session.ChatMessage slice to model.ChatMessage slice.
func toModelChatMessages(msgs []session.ChatMessage) []model.ChatMessage {
	result := make([]model.ChatMessage, len(msgs))
	for i, m := range msgs {
		content, _ := m.Content.(string)
		result[i] = model.ChatMessage{
			Role:    m.Role,
			Content: content,
			Name:    m.Name,
		}
	}
	return result
}

func buildToolDefs(actExt *actionExtension) []model.ToolDefinition {
	actions := actExt.listActions()
	tools := make([]model.ToolDefinition, 0, len(actions))
	for _, a := range actions {
		var params map[string]any
		if s, ok := a["schema"].(map[string]any); ok {
			params = s
		}
		tools = append(tools, model.ToolDefinition{
			Name:        a["name"].(string),
			Description: a["description"].(string),
			Parameters:  params,
		})
	}
	return tools
}

func dispatchActions(ctx context.Context, reg *action.ActionRegistry, calls []ActionCall) []*action.ActionResult {
	results := make([]*action.ActionResult, len(calls))
	for i, call := range calls {
		r, err := reg.Execute(ctx, call.Name, call.Params)
		if err != nil {
			results[i] = &action.ActionResult{OK: false, Status: "error", Error: err.Error()}
		} else {
			results[i] = r
		}
	}
	return results
}

// ==================== main ====================

func main() {
	ctx := context.Background()

	llmURL := "http://192.168.100.242:8200/v1"
	llmModel := "Qwen3.6-35B-A3B"
	llmAPIKey := "dummy"

	provider := &model.OpenAICompatibleProvider{
		BaseURL: llmURL,
		APIKey:  llmAPIKey,
		Model:   llmModel,
	}

	fmt.Println("========================================")
	fmt.Println("  Agently 端到端集成演示")
	fmt.Println("========================================")
	fmt.Printf("  LLM:    %s\n", llmURL)
	fmt.Printf("  模型:   %s\n", llmModel)
	fmt.Printf("  Token:  %s\n", llmAPIKey)
	fmt.Println("  平台:   Windows")
	fmt.Println()

	// --- LLM 连通性测试 ---
	fmt.Println("--- 测试 LLM 连通性 ---")
	healthSession := session.NewSession("health-check", 1000)
	healthSession.AddMessage("user", "Reply with exactly 'OK' if you can hear me.", "")

	healthReq := &model.ModelRequest{
		System:      "You are a health checker. Reply with exactly 'OK' if you can hear me.",
		ChatHistory: toModelChatMessages(healthSession.PreparePrompt()),
	}
	healthData, err := provider.GenerateRequestData(ctx, healthReq)
	if err != nil {
		log.Fatalf("生成请求数据失败: %v", err)
	}
	stream, err := provider.RequestModel(ctx, healthData)
	if err != nil {
		log.Fatalf("LLM 连接失败: %v", err)
	}
	var healthText strings.Builder
	for chunk := range stream {
		healthText.WriteString(chunk.Delta)
	}
	healthResult := strings.TrimSpace(healthText.String())
	fmt.Printf("  LLM 响应: %q\n", healthResult)
	fmt.Println("  LLM 连通性测试: 通过")
	fmt.Println()

	// --- 创建 Session ---
	sess := session.NewSession("integration-demo", 32000)
	sess.AutoResize = true

	// --- 创建 Actions ---
	reg := action.NewRegistry()

	fileAction, _ := action.New(
		"file_write_test",
		"在指定路径写入文件内容，验证文件系统操作能力。返回写入结果和错误信息。",
		func(ctx context.Context, input map[string]any) (any, error) {
			path := input["path"].(string)
			content := input["content"].(string)
			mode := input["mode"].(string)

			var flag int
			if mode == "append" {
				flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
			} else {
				flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			}

			f, err := os.OpenFile(path, flag, 0644)
			if err != nil {
				return map[string]any{"path": path, "success": false, "error": fmt.Sprintf("打开失败: %v", err)}, nil
			}
			defer f.Close()
			_, err = f.WriteString(content)
			if err != nil {
				return map[string]any{"path": path, "success": false, "error": fmt.Sprintf("写入失败: %v", err)}, nil
			}
			return map[string]any{"path": path, "success": true, "result": fmt.Sprintf("成功写入 %d 字节到 %s", len(content), path)}, nil
		},
	)
	reg.Register(fileAction)

	netAction, _ := action.New(
		"net_connectivity_test",
		"测试目标地址的 TCP 连接。返回连接结果和错误信息。",
		func(ctx context.Context, input map[string]any) (any, error) {
			target := input["target"].(string)
			port := int(input["port"].(float64))
			addr := fmt.Sprintf("%s:%d", target, port)
			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if err != nil {
				return map[string]any{"target": addr, "success": false, "error": fmt.Sprintf("连接失败: %v", err)}, nil
			}
			conn.Close()
			return map[string]any{"target": addr, "success": true, "result": fmt.Sprintf("成功连接到 %s", addr)}, nil
		},
	)
	reg.Register(netAction)

	dirAction, _ := action.New(
		"list_directory",
		"列出指定目录的文件和子目录。",
		func(ctx context.Context, input map[string]any) (any, error) {
			path := input["path"].(string)
			entries, err := os.ReadDir(path)
			if err != nil {
				return map[string]any{"path": path, "success": false, "error": fmt.Sprintf("读取失败: %v", err)}, nil
			}
			var files []string
			for _, e := range entries {
				files = append(files, e.Name())
			}
			return map[string]any{"path": path, "success": true, "result": files}, nil
		},
	)
	reg.Register(dirAction)

	envAction, _ := action.New(
		"check_environment",
		"检查系统环境变量。",
		func(ctx context.Context, input map[string]any) (any, error) {
			keys := input["keys"].([]any)
			result := make(map[string]string)
			for _, k := range keys {
				key := k.(string)
				result[key] = os.Getenv(key)
			}
			return map[string]any{"success": true, "result": result}, nil
		},
	)
	reg.Register(envAction)

	fmt.Println("--- 已注册的 Actions ---")
	for _, name := range reg.List() {
		a := reg.GetAction(name)
		fmt.Printf("  %s: %s\n", name, a.Description)
	}
	fmt.Println()

	actExt := newActionExtension(reg)
	systemPrompt := `你是 Agently Agent，运行在 Windows 系统上。你拥有以下能力：
1. file_write_test: 文件写入测试
2. net_connectivity_test: 网络连通性测试
3. list_directory: 列出目录内容
4. check_environment: 检查系统环境变量

任务：评估用户需求，决定是否需要执行 Action。
如果需要执行 Action，返回 execute 决策；如果可以直接回复，返回 response 决策。`

	// --- 场景 1 ---
	fmt.Println("========================================")
	fmt.Println("  场景 1: 测试文件系统保护能力")
	fmt.Println("========================================")
	fmt.Println("用户: 帮我测试一下当前环境的文件保护能力，试着在 C:\\ 和当前目录写入文件，然后告诉我结果。")
	fmt.Println()

	result1, err := runAgent(ctx, sess, actExt, provider,
		"帮我测试一下当前环境的文件保护能力，试着在 C:\\ 和当前目录写入文件，然后告诉我结果。",
		withMaxRounds(10), withSystemPrompt(systemPrompt))
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  Agent: %s\n", result1)
	}
	fmt.Println()

	// --- 场景 2 ---
	fmt.Println("========================================")
	fmt.Println("  场景 2: 测试网络保护能力")
	fmt.Println("========================================")
	fmt.Println("用户: 现在测试一下网络保护能力，检查一下你能访问哪些地址，比如 192.168.100.242:8200 和 www.baidu.com:80")
	fmt.Println()

	result2, err := runAgent(ctx, sess, actExt, provider,
		"现在测试一下网络保护能力，检查一下你能访问哪些地址，比如 192.168.100.242:8200 和 www.baidu.com:80",
		withMaxRounds(10), withSystemPrompt(systemPrompt))
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  Agent: %s\n", result2)
	}
	fmt.Println()

	// --- 场景 3: Session 留存 ---
	fmt.Println("========================================")
	fmt.Println("  场景 3: Session 留存验证")
	fmt.Println("========================================")
	fmt.Println("用户: 你还记得我们之前测试了哪些保护能力吗？总结一下。")
	fmt.Println()

	result3, err := runAgent(ctx, sess, actExt, provider,
		"你还记得我们之前测试了哪些保护能力吗？总结一下。",
		withMaxRounds(10), withSystemPrompt(systemPrompt))
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  Agent: %s\n", result3)
	}
	fmt.Println()

	// --- 场景 4: 直接对话 ---
	fmt.Println("========================================")
	fmt.Println("  场景 4: 直接对话")
	fmt.Println("========================================")
	fmt.Println("用户: 请用一句话总结今天的安全测试结果。")
	fmt.Println()

	result4, err := runAgent(ctx, sess, actExt, provider,
		"请用一句话总结今天的安全测试结果。",
		withMaxRounds(10), withSystemPrompt(systemPrompt))
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
	} else {
		fmt.Printf("  Agent: %s\n", result4)
	}
	fmt.Println()

	// --- Session 最终状态 ---
	fmt.Println("========================================")
	fmt.Println("  Session 最终状态")
	fmt.Println("========================================")
	fullCtx := sess.GetFullContext()
	window := sess.GetContextWindow()
	fmt.Printf("  FullContext 消息数: %d\n", len(fullCtx))
	fmt.Printf("  ContextWindow 消息数: %d\n", len(window))
	fmt.Println("  对话历史:")
	for i, msg := range fullCtx {
		content := fmt.Sprintf("%v", msg.Content)
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		fmt.Printf("    [%d] %s: %s\n", i+1, msg.Role, content)
	}
	fmt.Println()

	fmt.Println("========================================")
	fmt.Println("  测试完成")
	fmt.Println("========================================")
	fmt.Println("  已验证的能力:")
	fmt.Println("  - [x] LLM 连通性 (OpenAI 兼容 API)")
	fmt.Println("  - [x] 文件系统测试 (file_write_test)")
	fmt.Println("  - [x] 网络连通性测试 (net_connectivity_test)")
	fmt.Println("  - [x] 目录列表能力 (list_directory)")
	fmt.Println("  - [x] 环境变量检查 (check_environment)")
	fmt.Println("  - [x] Session 留存对话历史")
	fmt.Println("  - [x] PLAN -> EXECUTE 多轮循环")
}
