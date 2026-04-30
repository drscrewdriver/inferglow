# Inferglow 完整调用链与架构分析

## 一、Session、Action、Flow 的关系澄清

### 核心结论：三者完全独立，由上层 Agently 主模块串联

| 模块 | 职责 | 独立性 | 是否调用其他 inferglow 模块 |
|------|------|--------|--------------------------|
| **model** | LLM Provider 统一抽象 | 最底层 | 无 |
| **schema** | 契约优先的 Structured Output | 依赖 model | schema → model |
| **flow** | 步骤编排引擎（线性/事件驱动） | 依赖 schema | flow → schema |
| **action** | 工具注册与执行框架 | 完全独立 | 无 |
| **session** | 对话记忆管理（双列表） | 完全独立 | 无 |
| **sandbox** | 隔离执行环境 | 完全独立 | 无 |

### Session 子功能

Session 是**会话级别的对话记忆管理器**，子功能如下：

```
Session
├── 消息存储
│   ├── FullContext:   完整历史（永不裁剪）
│   └── ContextWindow: 当前窗口（可能被 resize 裁剪）
├── 上下文窗口管理
│   ├── AutoResize:     自动触发裁剪开关
│   └── ResizeHandler:  三种内置策略
│       ├── SimpleCut:          从前面丢弃，保留最近的
│       ├── SummaryFirst:       保留首条 + 末尾2条 + 中间摘要
│       └── TokenAware:         按 token 估算裁剪 (len/4 = 1 token)
├── 多策略调度
│   ├── AnalysisHandler:  分析器（决定是否触发裁剪）
│   └── ResizeHandlers:   多策略注册表（name → handler）
├── 持久化
│   ├── ToJSON() / ToYAML()
│   └── LoadJSON() / LoadYAML()
└── Memo: 会话级长期记忆（跨轮次共享的结构化数据）
```

### Action 子功能

Action 是**独立的工具执行框架**，子功能如下：

```
Action
├── 注册: Registry.Register(action)
├── 执行: Registry.Execute(name, input)
├── 执行器: ActionExecutor
│   ├── LocalFunctionExecutor:  三种签名自动包装
│   │   ├── func(ctx, InputT) (OutputT, error)
│   │   ├── func(InputT) (OutputT, error)
│   │   └── func(ctx, InputT) OutputT
│   ├── MCPExecutor:    远程 MCP 协议（待实现）
│   └── SandboxExecutor: 沙箱执行器（待实现）
└── 规格: ActionSpec
    ├── SideEffectLevel:   "read" | "write" | "exec"
    ├── ApprovalRequired:  是否需要审批
    ├── SandboxRequired:   是否需要沙箱
    ├── ReplaySafe:        是否可重放
    └── ExposeToModel:     是否暴露给 LLM 调用
```

### Session 和 Action 的关系：不是强耦合

```
session 包 import → 无（只 import "fmt", "sort", "time"）
action 包 import → 无（只 import stdlib）

session 不调用 action，action 也不调用 session。
```

它们的关系通过 **上层 Agently 主模块** 桥接：

```
Session: 只记录"人说的话"（用户消息 + 助手回复）
Action:  结果通过 prompt 的 action_results 字段传给下一轮 LLM
```

**它们是两条平行的管道，由上层统一编排。**

### Flow 和 Schema 的关系：Schema 是可选的

```go
type Step struct {
    Name   string
    Func   StepFunc                // 必需的
    Schema *schema.OutputSchema    // 可选的指针类型
}

// 不设置 Schema → Step 正常执行，无校验
step := NewStep("myStep", myFunc).Build()      // ✅

// 设置 Schema → Step 执行后做额外校验
step := NewStep("myStep", myFunc).WithOutputSchema(schema).Build()  // ✅
```

**Flow 不依赖 Schema 也能跑。Schema 的作用是输出格式校验。**

---

## 二、工具调用完整调用链

### 1. 工具定义（tool 提示词）在哪一层提交给 Model？

```
ActionRegistry.List()                            ← 找到所有注册的 Action
    ↓
为每个 Action 生成 ToolDefinition：
    ToolDefinition{
        Name:        action.Name,
        Description: action.Description,
        Parameters:  action.Schema,               // JSON Schema 格式
    }
    ↓
ModelRequest.Tools = [...]                       ← 注入到请求中
    ↓
OpenAICompatibleProvider.GenerateRequestData()
    → HTTP 请求体: {"tools": [...], "messages": [...]}
```

