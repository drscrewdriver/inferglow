# 07 · orchestrator 模块

## 一、职责

`orchestrator` 模块（`github.com/inferglow/orchestrator`）是整个框架的**编排层**，也是把基础模块（model / action / session / audit / flow）粘合在一起的上层胶水（security 通过接口注入，sandbox 通过 build tag 可选）。它提供：

- `agent.Agent`：用户入口，封装 Session + Action + ModelRequester
- `agent.Engine`：PLAN-EXECUTE 循环引擎
- `actionruntime.ActionDispatcher`：并发执行 Action + 审计
- `actionruntime.ParseDecision`：LLM 输出解析为结构化 Decision
- `agent.LoopGuard`：死循环检测
- `agent.TurnLoop` / `CancelManager`：轮次状态机 + 取消管理

依赖：`action` `audit` `model` `session` `flow`（`security` 已解耦为接口注入，`sandbox` 通过 `with_sandbox` build tag 可选；见 [orchestrator/go.mod](../../orchestrator/go.mod)）。orchestrator 通过 `FlowContext` 接口将横切能力注入 flow 步骤，支持 `WithFlow` RunOption 切换到 flow 编排模式。

## 二、agent 子包

### 2.1 Agent（用户入口）（[agent.go](../../orchestrator/agent/agent.go)）

```go
type Agent struct {
    session       *SessionExtension     // 桥接 session.Session
    actionExt     *ActionExtension      // 桥接 action.ActionRegistry
    engine        *Engine               // PLAN-EXECUTE 引擎
    maxRounds     int                   // 持久化默认（默认 10）
    systemPrompt  string
    streamTimeout time.Duration         // 流超时（默认 5min）
    outputHook    OutputSecurityHook    // 输出侧注入检测（接口，实现在 security/agenthook）
    piiMasker     PIIMasker             // PII 脱敏器（接口，实现在 security/agenthook）
    flow          *flow.Flow            // 可选 flow 编排定义（WithFlow，nil 时走 executeLoop）
    outputSchema  *model.OutputSchema   // 可选 L4 输出 schema（WithOutputSchema，nil 时禁用 L4）
}

func New(sess *session.Session, actionExt *ActionExtension, modelReq model.ModelRequester, opts ...RunOption) *Agent

func (a *Agent) Run(ctx context.Context, userMessage string, opts ...RunOption) (string, error)
```

#### RunOption（函数式选项）

```go
type RunOption func(*runConfig)

func WithMaxRounds(n int) RunOption                  // 最大循环轮数
func WithSystemPrompt(prompt string) RunOption       // system prompt
func WithStreamTimeout(d time.Duration) RunOption    // 流超时
func WithPIIMasker(m PIIMasker) RunOption            // PII 脱敏（接口，实现在 security/agenthook）
func WithFlow(f *flow.Flow) RunOption                // 切换到 flow 编排模式（与 executeLoop 互斥）
func WithOutputSchema(s *model.OutputSchema) RunOption // L4 后置校验 schema（nil 禁用）
```

### 2.2 Engine（循环引擎）（[engine.go](../../orchestrator/agent/engine.go)）

```go
type Engine struct {
    session       *SessionExtension
    actionExt     *ActionExtension
    modelReq      model.ModelRequester
    auditHook     audit.AuditHook      // 默认 NoOpHook
    loopGuard     *LoopGuard           // nil 时禁用
    streamTimeout time.Duration
    toolDefsHash  string               // 工具定义的 SHA-256（缓存失效用）
    turnLoop      *TurnLoop            // PLAN/ACTIVE/IDLE 状态机
    cancelManager *CancelManager       // 取消管理
    outputSchema  *model.OutputSchema  // L4 后置校验 schema（nil 时跳过 L3/L4）
}

// 四个构造函数（渐进式启用特性）
func NewEngine(sess, actExt, mr) *Engine                                    // 无审计无 LoopGuard
func NewEngineWithAudit(sess, actExt, mr, hook) *Engine                     // +审计
func NewEngineWithLoopGuard(sess, actExt, mr, guard) *Engine                // +LoopGuard
func NewEngineWithAuditAndLoopGuard(sess, actExt, mr, hook, guard) *Engine  // +两者
```

### 2.3 桥接扩展

#### SessionExtension（[session_ext.go](../../orchestrator/agent/session_ext.go)）

