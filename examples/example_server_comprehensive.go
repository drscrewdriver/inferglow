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

// 综合 Server 示例：展示 inferglow 完整系统能力
//
// 本示例演示：
//  1. Server 初始化与配置
//  2. 通过 REST API 创建 Agent 和管理运行
//  3. 所有基础模块能力的串联使用
//  4. 审计开关（启用/禁用）
//  5. 沙箱和可插拔架构
//
// 运行: go run example_server_comprehensive.go
// 沙箱模式: go run -tags with_sandbox example_server_comprehensive.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/flow"
	"github.com/inferglow/flow/flowdef"
	"github.com/inferglow/flow/stage"
	stagebuiltin "github.com/inferglow/flow/stage/builtin"
	"github.com/inferglow/model"
	"github.com/inferglow/sandbox"
	"github.com/inferglow/schema"
	"github.com/inferglow/server"
	"github.com/inferglow/session"
	"github.com/inferglow/workspace"
)

// ============================================================================
// MockLLM: 返回固定 Decision JSON，无需真实 API Key
// ============================================================================

// mockLLM 是一个模拟的 LLM Provider，返回固定的 JSON 响应。
// 实现 model.Provider 接口的子集用于演示。
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
		Delta:  `{"next_action":"response","final_response":"Hello from MockLLM!"}`,
		IsDone: true,
	}
	close(ch)
	return ch, nil
}

func (m *mockLLM) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

// ============================================================================
// MockAgent 和 MockAgentStore: 供 Server 使用的模拟 Agent
// ============================================================================

// mockAgent 实现 server.AgentLike 接口，使用 MockLLM 运行。
type mockAgent struct {
	name   string
	llm    *mockLLM
	sess   *session.Session
	action *action.ActionRegistry
}

func (a *mockAgent) Run(ctx context.Context, userMessage string) (string, error) {
	a.sess.AddMessage("user", userMessage, "")
	prompt := a.sess.PreparePrompt()
	_ = prompt
	// 模拟 LLM 回复
	reply := fmt.Sprintf("[%s] echo: %s", a.name, userMessage)
	a.sess.AddMessage("assistant", reply, "")
	return reply, nil
}

// mockAgentStore 实现 server.AgentStore 接口。
type mockAgentStore struct {
	agents map[string]*mockAgent
	nextID int
}

func newMockAgentStore() *mockAgentStore {
	return &mockAgentStore{agents: make(map[string]*mockAgent)}
}

func (s *mockAgentStore) Get(id string) server.AgentLike {
	a, ok := s.agents[id]
	if !ok {
		return nil
	}
	return a
}

func (s *mockAgentStore) List() []server.AgentLike {
	var out []server.AgentLike
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out
}

func (s *mockAgentStore) Create(cfg server.AgentConfig) (string, error) {
	s.nextID++
	id := "agent-" + strconv.Itoa(s.nextID)
	s.agents[id] = &mockAgent{
		name:   cfg.Name,
		llm:    &mockLLM{},
		sess:   session.NewSession(id, 4000),
		action: action.NewRegistry(),
	}
	return id, nil
}

func (s *mockAgentStore) Delete(id string) error {
	delete(s.agents, id)
	return nil
}

// ============================================================================
// 工具函数: 注册为 Action 供演示
// ============================================================================

type AddRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

func addNumbers(ctx context.Context, req AddRequest) (int, error) {
	return req.A + req.B, nil
}

type GreetRequest struct {
	Name string `json:"name"`
}

func greet(req GreetRequest) (string, error) {
	if req.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	return "Hello, " + req.Name + "!", nil
}

type WeatherRequest struct {
	City string `json:"city"`
}

func getWeather(ctx context.Context, req WeatherRequest) (map[string]any, error) {
	return map[string]any{
		"city":     req.City,
		"temp":     25.5,
		"humidity": 60,
		"forecast": "sunny",
	}, nil
}

// ============================================================================
// 主函数
// ============================================================================