**回答：在 model 层发出请求时提交。** 具体是 `ModelRequest.Tools` 字段。

### 2. 哪一层吃掉 LLM 返回的 FunctionCall？

```
LLM 返回 StreamChunk
    ↓
OpenAICompatibleProvider.RequestModel()           ← 解析 SSE 流
    ↓
chunk.ToolCalls = [{Name:"celsius_to_fahrenheit", Arguments:{"celsius":37}}]
    ↓
                                    ← inferglow 尚未实现！
Agently 主模块的 ActionRuntime:
    ├── AgentlyResponseParser                     ← 解析结构化输出
    ├── AgentlyActionRuntime                      ← 规划协议
    └── ActionNormalization.normalize_action_decision()  ← 提取 ActionCall
```

**回答：在 Agently 主模块的 ActionRuntime 层。** inferglow 目前只有 `ModelRequester` 能返回 `ToolCall`，但**没有一层来消费它、映射到 ActionRegistry、调度执行**。

### 3. Action 执行后结果怎么回传给 LLM？

```
ActionRegistry.Execute("celsius_to_fahrenheit", input)
    → ActionResult{OK: true, Result: 98.6}
    ↓
to_model_visible_records()                        ← 过滤敏感信息
    ↓
[{action_id, status, result}]
    ↓
重新构造 ModelRequest（追加 action_results）
    → ModelRequest.Actions = [...]
    → 发给 LLM 获取下一个决策
```

**回答：上层 Agenty 把 Action 执行结果追加到 `ModelRequest.Actions` 字段，作为下一轮 prompt 的一部分。**

### 4. Sandbox 穿透验证在哪一层触发？

```
ActionRegistry.Execute(action_id, input)
    ↓
ActionDispatcher.async_execute()                  ← 安全门控层
    │
    ├── 1. 从 ActionSpec 读取 sandbox_required = true ?
    │
    ├── 2. 检查 Executor.IsSandboxed() ?
    │       → 如果不 sandboxed → 返回 status: "blocked"
    │
    ├── 3. PolicyApproval.async_gate()            ← 审批检查
    │       → auto_approve / fail_closed / input_timeout_fail
    │
    ├── 4. ExecutionResourceManager.async_ensure()
    │       → 创建 Docker/gVisor/本地沙箱实例
    │
    └── 5. Executor.Execute()                     ← 实际执行
```

**回答：在 ActionDispatcher 层（执行调度层），在调用 `Executor.Execute()` 之前。**

---

## 三、完整调用链全景图

```
用户输入 "帮我转换 37°C 到华氏度"
    │
    ▼
┌─────────────────────────────────────────────────┐
│  Session (session 模块)                           │
│  AddMessage("user", "帮我转换...")                │
│  PreparePrompt() → 注入 chat_history             │
└──────────────────────┬──────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│  ModelRequest (model 模块)                       │
│  System: "你是一个助手"                           │
│  ChatHistory: session.PreparePrompt()            │
│  Tools: ActionRegistry → ToolDefinitions         │ ← 工具定义在这里提交
│  Output: {格式要求}                               │
└──────────────────────┬──────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│  ModelRequester.RequestModel()                   │
│  → HTTP POST /chat/completions                   │
│  → SSE 流: delta / tool_calls / done             │
└──────────────────────┬──────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│  ★ Agently 主模块 (尚未实现) ★                    │
│                                                  │
│  ★ 这是 inferglow 最缺的 "胶水" ★                 │
│                                                  │
│  1. 从 StreamChunk 中提取 ToolCall                │
│  2. 解析 LLM 的结构化输出 → ActionCall            │
│  3. ActionDispatcher 检查 PolicyApproval          │
│  4. ActionRegistry.Execute(action, input)         │
│  5. 检查 sandbox_required → 审批 → 创建沙箱       │
│  6. Executor.Execute() → ActionResult             │
│  7. 结果回写 prompt → 下一轮 LLM                  │
│  8. 循环直到 next_action == "response"            │
│     → 条件: next_action == "execute"              │
│       AND round_index < max_rounds                │
│       AND len(action_calls) > 0                   │
└──────────────────────┬──────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────┐
│  Session (session 模块)                           │
│  AddMessage("assistant", "37°C = 98.6°F")        │
│  → 对话历史闭环                                  │
└─────────────────────────────────────────────────┘
```