```go
type SessionExtension struct {
    s *session.Session
}

func (e *SessionExtension) AddUserMessage(content string)
func (e *SessionExtension) AddAssistantMessage(content string)
func (e *SessionExtension) AddActionResult(actionName string, result *action.ActionResult)
func (e *SessionExtension) PreparePrompt() []model.ChatMessage   // 历史转 prompt
func (e *SessionExtension) SetMessageMasker(m session.MessageMasker)
```

> **关键约束**（编译期检查）：`var _ session.MessageMasker = (*pii.Masker)(nil)` —— 确保 `pii.Masker` 实现 `session.MessageMasker` 接口。

#### ActionExtension（[action_ext.go](../../orchestrator/agent/action_ext.go)）

```go
type ActionExtension struct {
    registry *action.ActionRegistry
}

func NewActionExtension() *ActionExtension
func (e *ActionExtension) Register(a *action.Action) error
func (e *ActionExtension) ListActions() []map[string]any      // 转为 tool 定义格式
func (e *ActionExtension) GetRegistry() *action.ActionRegistry
func (e *ActionExtension) Execute(ctx, name, input) (*action.ActionResult, error)
```

### 2.4 LoopGuard（死循环检测）（[loop_guard.go](../../orchestrator/agent/loop_guard.go)）

```go
type LoopGuardConfig struct {
    Disabled                  bool
    RepeatActionWindow        int           // 默认 3
    OutputStagnationWindow    int           // 默认 3
    OutputSimilarityThreshold float64       // 默认 0.9
    TimeBudget                time.Duration // 默认 5min
    TokenBudget               int           // 默认 100000
}

type LoopGuardState struct {
    Round       int
    ActionCalls []actionruntime.ActionCall
    LastOutput  string
    TotalTokens int
    StartedAt   time.Time
}

type LoopGuardVerdict struct {
    Action VerdictAction   // "continue" | "break" | "degrade"
    Reason string
}

func NewLoopGuard(cfg LoopGuardConfig) *LoopGuard
func (g *LoopGuard) Check(state LoopGuardState) (*LoopGuardVerdict, error)
func (g *LoopGuard) Reset()
```

#### 四种检测策略（按优先级顺序）

| 优先级 | 策略 | 触发条件 | 默认参数 |
|:------:|------|---------|---------|
| 1 | **RepeatAction** | 连续 N 轮 ActionCall 完全相同 | N=3 |
| 2 | **OutputStagnation** | 连续 N 轮输出 Jaccard 相似度 > 阈值 | N=3, 阈值=0.9 |
| 3 | **TimeBudget** | 总耗时超过上限 | 5min |
| 4 | **TokenBudget** | 累计 token 超过上限 | 100000 |

首个命中的策略决定 verdict，后续不再检查。

### 2.5 TurnLoop / CancelManager

| 文件 | 内容 |
|------|------|
| [turn_loop.go](../../orchestrator/agent/turn_loop.go) | `TurnLoop` 状态机：`Idle → Planning → Active → Idle`，支持抢占 |
| [cancel.go](../../orchestrator/agent/cancel.go) | `CancelManager` 取消管理：`CancelImmediate` / `CancelAfterChatModel` / `CancelAfterToolCalls` 三种安全点 |
| [streaming.go](../../orchestrator/agent/streaming.go) | 流式输出支持 |
| [chatmodel_agent.go](../../orchestrator/agent/chatmodel_agent.go) | ChatModel Agent 变体 |
| [security_hook.go](../../orchestrator/agent/security_hook.go) | `OutputSecurityHook` 接口 |
| [ratelimit_hook.go](../../orchestrator/agent/ratelimit_hook.go) | `RateLimitHook` 接口 |

---

## 三、actionruntime 子包

### 3.1 核心类型（[types.go](../../orchestrator/actionruntime/types.go)）

```go
// 单次 Action 调用
type ActionCall struct {
    Name   string         `json:"name"`
    Params map[string]any `json:"params,omitempty"`
}

// LLM 规划决策
type Decision struct {
    NextAction    string       `json:"next_action"`    // "execute" | "response"
    ActionCalls   []ActionCall `json:"action_calls,omitempty"`
    FinalResponse string       `json:"final_response,omitempty"`
}
```

### 3.2 ActionDispatcher（[dispatcher.go](../../orchestrator/actionruntime/dispatcher.go)）