func main() {
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println("  InferGlow 综合 Server 示例 / Comprehensive Server Example")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println()

	// =========================================================================
	// 第一部分: Server 初始化与配置
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 1] Server 初始化与配置 / Server Init & Config")
	fmt.Println(strings.Repeat("-", 72))

	// 创建默认配置
	srvConfig := server.DefaultConfig()
	srvConfig.Addr = ":8080"
	srvConfig.ReadTimeout = 30 * time.Second
	srvConfig.WriteTimeout = 60 * time.Second
	srvConfig.IdleTimeout = 120 * time.Second
	fmt.Println("  Server config: addr=", srvConfig.Addr)
	fmt.Println("  ReadTimeout:  ", srvConfig.ReadTimeout)
	fmt.Println("  WriteTimeout: ", srvConfig.WriteTimeout)
	fmt.Println("  IdleTimeout:  ", srvConfig.IdleTimeout)

	// 创建 AgentStore
	agentStore := newMockAgentStore()
	fmt.Println("  MockAgentStore created")

	// 创建 Stage Registry 并注册 stage 函数
	stageReg := stage.NewRegistry()
	stageReg.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
		return stage.Outputs{"message": in["message"]}, nil
	})
	stageReg.Register("greet", func(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
		name, _ := in["name"].(string)
		return stage.Outputs{"greeting": "Hello, " + name + "!"}, nil
	})
	stagebuiltin.RegisterAll(stageReg)
	fmt.Println("  Stage registry created with echo, greet, and builtin stages")

	// 创建 FlowStore 并传给 Server
	flowStore := server.NewFlowStore(stageReg)
	fmt.Println("  FlowStore created with pre-configured stages")

	// 创建 Server 实例 (使用 NewServerWithFlows 传入预配置的 FlowStore)
	srv := server.NewServerWithFlows(srvConfig, agentStore, flowStore)
	fmt.Println("  Server instance created with FlowStore")

	// 配置内存存储
	memStore := server.NewInMemoryStore()
	srv.SetMemoryStore(memStore)
	fmt.Println("  InMemoryStore configured")

	fmt.Println()

	// =========================================================================
	// 第二部分: Model 层 — LLM Provider 设置
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 2] Model 层 / Model Layer — MockLLM Provider")
	fmt.Println(strings.Repeat("-", 72))

	llm := &mockLLM{}
	fmt.Println("  MockLLM created (no API key needed)")
	fmt.Println("  Provider name:", llm.Name())

	// 演示 ModelRequest 构建
	modelReq := &model.ModelRequest{
		System: "You are a helpful assistant.",
		ChatHistory: []model.ChatMessage{
			{Role: "user", Content: "What is the weather?"},
		},
		Model:       "mock-model",
		Temperature: 0.5,
	}
	data, err := llm.GenerateRequestData(context.Background(), modelReq)
	if err != nil {
		fmt.Println("  GenerateRequestData error:", err)
	} else {
		fmt.Println("  ModelRequest built successfully")
		fmt.Println("  Model:", data.Model)
		fmt.Println("  Messages:", len(data.Messages))
	}

	// 演示模型请求
	streamCh, err := llm.RequestModel(context.Background(), data)
	if err != nil {
		fmt.Println("  RequestModel error:", err)
	} else {
		for chunk := range streamCh {
			fmt.Println("  LLM response chunk:", chunk.Delta)
		}
	}

	fmt.Println()

	// =========================================================================
	// 第三部分: Action 层 — 注册多个工具
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 3] Action 层 / Action Layer — Tool Registration")
	fmt.Println(strings.Repeat("-", 72))

	// 创建 Action Registry
	actionRegistry := action.NewRegistry() // *action.ActionRegistry

	// 注册 add 工具
	addAction, err := action.New("add", "Add two numbers together", addNumbers)
	if err != nil {
		fmt.Println("  Failed to create add action:", err)
		return
	}
	_ = actionRegistry.Register(addAction)
	fmt.Println("  Registered action: add (func(context, AddRequest) (int, error))")

	// 注册 greet 工具
	greetAction, err := action.New("greet", "Greet someone by name", greet)
	if err != nil {
		fmt.Println("  Failed to create greet action:", err)
		return
	}
	_ = actionRegistry.Register(greetAction)
	fmt.Println("  Registered action: greet (func(GreetRequest) (string, error))")

	// 注册 weather 工具
	weatherAction, err := action.New("weather", "Get weather for a city", getWeather)
	if err != nil {
		fmt.Println("  Failed to create weather action:", err)
		return
	}
	_ = actionRegistry.Register(weatherAction)
	fmt.Println("  Registered action: weather (func(context, WeatherRequest) (map, error))")

	fmt.Println("  Total registered actions:", len(actionRegistry.List()))
	for _, name := range actionRegistry.List() {
		fmt.Println("    -", name)
	}

	// 执行 Action 演示
	ctx := context.Background()

	result, _ := actionRegistry.Execute(ctx, "add", map[string]any{"a": 10, "b": 20})
	fmt.Println("  Execute add(10, 20):", result.Result)

	result, _ = actionRegistry.Execute(ctx, "greet", map[string]any{"name": "InferGlow"})
	fmt.Println("  Execute greet('InferGlow'):", result.Result)

	result, _ = actionRegistry.Execute(ctx, "weather", map[string]any{"city": "Beijing"})
	fmt.Println("  Execute weather('Beijing'):", result.Result)

	fmt.Println()

	// =========================================================================
	// 第四部分: Session 层 — 对话记忆管理
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 4] Session 层 / Session Layer — Memory Management")
	fmt.Println(strings.Repeat("-", 72))

	// 创建 Session
	sess := session.NewSession("demo-session", 1000)
	fmt.Println("  Created session: ID=", sess.ID, "MaxLength=", sess.MaxLength)

	// 添加对话消息
	sess.AddMessage("system", "You are a helpful assistant.", "")
	sess.AddMessage("user", "Hello! What is the weather in Beijing?", "")
	sess.AddMessage("assistant", "The weather in Beijing is sunny at 25C.", "")
	sess.AddMessage("user", "What about Shanghai?", "")
	sess.AddMessage("assistant", "The weather in Shanghai is cloudy at 22C.", "")

	fmt.Println("  Added 5 messages to session")
	fmt.Println("  FullContext messages:", len(sess.FullContext))
	fmt.Println("  ContextWindow messages:", len(sess.ContextWindow))

	// 获取 Prompt
	prompt := sess.PreparePrompt()
	fmt.Println("  PreparePrompt returns", len(prompt), "messages for LLM")

	// 演示上下文窗口裁剪
	sessAuto := session.NewSession("auto-resize", 200)
	sessAuto.AutoResize = true
	sessAuto.RegisterResizeHandler("simple_cut", session.SimpleCutResizeHandler)
	sessAuto.SetDefaultResizeHandler("simple_cut")

	for i := 1; i <= 10; i++ {
		sessAuto.AddMessage("user", fmt.Sprintf("Message %d: test content.", i), "")
		sessAuto.AddMessage("assistant", fmt.Sprintf("Reply %d: acknowledged.", i), "")
	}
	fmt.Println("  Added 20 messages with MaxLength=200")
	fmt.Println("  FullContext (never trimmed):", len(sessAuto.FullContext))
	fmt.Println("  ContextWindow (auto-resized):", len(sessAuto.ContextWindow))

	fmt.Println()

	// =========================================================================
	// 第五部分: Audit 层 — 审计开关（启用/禁用）
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 5] Audit 层 / Audit Layer — Switch (Enabled/Disabled)")
	fmt.Println(strings.Repeat("-", 72))

	// --- 5.1 审计启用模式: HMAC-SHA256 签名 + 验证 ---
	fmt.Println("  --- 5.1 Audit Enabled Mode: HMAC-SHA256 ---")

	signatureKey := []byte("demo-secret-key-12345")

	enabledChain, err := audit.NewAuditChain(audit.AuditConfig{
		Enabled:      true,
		SignatureKey: signatureKey,
		MaxEntries:   100,
	})
	if err != nil {
		fmt.Println("  Failed to create audit chain:", err)
		return
	}
	fmt.Println("  Audit chain created: enabled=true, HMAC-SHA256 signing active")

	// 追加审计条目
	entries := []*audit.AuditEntry{
		{Source: "agent", Action: "decision", Input: "user request", Output: map[string]any{"action": "execute"}},
		{Source: "action", Action: "execute", Input: map[string]any{"tool": "weather"}, Output: map[string]any{"success": true}, Duration: 50 * time.Millisecond},
		{Source: "model", Action: "request", Input: "generate response", Output: "sunny day"},
	}

	for i, e := range entries {
		hash, err := enabledChain.Append(e)
		if err != nil {
			fmt.Printf("  Append[%d] failed: %v\n", i, err)
		} else {
			fmt.Printf("  Append[%d] source=%s action=%s hash=%s\n", i, e.Source, e.Action, hash[:16])
		}
	}
	fmt.Println("  Chain length:", enabledChain.Len())

	// 验证签名
	allEntries, _ := enabledChain.Query(audit.QueryFilter{})
	if len(allEntries) > 0 {
		last := allEntries[len(allEntries)-1]
		valid := audit.VerifyEntry(last, signatureKey)
		fmt.Println("  Last entry signature valid:", valid)
	}

	// 全链验证
	if err := enabledChain.VerifyChain(); err != nil {
		fmt.Println("  Chain verification FAILED:", err)
	} else {
		fmt.Println("  Chain verification: PASSED (tamper-proof)")
	}

	// 导出审计条目
	fmt.Println("  Export JSON:")
	var jsonBuf bytes.Buffer
	if err := enabledChain.Export(audit.ExportJSON, &jsonBuf); err != nil {
		fmt.Println("    Export error:", err)
	} else {
		fmt.Println("    ", jsonBuf.String()[:120]+"...")
	}

	// 将审计链挂载到 Server
	srv.SetAuditChain(enabledChain)
	fmt.Println("  Audit chain attached to server")

	fmt.Println("  --- 5.2 Audit Disabled Mode: Zero Overhead ---")

	// 创建禁用审计的 chain (cfg.Enabled=false)
	disabledChain, err := audit.NewAuditChain(audit.AuditConfig{
		Enabled: false,
	})
	if err != nil {
		fmt.Println("  Failed to create disabled audit chain:", err)
		return
	}
	fmt.Println("  Disabled audit chain created: enabled=false")

	// 禁用模式下 Append 是 no-op
	hash, _ := disabledChain.Append(&audit.AuditEntry{
		Source: "test", Action: "should-be-noop",
	})
	fmt.Println("  Append on disabled chain returned hash='' (no-op):", hash == "")

	// 禁用模式下 IsEnabled() 返回 false
	fmt.Println("  IsEnabled:", disabledChain.IsEnabled())

	// 演示 NoOpHook (零开销默认实现)
	noopHook := &audit.NoOpHook{}
	noopHash, _ := noopHook.Append(&audit.AuditEntry{Source: "test", Action: "noop"})
	fmt.Println("  NoOpHook.Append returns hash='':", noopHash == "")
	fmt.Println("  NoOpHook.IsEnabled:", noopHook.IsEnabled())

	fmt.Println()

	// =========================================================================
	// 第六部分: Flow 层 — 步骤编排与条件分支
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 6] Flow 层 / Flow Layer — Step Orchestration")
	fmt.Println(strings.Repeat("-", 72))

	// 6.1 简单线性流程
	fmt.Println("  --- 6.1 Simple Linear Flow ---")

	parseStep := flow.NewStep("parse", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		return fmt.Sprintf("Parsed: %s", text), nil
	}).Build()

	validateStep := flow.NewStep("validate", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		valid := len(text) > 5
		return fmt.Sprintf("Valid=%v: %s", valid, text), nil
	}).Build()

	formatStep := flow.NewStep("format", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		return fmt.Sprintf("Final: [%s]", text), nil
	}).Build()

	simpleFlow := flow.NewFlow().
		AddStep(parseStep).
		To(validateStep).
		To(formatStep).
		Build()

	exe := simpleFlow.Execute(ctx, "Hello World!")
	fmt.Println("  Input: 'Hello World!'")
	fmt.Println("  Status:", exe.State.Status)
	fmt.Println("  Result:", exe.State.Result)
	fmt.Println("  Steps executed:", len(exe.State.StepLog))
	for _, entry := range exe.State.StepLog {
		fmt.Printf("    - %s: %v\n", entry.StepName, entry.Output)
	}

	// 6.2 条件分支流程
	fmt.Println("  --- 6.2 Flow with Conditional Branch ---")

	analyzeStep := flow.NewStep("analyze", func(ctx context.Context, input any) (any, error) {
		num := input.(int)
		if num >= 0 {
			return map[string]any{"result": "positive", "value": num}, nil
		}
		return map[string]any{"result": "negative", "value": num}, nil
	}).Build()

	handlePositive := flow.NewStep("handle_positive", func(ctx context.Context, input any) (any, error) {
		data := input.(map[string]any)
		return fmt.Sprintf("Positive branch: %v", data["value"]), nil
	}).Build()

	handleNegative := flow.NewStep("handle_negative", func(ctx context.Context, input any) (any, error) {
		data := input.(map[string]any)
		return fmt.Sprintf("Negative branch: %v", data["value"]), nil
	}).Build()

	branchFlow := flow.NewFlow().
		AddStep(analyzeStep).
		If(func(output any) bool {
			data := output.(map[string]any)
			return data["result"] == "positive"
		}, handlePositive, handleNegative).
		Build()

	posExe := branchFlow.Execute(ctx, 42)
	fmt.Println("  Input: 42 (positive)")
	for _, entry := range posExe.State.StepLog {
		fmt.Printf("    - %s: %v\n", entry.StepName, entry.Output)
	}

	negExe := branchFlow.Execute(ctx, -10)
	fmt.Println("  Input: -10 (negative)")
	for _, entry := range negExe.State.StepLog {
		fmt.Printf("    - %s: %v\n", entry.StepName, entry.Output)
	}

	// 6.3 声明式 FlowDef 注册到 Server
	fmt.Println("  --- 6.3 Declarative FlowDef via Stage Registry ---")

	// 创建 FlowDef (使用已在 server 初始化时注册的 greet stage)
	flowDef := &flowdef.FlowDef{
		APIVersion: "v1",
		Kind:       "Flow",
		Metadata: flowdef.Metadata{
			Name:        "demo-pipeline",
			Version:     "1.0.0",
			Description: "A demo pipeline flow",
		},
		Spec: flowdef.Spec{
			Inputs: []flowdef.InputDef{
				{Name: "name", Type: "string", Required: true},
			},
			Steps: []flowdef.StepDef{
				{
					Name:     "greet-step",
					Operator: "stage",
					Stage:    "greet",
					Inputs:   map[string]any{"name": "${inputs.name}"},
				},
			},
			Outputs: map[string]string{
				"result": "${steps.greet-step.greeting}",
			},
		},
	}

	// 验证 FlowDef
	if err := flowdef.Validate(flowDef); err != nil {
		fmt.Println("  FlowDef validation error:", err)
	} else {
		fmt.Println("  FlowDef validated: name=", flowDef.Metadata.Name)
		fmt.Println("  Steps:", len(flowDef.Spec.Steps))
	}

	fmt.Println()

	// =========================================================================
	// 第七部分: Schema 层 — 输出 Schema 定义与验证
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 7] Schema 层 / Schema Layer — Output Definition & Validation")
	fmt.Println(strings.Repeat("-", 72))

	// 7.1 泛型方法推导 Schema
	fmt.Println("  --- 7.1 Generic DefineOutput ---")

	type WeatherResponse struct {
		City     string  `json:"city" description:"City name"`
		Temp     float64 `json:"temp" description:"Temperature in Celsius"`
		Humidity int     `json:"humidity,omitempty" description:"Humidity percentage"`
	}

	weatherSchema := schema.DefineOutput[WeatherResponse]()
	fmt.Println("  WeatherResponse schema:")
	fmt.Println("    EnsureAll:", weatherSchema.EnsureAll)
	fmt.Println("    Format:", weatherSchema.Format)
	for name, field := range weatherSchema.Fields {
		fmt.Printf("    Field '%s': Type=%s, Required=%v, Desc=%s\n",
			name, field.Type, field.Required, field.Description)
	}

	// 7.2 JSON Schema 转换
	fmt.Println("  --- 7.2 JSON Schema Conversion ---")

	jsonSchema := schema.GenerateJSONSchema(weatherSchema)
	fmt.Println("  JSON Schema type:", jsonSchema["type"])
	fmt.Println("  JSON Schema required:", jsonSchema["required"])
	props := jsonSchema["properties"].(map[string]any)
	for name, prop := range props {
		p := prop.(map[string]any)
		fmt.Printf("    property '%s': type=%v\n", name, p["type"])
	}

	// 7.3 手动构建 Schema
	fmt.Println("  --- 7.3 Manual Schema Construction ---")

	customSchema := &schema.OutputSchema{
		Format:    schema.OutputJSON,
		EnsureAll: true,
		Fields: map[string]*schema.FieldDef{
			"message": {Type: schema.TypeString, Description: "Response content", Required: true},
			"code":    {Type: schema.TypeInt, Description: "Error code", Required: false},
		},
	}
	fmt.Println("  Custom schema: Format=", customSchema.Format)
	for name, field := range customSchema.Fields {
		fmt.Printf("    Field '%s': Type=%s, Required=%v\n", name, field.Type, field.Required)
	}

	fmt.Println()

	// =========================================================================
	// 第八部分: Sandbox 层 — 沙箱执行
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 8] Sandbox 层 / Sandbox Layer — Isolated Execution")
	fmt.Println(strings.Repeat("-", 72))

	// 创建 Sandbox Manager
	mgr := sandbox.NewManager()
	_ = mgr.Register(sandbox.NewTrustedLocalProvider())
	fmt.Println("  Sandbox Manager created")
	fmt.Println("  Registered TrustedLocalProvider")

	// 创建 SandboxExecutor (stub when built without -tags with_sandbox)
	sandboxExec := action.NewSandboxExecutor(action.SandboxExecutorConfig{})

	// 执行沙箱命令
	sandboxResult, err := sandboxExec.Execute(ctx, map[string]any{
		"argv":    []string{"echo", "hello from sandbox"},
		"timeout": "5s",
	})
	if err != nil {
		fmt.Println("  Sandbox execute error:", err)
	} else {
		fmt.Println("  Sandbox result status:", sandboxResult.Status)
		fmt.Println("  Sandbox result OK:", sandboxResult.OK)
		if sandboxResult.OK {
			if r, ok := sandboxResult.Result.(map[string]any); ok {
				fmt.Println("  Sandbox stdout:", r["stdout"])
			}
		}
	}
	if sandboxResult != nil && !sandboxResult.OK {
		fmt.Println("  (Sandbox requires -tags with_sandbox for real execution)")
	}

	fmt.Println()

	// =========================================================================
	// 第九部分: Workspace 层 — 安全文件操作与路径穿越防护
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 9] Workspace 层 / Workspace Layer — Safe File Operations")
	fmt.Println(strings.Repeat("-", 72))

	// 创建临时工作目录
	tmpDir := filepath.Join(os.TempDir(), "inferglow-workspace-demo")
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// 创建 Workspace
	ws, err := workspace.New(workspace.Config{
		RootDir:      tmpDir,
		MaxFileSize:  1024 * 1024,
		MaxFileCount: 100,
		ReadOnly:     false,
	})
	if err != nil {
		fmt.Println("  Failed to create workspace:", err)
		return
	}
	fmt.Println("  Workspace root:", ws.Root())

	// 9.1 路径穿越防护
	fmt.Println("  --- 9.1 Path Traversal Protection ---")

	paths := []string{
		"data/file.txt",
		"../../etc/passwd",
		"/etc/passwd",
	}
	for _, p := range paths {
		abs, err := ws.SafePath(p)
		if err != nil {
			fmt.Printf("    [REJECTED] %-25q -> %v\n", p, err)
		} else {
			fmt.Printf("    [ALLOWED]  %-25q -> %s\n", p, abs)
		}
	}

	// 9.2 安全文件 IO
	fmt.Println("  --- 9.2 Safe File I/O ---")

	if err := ws.MkdirAll("data"); err != nil {
		fmt.Println("  MkdirAll error:", err)
	} else {
		fmt.Println("  Created directory: data/")
	}

	if err := ws.WriteFile("data/hello.txt", []byte("Hello from InferGlow workspace!\n")); err != nil {
		fmt.Println("  WriteFile error:", err)
	} else {
		fmt.Println("  Written file: data/hello.txt")
	}

	content, err := ws.ReadFile("data/hello.txt")
	if err != nil {
		fmt.Println("  ReadFile error:", err)
	} else {
		fmt.Println("  Read file content:", string(content))
	}

	files, err := ws.ListDir("data")
	if err != nil {
		fmt.Println("  ListDir error:", err)
	} else {
		fmt.Println("  Directory listing:", files)
	}

	// 9.3 文件血缘管理
	fmt.Println("  --- 9.3 File Lineage Tracking ---")

	lineage := workspace.NewMemoryLineageStore()

	_ = lineage.Record(workspace.LineageNode{
		Path:      "data/raw.txt",
		Operation: "write",
		CreatedBy: "ingest",
	})
	_ = lineage.Record(workspace.LineageNode{
		Path:      "data/processed.txt",
		Operation: "transform",
		CreatedBy: "processor",
		Parents:   []string{"data/raw.txt"},
	})
	_ = lineage.Record(workspace.LineageNode{
		Path:      "data/report.txt",
		Operation: "transform",
		CreatedBy: "reporter",
		Parents:   []string{"data/processed.txt"},
	})

	fmt.Println("  Lineage records:", lineage.Size())

	ancestors, _ := lineage.Ancestors("data/report.txt")
	fmt.Println("  Ancestors of report.txt:", ancestors)

	descendants, _ := lineage.Descendants("data/raw.txt")
	fmt.Println("  Descendants of raw.txt:", descendants)

	fmt.Println()

	// =========================================================================
	// 第十部分: Server REST API 调用
	// =========================================================================
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("  [Part 10] Server REST API 调用 / REST API Calls")
	fmt.Println(strings.Repeat("-", 72))

	// 使用 httptest 创建测试服务器
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 10.1 Health Check
	fmt.Println("  --- 10.1 Health Check ---")
	resp := doGet(ts, "/health")
	fmt.Println("  GET /health -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.2 List Agents (empty)
	fmt.Println("  --- 10.2 List Agents (empty) ---")
	resp = doGet(ts, "/v1/agents")
	fmt.Println("  GET /v1/agents -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.3 Create Agent
	fmt.Println("  --- 10.3 Create Agent ---")
	resp = doPost(ts, "/v1/agents", map[string]any{
		"name": "demo-agent",
	})
	fmt.Println("  POST /v1/agents -> status:", resp.StatusCode)
	agentResp := printJSON(resp)

	// 10.4 Chat with Agent
	fmt.Println("  --- 10.4 Chat with Agent ---")
	if agentID, ok := agentResp["id"].(string); ok {
		chatBody := map[string]any{"message": "Hello, what can you do?"}
		resp = doPost(ts, "/v1/agents/"+agentID+"/chat", chatBody)
		fmt.Println("  POST /v1/agents/{id}/chat -> status:", resp.StatusCode)
		printJSON(resp)
	}

	// 10.5 List Agents (after creation)
	fmt.Println("  --- 10.5 List Agents (after creation) ---")
	resp = doGet(ts, "/v1/agents")
	fmt.Println("  GET /v1/agents -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.6 List Tools
	fmt.Println("  --- 10.6 List Tools ---")
	resp = doGet(ts, "/v1/tools")
	fmt.Println("  GET /v1/tools -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.7 Create Memory
	fmt.Println("  --- 10.7 Create Memory ---")
	resp = doPost(ts, "/v1/memories", map[string]any{
		"content":  "InferGlow is an agent framework written in Go.",
		"category": "knowledge",
		"facts":    []string{"Go-based", "modular", "extensible"},
	})
	fmt.Println("  POST /v1/memories -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.8 Search Memories
	fmt.Println("  --- 10.8 Search Memories ---")
	resp = doGet(ts, "/v1/memories?q=InferGlow")
	fmt.Println("  GET /v1/memories?q=InferGlow -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.9 Register a Flow via REST API
	fmt.Println("  --- 10.9 Register Flow via REST API ---")
	flowYAML := `api_version: v1
kind: Flow
metadata:
  name: rest-demo-flow
  version: "1.0"
  description: "Flow registered via REST API"
spec:
  inputs:
    - name: message
      type: string
      required: true
  steps:
    - name: echo-step
      operator: stage
      stage: echo
      inputs:
        message: "${inputs.message}"
  outputs:
    result: "${steps.echo-step.message}"
`
	resp = doPostRaw(ts, "/v1/flows", "application/x-yaml", []byte(flowYAML))
	fmt.Println("  POST /v1/flows -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.10 List Flows
	fmt.Println("  --- 10.10 List Flows ---")
	resp = doGet(ts, "/v1/flows")
	fmt.Println("  GET /v1/flows -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.11 List Stages
	fmt.Println("  --- 10.11 List Stages ---")
	resp = doGet(ts, "/v1/stages")
	fmt.Println("  GET /v1/stages -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.12 Audit Verify (with audit enabled)
	fmt.Println("  --- 10.12 Audit Verify ---")
	resp = doGet(ts, "/v1/audit/verify")
	fmt.Println("  GET /v1/audit/verify -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.13 Audit Entries
	fmt.Println("  --- 10.13 Audit Entries ---")
	resp = doGet(ts, "/v1/audit/entries?limit=10")
	fmt.Println("  GET /v1/audit/entries -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.14 Get Session
	fmt.Println("  --- 10.14 Get Session ---")
	resp = doGet(ts, "/v1/sessions/demo-session")
	fmt.Println("  GET /v1/sessions/{id} -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.15 OpenAPI Spec
	fmt.Println("  --- 10.15 OpenAPI Spec ---")
	resp = doGet(ts, "/openapi.json")
	fmt.Println("  GET /openapi.json -> status:", resp.StatusCode)
	printJSON(resp)

	// 10.16 Delete Agent
	fmt.Println("  --- 10.16 Delete Agent ---")
	if agentID, ok := agentResp["id"].(string); ok {
		resp = doDelete(ts, "/v1/agents/"+agentID)
		fmt.Println("  DELETE /v1/agents/{id} -> status:", resp.StatusCode)
		printJSON(resp)
	}

	fmt.Println()

	// =========================================================================
	// 总结
	// =========================================================================
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println("  Summary / 总结")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println("  Layers demonstrated:")
	fmt.Println("    1. Server initialization and configuration")
	fmt.Println("    2. Model layer: MockLLM Provider")
	fmt.Println("    3. Action layer: Tool registration (add, greet, weather)")
	fmt.Println("    4. Session layer: Conversation memory management")
	fmt.Println("    5. Audit layer: Enabled (HMAC-SHA256) and Disabled (no-op)")
	fmt.Println("    6. Flow layer: Linear flow, conditional branching, FlowDef")
	fmt.Println("    7. Schema layer: Output definition, JSON Schema conversion")
	fmt.Println("    8. Sandbox layer: Isolated execution (stub w/o build tag)")
	fmt.Println("    9. Workspace layer: Safe file I/O, path traversal protection")
	fmt.Println("   10. Server REST API: 16 endpoint calls via httptest")
	fmt.Println()
	fmt.Println("  Audit switch: enabled=true (HMAC-SHA256), enabled=false (zero overhead)")
	fmt.Println("  Sandbox mode: requires -tags with_sandbox for real execution")
	fmt.Println()
	fmt.Println("  All examples completed successfully!")
}

// ============================================================================
// HTTP 辅助函数
// ============================================================================

func doGet(ts *httptest.Server, path string) *http.Response {
	resp, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		panic(fmt.Sprintf("GET %s failed: %v", path, err))
	}
	return resp
}

func doPost(ts *httptest.Server, path string, body any) *http.Response {
	data, _ := json.Marshal(body)
	resp, err := ts.Client().Post(ts.URL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		panic(fmt.Sprintf("POST %s failed: %v", path, err))
	}
	return resp
}

func doPostRaw(ts *httptest.Server, path, contentType string, body []byte) *http.Response {
	resp, err := ts.Client().Post(ts.URL+path, contentType, bytes.NewReader(body))
	if err != nil {
		panic(fmt.Sprintf("POST %s failed: %v", path, err))
	}
	return resp
}

func doDelete(ts *httptest.Server, path string) *http.Response {
	req, _ := http.NewRequest("DELETE", ts.URL+path, nil)
	resp, err := ts.Client().Do(req)
	if err != nil {
		panic(fmt.Sprintf("DELETE %s failed: %v", path, err))
	}
	return resp
}

func printJSON(resp *http.Response) map[string]any {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		// Try array
		var arr []any
		if err2 := json.Unmarshal(body, &arr); err2 != nil {
			fmt.Println("    Response:", string(body))
			return nil
		}
		fmt.Printf("    Response (array, len=%d): %v\n", len(arr), truncJSON(fmt.Sprintf("%v", arr)))
		return nil
	}
	fmt.Println("    Response:", truncJSON(string(body)))
	return result
}

func truncJSON(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