### 循环控制条件

```go
func shouldContinue(decision, roundIndex, maxRounds int) bool {
    if maxRounds > 0 && roundIndex >= maxRounds {
        return false  // 达到最大轮数
    }
    if decision.NextAction != "execute" {
        return false  // LLM 决定直接回复
    }
    if !decision.UseAction {
        return false  // 不需要使用 action
    }
    return len(decision.ActionCalls) > 0  // 有可执行的 action
}
```

---

## 四、inferglow 当前状态评估

### 已完成（零件）

| 模块 | 完成度 | 说明 |
|------|--------|------|
| **model** | ✅ 100% | Provider 抽象、流式传输、配置系统、重试 |
| **schema** | ✅ 100% | 泛型推导、JSON Schema 转换、ContractEngine、Blueprint |
| **flow** | ✅ 100% | 线性 Flow、TriggerFlow、13 种算子、持久化 |
| **action** | ✅ 80% | Registry、LocalFunctionExecutor 完整，MCP/Sandbox Executor 待实现 |
| **session** | ✅ 90% | 双列表、三种 resize 策略、持久化完整 |
| **sandbox** | ✅ 85% | Docker/gVisor/本地后端完整，Windows/macOS 适配中 |

### 缺失（组装线）

| 组件 | 说明 | 优先级 |
|------|------|--------|
| **ActionRuntime** | 规划协议 + 执行调度 | **P0 必须** |
| **Agent 类** | SessionExtension + ActionExtension + ModelRequester 桥接 | **P0 必须** |
| **PolicyApproval** | 安全门控（审批模式） | **P1 重要** |
| **ExecutionResource** | 沙箱环境创建与管理 | **P1 重要** |
| **MCPExecutor** | MCP 协议执行器 | **P2 可选** |
| **Structured Planning Handler** | 结构化输出解析 + ActionCall 提取 | **P0 必须** |

### 缺失的本质

inferglow 目前有 **model、schema、flow、action、session、sandbox 六个"零件"**，但缺少把它们串起来的**组装线**——这个组装线就是上层的 **Agently 主模块**（对应 Python Agently 的 `Agently/agently/` 目录）。

---

## 五、Qwen3 32B 模型能力评估

### 1. Qwen3 32B 的关键能力

| 能力 | Qwen3 32B | 评估 |
|------|-----------|------|
| 工具调用（Function Calling） | ✅ 支持，结构化输出稳定 | 够用 |
| JSON 结构化输出 | ✅ 输出格式控制较好 | 够用 |
| 长上下文（128K） | ✅ 原生支持 | 完全覆盖 |
| 推理能力 | ✅ 中等偏上 | 够用 |
| 代码生成 | ✅ 基本合格 | 够用 |
| 多轮对话一致性 | ✅ 良好 | 够用 |
| 中文理解 | ✅ 原生优势 | 优秀 |

### 2. 是否需要高级模型（GPT-4o / Claude Opus）介入？

**结论：不需要。** 理由如下：

#### 2.1 工具调用能力

Qwen3 32B 的 Function Calling 能力已经足够：
- 支持 `tools` 参数注入 ToolDefinition 列表
- 响应中的 `tool_calls` 字段格式稳定
- 参数提取准确率高（>85%）

**高级模型的增强**（GPT-4o）主要体现在：
- 更复杂的参数嵌套提取（Qwen3 32B 约 85% vs GPT-4o 约 95%）
- 更复杂的推理链（Qwen3 32B 约 75% vs GPT-4o 约 90%）

**但对于 inferglow 的 MVP 阶段，Qwen3 32B 完全够用。**

#### 2.2 结构化输出（Contract-First Schema）

这是 inferglow 的核心设计之一，需要 LLM 严格按 Schema 输出。

- Qwen3 32B + `output_schema` 约束 → 准确率约 80%
- GPT-4o + `output_schema` 约束 → 准确率约 95%

**建议方案：分阶段使用**
- **Phase 1 (MVP)**: 使用 Qwen3 32B，配合 ContractEngine 的重试机制自动修复格式错误
- **Phase 2 (生产)**: 如果关键业务需要更高准确率，可动态切换到高级模型