```go
type ActionDispatcher struct {
    registry  *action.ActionRegistry
    auditHook audit.AuditHook
}

func NewActionDispatcher(r *action.ActionRegistry) *ActionDispatcher
func NewActionDispatcherWithAudit(r *action.ActionRegistry, hook audit.AuditHook) *ActionDispatcher

// 并发执行所有 ActionCall，返回有序结果
func (d *ActionDispatcher) Execute(ctx context.Context, calls []ActionCall) []*action.ActionResult
```

#### 并发执行 + panic 恢复

`Execute` 为每个 ActionCall 启动一个 goroutine，用 `sync.WaitGroup` 等待全部完成：

```
for i, call := range calls {
    go func(idx, c) {
        defer wg.Done()
        defer recover() → 合成 "panic" ActionResult + 审计条目

        result, err := registry.Execute(ctx, c.Name, c.Params)
        results[idx] = result 或 error-shaped ActionResult

        if auditHook != nil:
            auditHook.Append(AuditEntry{Source:"action", Action:"execute", ...})
    }(i, call)
}
wg.Wait()
```

> **关键设计**：审计失败不阻断 Action 执行（`_, _ = auditHook.Append(entry)`）。panic 被恢复为结构化 `ActionResult`，避免 nil 指针 panic 上游。

### 3.3 ParseDecision（[planning.go](../../orchestrator/actionruntime/planning.go)）

```go
func ParseDecision(content string) (*Decision, error)
func RepairLLMJSON(input string) string
```

#### LLM 输出修复管道

LLM 输出常带 markdown 代码块、前后噪声文本、尾随逗号。`RepairLLMJSON` 按顺序修复：

```
RepairLLMJSON(input)
    │
    ├──[1] 剥离 markdown 代码围栏: ```json ... ``` → 内部内容
    │
    ├──[2] 提取首个平衡的 {...} 对象
    │       (忽略字符串字面量内的花括号)
    │       处理 LLM 前后缀噪声: "Sure! Here is: {...}"
    │
    └──[3] 移除尾随逗号: ",}" → "}", ",]" → "]"
            (循环 16 次上限，处理字符串字面量内的逗号不误伤)
```

然后 `json.Unmarshal` 解析为 `Decision`，校验 `next_action` 为 `"execute"` 或 `"response"`。

### 3.4 ShouldContinue（[flow.go](../../orchestrator/actionruntime/flow.go)）

```go
func ShouldContinue(decision Decision, roundIndex, maxRounds int) bool
```

循环继续条件（全部满足）：
1. `roundIndex < maxRounds`（maxRounds > 0 时）
2. `decision.NextAction == "execute"`
3. `len(decision.ActionCalls) > 0`

---

## 四、executeLoop 完整逻辑（[engine.go](../../orchestrator/agent/engine.go) L163-L475）

这是整个框架的核心函数，逐行解析：