#### 2.3 成本控制

| 模型 | 输入价格 (每百万 token) | 输出价格 (每百万 token) | 适合场景 |
|------|----------------------|---------------------|---------|
| Qwen3 32B | ~¥2 (≈$0.30) | ~¥6 (≈$0.80) | **绝大多数场景** |
| GPT-4o | ~$10 | ~$30 | 复杂推理、高要求场景 |
| Claude Opus | ~$15 | ~$75 | 极端复杂任务 |

**Qwen3 32B 的成本是 GPT-4o 的 1/30 ~ 1/100。**

### 3. 最终建议

| 场景 | 推荐模型 | 理由 |
|------|---------|------|
| MVP 开发 / 测试 | Qwen3 32B | 成本低，能力足够 |
| 生产环境 - 常规任务 | Qwen3 32B | 成本效益最优 |
| 生产环境 - 复杂推理 | 动态切换 GPT-4o | 按需使用高级模型 |
| 生产环境 - 高安全要求 | 动态切换 Claude Opus | 复杂审批场景 |

**核心结论：Qwen3 32B 可以覆盖 inferglow 的全部核心能力需求。高级模型仅在特定场景作为"升级选项"出现，不是必须的。**

---

## 六、主模块实施蓝图

### 总体架构

```
Agently 主模块（上层业务逻辑）
│
├── Agent 类                          ← 用户入口
│   ├── SessionExtension              ← 对话记忆管理
│   ├── ActionExtension               ← 工具注册与执行
│   └── ModelRequestRunner            ← 模型调用
│
├── ActionRuntime                      ← 规划协议 + 执行调度
│   ├── StructuredPlanningHandler     ← 结构化规划
│   ├── NativeToolCallsHandler        ← 原生工具调用
│   └── ActionExecutionHandler        ← 执行动作
│
├── ActionDispatcher                   ← 安全门控 + 执行调度
│   ├── PolicyApproval                ← 审批检查
│   ├── ExecutionResource             ← 沙箱环境
│   └── Executor.Execute              ← 实际执行
│
└── ActionFlow (DAG)                   ← PLAN → EXECUTE 循环
    ├── plan_step:  决定下一步 action
    ├── execute_step: 执行 action_calls
    └── finalize:   返回最终结果
```

### Phase 1: 核心引擎（必须实现）

**目标**: 跑通最简单的 "用户输入 → LLM → 工具调用 → 结果" 闭环

| 任务 | 模块 | 工作量 | 依赖 |
|------|------|--------|------|
| 1.1 ActionRuntime 基础 | 新建 `agently/` 主模块 | 大 | action, model |
| 1.2 结构化规划协议 | actionruntime/planning.go | 中 | model |
| 1.3 ActionDispatcher 基础 | actionruntime/dispatcher.go | 中 | action |
| 1.4 Agent 基础类 | agent/agent.go | 大 | session, action, model |
| 1.5 SessionExtension | agent/session_ext.go | 小 | session |
| 1.6 ActionExtension | agent/action_ext.go | 中 | action |

**验收标准**:
- 注册 2-3 个 Action
- 用户输入 → LLM 选择工具 → Action 执行 → 结果返回 → 下一轮 LLM
- 至少支持 OpenAI 兼容 Provider

### Phase 2: 编排增强（重要）

**目标**: 支持复杂的多步工具调用和并发执行

| 任务 | 模块 | 工作量 | 依赖 |
|------|------|--------|------|
| 2.1 ActionFlow (PLAN → EXECUTE 循环) | actionruntime/flow.go | 大 | actionruntime |
| 2.2 并发执行控制 | actionruntime/concurrency.go | 中 | actionruntime |
| 2.3 TriggerFlow 集成 | agent/triggerflow.go | 中 | flow, action |
| 2.4 结构化输出验证 | schema/validator.go | 小 | schema, model |

**验收标准**:
- 支持 max_rounds 控制
- 支持并发 action 执行
- ContractEngine 验证 + 自动重试

### Phase 3: 安全框架（重要）

**目标**: 完整的沙箱审批和安全门控