```
engine.executeLoop(ctx, userMessage, maxRounds, systemPrompt) → (*Decision, error)
    │
    ├── session.AddUserMessage(userMessage)
    │
    ├── defer: 确保 TurnLoop 回到 Idle
    │
    ├── round := 0
    ├── for {
    │     │
    │     ├──[1] LoopGuard.Check(state)                    ← L187-210
    │     │       ├── VerdictBreak   → return ErrLoopDetected
    │     │       ├── VerdictDegrade → systemPrompt += degrade 提示
    │     │       └── VerdictContinue → 继续
    │     │
    │     ├──[2] CancelManager 检查 (CancelImmediate)      ← L218-225
    │     │       pending → return "agent cancelled"
    │     │
    │     ├──[3] TurnLoop.EnterPlanning() → preemptCh       ← L231
    │     │
    │     ├──[4] buildToolDefinitions()                     ← L235
    │     │       actionExt.ListActions() → []model.ToolDefinition
    │     │       排序 + 计算 SHA-256 (toolDefsHash, 缓存失效用)
    │     │
    │     ├──[5] 构造 ModelRequest{                         ← L235-275
    │     │       System: systemPrompt,
    │     │       ChatHistory: session.PreparePrompt(),
    │     │       Tools: tools,
    │     │       Output: {next_action, action_calls, final_response},
    │     │       Options: {"force_json": true},
    │     │   }
    │     │
    │     ├──[5a] L3 兜底 prompt 注入                        ← L277 (NEW)
    │     │       if e.outputSchema != nil && shouldInjectSchemaPrompt(req):
    │     │         req.System += formatSchemaInstruction(req.Output)
    │     │       ← 仅当 provider 不支持 json_schema 级 response_format 时启用
    │     │
    │     ├──[6] modelReq.GenerateRequestData(ctx, req)     ← L282
    │     │
    │     ├──[7] timeoutCtx (默认 5min)                     ← L297
    │     │       modelReq.RequestModel(timeoutCtx, data) → <-chan *StreamChunk  ← L298
    │     │
    │     ├──[8] streamLoop: 收集 content                   ← L302-335
    │     │       select {
    │     │         case chunk := <-stream: content.WriteString(chunk.Delta)
    │     │         case <-timeoutCtx.Done(): return timeoutCtx.Err()
    │     │         case <-preemptCh: return "agent preempted"
    │     │       }
    │     │
    │     ├──[8a] L4 后置校验 + 重试                         ← L340-375 (NEW)
    │     │       if e.outputSchema != nil:
    │     │         validator := model.NewOutputValidator(e.outputSchema)
    │     │         validator.MaxRetries = 2
    │     │         validatedResp, err := validator.ValidateAndRetryWithFetch(ctx, fetcher)
    │     │       ← fetcher 首次返回已收集 content，重试时重新调用 GenerateRequestData + RequestModel
    │     │       ← 失败 → return "L4 output validation failed after retries: ..."
    │     │       ← 成功 → content 替换为 validatedResp.Content
    │     │
    │     ├──[9] totalTokens += len(content)                ← L379
    │     │
    │     ├──[10] ParseDecision(content)                    ← L382
    │     │       失败 → 降级为 {NextAction:"response", FinalResponse:rawContent}
    │     │
    │     ├──[11] auditHook.Append(AuditEntry{Source:"agent", Action:"decision"})  ← L400-415
    │     │
    │     ├──[12] ShouldContinue(decision, round, maxRounds)?  ← L419
    │     │       false → return decision
    │     │
    │     ├──[13] CancelManager 检查 (CancelAfterChatModel)   ← L429
    │     │
    │     ├──[14] TurnLoop.EnterActive()                      ← L439
    │     │
    │     ├──[15] dispatcher.Execute(ctx, decision.ActionCalls)  ← L443-445
    │     │
    │     ├──[16] for i, call := range decision.ActionCalls:     ← L449-453
    │     │       session.AddActionResult(call.Name, results[i])
    │     │
    │     ├──[17] CancelManager 检查 (CancelAfterToolCalls)      ← L461
    │     │
    │     ├──[18] TurnLoop.EnterIdle()                            ← L470
    │     │
    │     └── round++                                             ← L473
    │   }
```

## 五、Agent.Run 完整逻辑（[agent.go](../../orchestrator/agent/agent.go) L240-L290）

```
Agent.Run(ctx, userMessage, opts) → (string, error)
    │
    ├──[1] 合并配置: Agent 默认 + per-call opts          ← L247-254
    │       (maxRounds, systemPrompt, streamTimeout, outputHook, piiMasker, outputSchema)
    │
    ├──[2] engine.streamTimeout = c.streamTimeout        ← L268
    │       engine.outputSchema = c.outputSchema          ← L272 (NEW: 传播 L4 schema)
    │
    ├──[3] if piiMasker != nil:
    │       session.SetMessageMasker(piiMasker)  (输入侧脱敏)
    │
    ├──[4] decision, err := engine.executeLoop(ctx, userMessage, maxRounds, systemPrompt)
    │
    ├──[5] if decision.NextAction == "response":
    │       │
    │       ├── if outputHook != nil:
    │       │     outputHook.CheckOutput(decision.FinalResponse)
    │       │     (注入检测，error 阻断返回)
    │       │
    │       ├── response := decision.FinalResponse
    │       │
    │       ├── if piiMasker != nil:
    │       │     response = piiMasker.MaskOutput(response)  (输出侧脱敏)
    │       │
    │       ├── session.AddAssistantMessage(response)
    │       │
    │       └── return response, nil
    │
    └──[6] return "", ErrNoFinalResponse
```

## 六、取消安全点

`CancelManager` 定义三种安全点，取消请求可指定在哪个安全点生效：

| 安全点 | 检查时机（executeLoop 中） | 行为 |
|--------|---------------------------|------|
| `CancelImmediate` | 每轮开头 + 所有安全点 | 立即中断 |
| `CancelAfterChatModel` | LLM 返回后、工具执行前 | 返回当前 decision |
| `CancelAfterToolCalls` | 工具执行后、下一轮前 | 返回 error |

超时升级：`CheckTimeoutEscalation()` 可将安全点取消升级为 `CancelImmediate`。

## 七、模块间调用关系总览