| 任务 | 模块 | 工作量 | 依赖 |
|------|------|--------|------|
| 3.1 PolicyApproval 管理器 | security/policy_approval.go | 中 | action |
| 3.2 四种审批模式 | security/handlers.go | 小 | policy_approval |
| 3.3 ExecutionResource 管理 | security/exec_resource.go | 大 | sandbox |
| 3.4 SandboxExecutor 实现 | action/executor_sandbox.go | 中 | sandbox, action |

**验收标准**:
- sandbox_required = true 的 Action 必须走审批流程
- 支持 auto_approve / fail_closed / input_timeout_fail
- SandboxExecutor 能创建 Docker 容器执行命令

### Phase 4: 高级功能（可选）

| 任务 | 模块 | 工作量 |
|------|------|--------|
| 4.1 MCPExecutor | action/executor_mcp.go | 中 |
| 4.2 SessionMemory 插件 | session/memory_plugin.go | 中 |
| 4.3 Blueprint 序列化 | schema/blueprint.go | 小 |
| 4.4 Flow 持久化/恢复 | flow/persistence.go | 小 |

### 依赖关系图

```
phase 1:  ActionRuntime + Agent 类
              ↓
phase 2:  ActionFlow + 并发 + TriggerFlow 集成
              ↓
phase 3:  PolicyApproval + ExecutionResource
              ↓
phase 4:  MCPExecutor + SessionMemory + 高级功能
```

### 模块结构建议

```
agently/                              ← 主模块（新建）
├── go.mod
├── agent/
│   ├── agent.go                      ← Agent 类（用户入口）
│   ├── session_ext.go                ← SessionExtension
│   ├── action_ext.go                 ← ActionExtension
│   └── streaming.go                  ← 流式输出支持
├── actionruntime/
│   ├── planning.go                   ← 结构化规划协议
│   ├── dispatcher.go                 ← ActionDispatcher
│   ├── execution.go                  ← ActionExecutionHandler
│   └── flow.go                       ← PLAN → EXECUTE 循环
├── security/
│   ├── policy_approval.go            ← PolicyApprovalManager
│   ├── handlers.go                   ← 四种审批模式
│   └── exec_resource.go              ← ExecutionResourceManager
├── builtins/
│   ├── actions/                      ← 内置 Action（Browse, Search 等）
│   ├── executors/                    ← 内置 Executor（Local, Sandbox, MCP）
│   └── plugins/                      ← 内置插件
└── examples/
    └── basic_agent.go                ← 完整示例
```

### 与 inferglow 的关系

```
inferglow/                    ← 基础设施库（已完成）
├── model/                    ← LLM Provider 抽象
├── schema/                   ← Schema 引擎
├── flow/                     ← Flow 编排引擎
├── action/                   ← Action Runtime
├── session/                  ← Session 记忆管理
└── sandbox/                  ← 沙箱框架

agently/                      ← 上层应用层（待开发）
├── agent/                    ← Agent 类
├── actionruntime/            ← 规划 + 调度
├── security/                 ← 安全门控
└── builtins/                 ← 内置实现
```

**agently 主模块通过 replace 指令引用 inferglow 的子模块：**

```go
// agently/go.mod
replace github.com/inferglow/model => ../inferglow/model
replace github.com/inferglow/schema => ../inferglow/schema
replace github.com/inferglow/flow => ../inferglow/flow
replace github.com/inferglow/action => ../inferglow/action
replace github.com/inferglow/session => ../inferglow/session
replace github.com/inferglow/sandbox => ../inferglow/sandbox
```

---

## 七、总结

### inferglow 当前状态

- **已完成**: 6 个基础设施子模块（model/schema/flow/action/session/sandbox）
- **缺失**: Agently 主模块（组装线）
- **定位**: 纯基础设施库，等待上层 Agently 主模块引用

### 模型选择

- **Qwen3 32B 完全够用**，成本约为 GPT-4o 的 1/30
- 高级模型仅在特定场景作为"升级选项"
- 核心能力：ContractEngine 自动重试可以弥补 Qwen3 32B 在结构化输出上的不足

### 实施策略

1. **Phase 1 先跑通闭环**：最小可用 Agent（Session + Action + ModelRequest）
2. **Phase 2 增强编排**：多步工具调用、并发执行
3. **Phase 3 安全门控**：PolicyApproval、沙箱审批
4. **Phase 4 锦上添花**：MCP、SessionMemory、Blueprint