```
Agent.Run
    │
    ├── session.Session (via SessionExtension)
    │     ├── AddUserMessage / AddAssistantMessage / AddActionResult
    │     ├── PreparePrompt
    │     └── SetMessageMasker (pii.Masker)
    │
    ├── Engine.executeLoop
    │     ├── LoopGuard.Check
    │     ├── TurnLoop (Planning/Active/Idle)
    │     ├── CancelManager
    │     ├── ActionExtension.ListActions → model.ToolDefinition
    │     ├── model.ModelRequester
    │     │     ├── GenerateRequestData
    │     │     └── RequestModel (stream)
    │     ├── actionruntime.ParseDecision
    │     ├── actionruntime.ShouldContinue
    │     ├── audit.AuditHook.Append (decision 条目)
    │     └── ActionDispatcher.Execute
    │           ├── action.ActionRegistry.Execute
    │           │     └── ActionExecutor.Execute
    │           │           (LocalFunctionExecutor / SandboxExecutor / MCPExecutor)
    │           └── audit.AuditHook.Append (action 条目)
    │
    ├── OutputSecurityHook.CheckOutput (prompt_injection.Detector)
    └── pii.Masker.MaskOutput
```

## 八、可插拔架构改进（v2）：orchestrator/agent 安全接口注入

v2 起 `orchestrator/agent` 不再直接 import `security/pii` 与 `security/prompt_injection`，改为仅保留接口契约，实现移至 `security/agenthook` 子包。`Agent` 结构体的 `piiMasker` 字段也从 `*pii.Masker` 改为 `PIIMasker` 接口类型。

### 8.1 接口契约与实现位置

| 接口 | 定义位置 | 实现位置 | 注入入口 |
|------|---------|---------|---------|
| `agent.OutputSecurityHook` | `orchestrator/agent/security_hook.go` | `security/agenthook.OutputInjectionHook` | `agent.WithOutputSecurityHook(hook)` |
| `agent.PIIMasker` | `orchestrator/agent/agent.go` | `security/agenthook.PIIMasker`（适配 `*pii.Masker`） | `agent.WithPIIMasker(m)` |

### 8.2 变更要点

**`orchestrator/agent/agent.go`**：
- 移除 `import "github.com/inferglow/security/pii"`
- `piiMasker` 字段类型从 `*pii.Masker` 改为 `PIIMasker` 接口
- `WithPIIMasker` 选项签名改为接受 `PIIMasker` 接口
- 新增 `PIIMasker` 接口定义（`MaskInput(text string) string` / `MaskOutput(text string) string`），方法集与 `session.MessageMasker` 一致

**`orchestrator/agent/security_hook.go`**：
- 移除 `import promptinjection "github.com/inferglow/security/prompt_injection"`
- 移除 `OutputInjectionHook` 结构体实现
- 保留 `OutputSecurityHook` 接口与 `ErrOutputInjectionBlocked` 错误变量

**`security/agenthook/hook.go`**：`OutputInjectionHook` 实现 + `NewOutputInjectionHook(cfg)` + 编译期断言 `var _ agent.OutputSecurityHook = (*OutputInjectionHook)(nil)`

**`security/agenthook/pii_adapter.go`**：`PIIMasker` 适配器包装 `*pii.Masker` + `NewPIIMasker(m)` + 编译期断言 `var _ agent.PIIMasker = (*PIIMasker)(nil)`

### 8.3 依赖方向

```
security/agenthook  →  orchestrator/agent   （实现 OutputSecurityHook / PIIMasker）
security/agenthook  →  security/pii          （PIIMasker 适配器包装 *pii.Masker）
security/agenthook  →  security/prompt_injection
```

`orchestrator/agent` 对 `security` 完全无感知。不注入 `outputHook` / `piiMasker` 时，`Agent.Run` 跳过对应检查，**零开销**。

### 8.4 注入示例

```go
import (
    "github.com/inferglow/orchestrator/agent"
    "github.com/inferglow/security/agenthook"
    "github.com/inferglow/security/pii"
    promptinjection "github.com/inferglow/security/prompt_injection"
)

outHook := agenthook.NewOutputInjectionHook(promptinjection.NewDefaultConfig())
piiMasker := agenthook.NewPIIMasker(pii.NewMasker(pii.MaskConfig{}))

ag := agent.New(sess, actExt, llm,
    agent.WithOutputSecurityHook(outHook),
    agent.WithPIIMasker(piiMasker),
)
```
