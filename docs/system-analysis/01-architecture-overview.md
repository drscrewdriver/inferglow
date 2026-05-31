# 01 · 架构总览与调用关系图

## 一、分层架构

InferGlow 采用**基础设施层 + 编排层**的两段式架构。基础设施层由 8 个独立子模块组成（model / schema / flow / action / session / sandbox / resource / approval），编排层（orchestrator）把它们粘合成可运行的 Agent。安全（security）、审计（audit）、可观测（observability）作为横切关注点贯穿全栈。

```
┌─────────────────────────────────────────────────────────────────────┐
│                        用户代码 / Examples                           │
└──────────────────────────────────┬──────────────────────────────────┘
                                   │  Agent.Run(ctx, userMessage)
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     编排层 (orchestrator)                            │
│  ┌───────────────┐   ┌───────────────────┐   ┌──────────────────┐   │
│  │  agent.Agent  │──▶│  agent.Engine     │──▶│ actionruntime.   │   │
│  │  (用户入口)    │   │  (PLAN-EXECUTE   │   │ ActionDispatcher │   │
│  │  +Strategy    │   │   循环引擎)       │   │ (并发执行+审计)   │   │
│  │  +Extensions  │   │                   │   │ +DAGActionFlow   │   │
│  └──────┬────────┘   └─────────┬─────────┘   └────────┬─────────┘   │
│         │                      │                      │             │
│         │     ┌────────────────┴────────┐             │             │
│         │     │  agent.LoopGuard        │             │             │
│         │     │  (死循环检测)            │             │             │
│         │     │  agent.TurnLoop         │             │             │
│         │     │  (PLAN/ACTIVE/IDLE)     │             │             │
│         │     │  agent.CancelManager    │             │             │
│         │     └─────────────────────────┘             │             │
│         │                                            │             │
│         │     ┌──────────────────────────────────────┘             │
│         │     │                                                      │
│         ▼     ▼                                                      │
│  ┌───────────────────┐   ┌──────────────────────────────────────┐   │
│  │ SessionExtension  │   │ actionruntime.ParseDecision          │   │
│  │ ActionExtension   │   │ actionruntime.ShouldContinue         │   │
│  │ (桥接 session/    │   │ (LLM 输出→Decision 解析+循环控制)     │   │
│  │  action 到 engine)│   └──────────────────────────────────────┘   │
│  └───────────────────┘                                              │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  orchestrator 子包                                            │   │
│  │  recordstore/  — 统一执行记录存储（Record/Checkpoint/Event）   │   │
│  │  taskcontext/  — 任务上下文聚合（ContextSource + Budget）      │   │
│  │  taskdag/      — 模型生成的 DAG 执行（TopoSort + Executor）    │   │
│  │  skill/        — 技能库管理（安装/版本/绑定）                   │   │
│  │  blocks/       — 结构化可组合执行块（Reason/Act/Intent）       │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        ▼                          ▼                          ▼
┌─────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   model         │      │   action         │      │   session        │
│   LLM Provider  │      │   Action Runtime │      │   对话记忆        │
│   抽象层         │      │   + MCP 协议层    │      │   双列表+resize  │
└────────┬────────┘      └────────┬─────────┘      └──────────────────┘
         │                        │
         ▼                        ▼
┌─────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   schema        │      │   sandbox        │      │   audit          │
│   契约校验引擎   │      │   沙箱执行框架    │      │   链表式审计链    │
│   泛型推导        │      │   7 种后端        │      │   SHA-256 哈希链  │
└─────────────────┘      └──────────────────┘      └──────────────────┘

┌─────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   security      │      │  observability   │      │   workspace      │
│  PII/注入/限流/  │      │  OpenTelemetry   │      │   工作区文件操作   │
│  RBAC           │      │  Span 集成        │      │                  │
└─────────────────┘      └──────────────────┘      └──────────────────┘

┌─────────────────┐      ┌──────────────────┐
│  components     │      │   builtins       │
│  Prompt/Tool    │      │  内置 Action/    │
│  通用接口        │      │  Policy/Tool     │
└─────────────────┘      └──────────────────┘

┌─────────────────┐      ┌──────────────────┐
│   resource      │      │   approval       │
│  执行资源生命    │      │  策略审批框架     │
│  周期管理        │      │  可插拔 Handler  │
└─────────────────┘      └──────────────────┘
```

## 二、模块依赖关系图

依赖关系来源于各模块的 `go.mod`（`replace` 指令）与源码 `import` 语句。箭头方向为「依赖方 → 被依赖方」。

```
                    ┌──────────────┐
                    │ orchestrator │  (编排层)
                    │              │
                    │  agent/    ──┼──▶ flow (Step-based: Flow, Step, Execution)
                    │  taskdag/  ──┼──▶ flow (Step-based: FlowBuilder → Flow)
                    │  blocks/   ──┼──▶ flow (Signal-driven: Operator, OpResultSink)
                    │              │
                    └──────┬───────┘
        ┌──────────┬───────┼────────┬──────────┬────────────┐
        ▼          ▼       ▼        ▼          ▼            ▼
   ┌────────┐ ┌────────┐ ┌──────┐ ┌──────┐ ┌────────┐ ┌─────────┐
   │ model  │ │ action │ │session│ │audit │ │security│ │ sandbox │
   └────┬───┘ └───┬────┘ └──────┘ └──────┘ └────────┘ └─────────┘
        │         │
        │         │ action.executor_sandbox.go
        │         └─────────────────────────────▶ sandbox
        ▼
   ┌────────┐
   │ schema │
   └────┬───┘
        │
        ▼
   ┌──────────────────────────────────────────────────┐
   │  flow  (19K行, 双范式共存于同一 Go package)       │
   │                                                  │
   │  范式 A: Step-based (~900行)                     │
   │    Flow → Step → Engine.Execute()                │
   │    消费者: orchestrator/agent/, orchestrator/taskdag/ │
   │                                                  │
   │  范式 B: Signal-driven (~2.5K行)                   │
   │    TriggerFlow → Operator → SignalNet            │
   │    消费者: orchestrator/blocks/                    │
   │                                                  │
   │  范式 C: 共享基础设施 (~2.6K行)                    │
   │    persistence / subflow / inputsource / lifecycle│
   └──────────────────────────────────────────────────┘
```

> **注**：`flow` 模块内部包含两套编排范式（详见 [03-flow.md 第六节](./03-flow.md)）。理想情况下范式 B 可拆分为独立的 `triggerflow/` 模块，但当前处于活跃开发期，暂不拆分。

**关键依赖事实（来自 `go.mod`）：**

| 模块 | 直接依赖（inferglow 内部） |
|------|---------------------------|
| `orchestrator` | `action` `audit` `flow` `model` `observability` `session`（子包 recordstore/taskcontext/taskdag/skill/blocks 在模块内部） |
| `orchestrator/agent` | `flow` 的 Step-based API（Flow, Step, Execution, FlowContext） |
| `orchestrator/taskdag` | `flow` 的 Step-based API（FlowBuilder → Flow） |
| `orchestrator/blocks` | `flow` 的 Signal-driven API（Operator, OpResultSink, OpMatchRoute） |
| `schema` | `model` |
| `flow` | `schema` |
| `action` | `sandbox`（仅 `executor_sandbox.go` 一个文件） |
| `model` | 无 |
| `session` | 无 |
| `sandbox` | 无 |
| `audit` | 无 |
| `security` | 无 |
| `observability` | 无 |
| `workspace` | 无 |
| `resource` | 无（独立 Go module，无 inferglow 内部依赖） |
| `approval` | 无（独立 Go module，无 inferglow 内部依赖） |
| `components` | 无 |
| `builtins` | 无 |

> **注**：`resource` 和 `approval` 是独立 Go module（各有自己的 `go.mod`），不依赖任何 inferglow 内部模块。它们通过 `orchestrator/agent` 的 `AgentExtensions` 可选注入，零注入 = 零变化 = 向后兼容。

> 注意：`security/pii` 的 `Masker` 类型在运行时被 `orchestrator/agent` 引用（通过 `session.MessageMasker` 接口），但这是接口依赖而非编译期包依赖——`session` 只定义接口，`pii.Masker` 实现它。

## 三、Agent 主循环数据流

下图展示一次完整的 `Agent.Run()` 调用如何在模块间流转数据。

```
用户消息 "帮我查北京天气"
    │
    ▼
┌──────────────────────────────────────────────────────────────┐
│ Agent.Run()                                  [orchestrator]   │
│  1. session.AddUserMessage(userMessage)      ──▶ session     │
│  2. engine.executeLoop(...)                  ──▶ agent.Engine│
└──────────────────────────┬───────────────────────────────────┘
                           │
        ┌──────────────────┐ ▼ ┌───────────────────┐
        │  LoopGuard.Check │   │  循环 round=0..N   │
        │  (死循环检测)     │   │                   │
        └──────────────────┘   └─────────┬─────────┘
                                          │
                          ┌───────────────┼───────────────┐
                          ▼               ▼               ▼
                  ┌──────────────┐ ┌─────────────┐ ┌──────────────┐
                  │ buildToolDefs│ │ session.    │ │ modelReq.    │
                  │ (action→tool)│ │ PreparePrompt│ │ GenerateReq  │
                  │              │ │ (历史→prompt)│ │ Data         │
                  └──────┬───────┘ └──────┬──────┘ └──────┬───────┘
                         │                │               │
                         └────────────────┼───────────────┘
                                          ▼
                          ┌───────────────────────────────┐
                          │ modelReq.RequestModel(ctx)    │  [model]
                          │  → HTTP POST /chat/completions│
                          │  → SSE 流式返回 StreamChunk    │
                          └───────────────┬───────────────┘
                                          │ <-chan *StreamChunk
                                          ▼
                          ┌───────────────────────────────┐
                          │ 流式收集 content              │
                          │ (含超时/抢占/cancel 检查)      │
                          └───────────────┬───────────────┘
                                          │ string
                                          ▼
                          ┌───────────────────────────────┐
                          │ actionruntime.ParseDecision   │  [orchestrator]
                          │  RepairLLMJSON → json.Unmarshal│
                          │  → Decision{NextAction,...}   │
                          └───────────────┬───────────────┘
                                          │
                          ┌───────────────┴───────────────┐
                          ▼                               ▼
                 NextAction=="response"          NextAction=="execute"
                          │                               │
                          ▼                               ▼
              ┌─────────────────────┐        ┌─────────────────────┐
              │ outputHook.Check    │        │ ActionDispatcher.   │
              │ (注入检测)          │        │ Execute(ctx, calls) │
              │ piiMasker.MaskOutput│        │  并发 goroutine:    │
              │ session.AddAssistant│        │   registry.Execute  │
              │  Message            │        │   → ActionExecutor  │
              │ return response     │        │   → audit.Append    │
              └─────────────────────┘        └──────────┬──────────┘
                                                        │
                                                        ▼
                                              ┌─────────────────────┐
                                              │ session.AddAction   │
                                              │  Result(name,result)│
                                              │ round++ → 下一轮    │
                                              └─────────────────────┘
```

## 四、横切关注点调用关系

### 4.1 审计链（AuditChain）的接入点

审计链通过 `audit.AuditHook` 接口接入，**默认关闭**（`NoOpHook` 零开销）。开启后有两个写入点：

```
agent.Engine.executeLoop()
    │
    ├── auditHook.Append(AuditEntry{Source:"agent", Action:"decision"})
    │   (每轮 LLM 决策后写入)
    │
    └── ActionDispatcher.Execute()
        └── auditHook.Append(AuditEntry{Source:"action", Action:"execute"})
            (每个 Action 执行后写入，含 panic 恢复)
```

### 4.2 安全钩子接入点

```
Agent.Run()
    │
    ├── piiMasker.MaskInput  ──▶ session.AddMessageChecked (输入侧脱敏)
    │
    ├── outputHook.CheckOutput ──▶ 最终响应注入检测
    │
    ├── piiMasker.MaskOutput ──▶ 最终响应脱敏
    │
    └── session.securityHook.BeforeAddMessage ──▶ 消息拦截（注入阻断）

ActionDispatcher.Execute()
    └── (SandboxExecutor 内) ApprovalService.Submit ──▶ 沙箱审批门控
```

### 4.3 LoopGuard 接入点

```
engine.executeLoop() 每轮开头：
    │
    ├── LoopGuard.Check(state)
    │       │
    │       ├── RepeatAction 检测 (滑动窗口 N=3)
    │       ├── OutputStagnation 检测 (Jaccard 相似度 > 0.9)
    │       ├── TimeBudget 检测 (默认 5min)
    │       └── TokenBudget 检测 (默认 100000)
    │
    ├── VerdictBreak   → return ErrLoopDetected
    ├── VerdictDegrade → systemPrompt += degrade 提示
    └── VerdictContinue → 继续
```

## 五、Go Module 拓扑

InferGlow 采用**多 module 单仓库**（polyrepo 风格），每个子目录是独立的 Go module，通过 `replace` 指令指向本地路径。

```
inferglow/
├── go.mod                      module github.com/inferglow/model   (根 module 即 model)
├── schema/go.mod               module github.com/inferglow/schema
├── flow/go.mod                 module github.com/inferglow/flow
├── action/go.mod               module github.com/inferglow/action
├── session/go.mod              module github.com/inferglow/session
├── sandbox/go.mod              module github.com/inferglow/sandbox
├── audit/go.mod                module github.com/inferglow/audit
├── security/go.mod             module github.com/inferglow/security
├── observability/go.mod        module github.com/inferglow/observability
├── workspace/go.mod            module github.com/inferglow/workspace
├── resource/go.mod             module github.com/inferglow/resource    (新增: 执行资源管理)
├── approval/go.mod             module github.com/inferglow/approval    (新增: 策略审批框架)
├── orchestrator/go.mod         module github.com/inferglow/orchestrator
├── components/go.mod           module github.com/inferglow/components
├── builtins/go.mod             module github.com/inferglow/builtins
└── examples/go.mod             module github.com/inferglow/examples
```

> 根 `go.mod` 的 module 名为 `github.com/inferglow/model`（历史遗留），子模块各自独立。`orchestrator/go.mod` 通过 6 条 `replace` 指令把基础模块指向本地 `../` 路径。`resource/` 和 `approval/` 是独立 Go module，无 inferglow 内部依赖。

## 六、与旧版 ARCHITECTURE.md 的差异

旧版 `ARCHITECTURE.md` 描述的状态已过时，主要差异：

| 项目 | 旧文档描述 | 实际现状 |
|------|-----------|---------|
| 编排层 | "Agently 主模块（尚未实现）" | ✅ `orchestrator` 模块已完整实现 |
| MCPExecutor | "待实现" | ✅ `action/executor_mcp.go` + `action/mcp/` 已实现 |
| SandboxExecutor | "待实现" | ✅ `action/executor_sandbox.go` 已实现 |
| AuditChain | 未提及 | ✅ `audit/` 模块已实现（P0 新增） |
| LoopGuard | 未提及 | ✅ `orchestrator/agent/loop_guard.go` 已实现（P0 新增） |
| PII 脱敏 | 未提及 | ✅ `security/pii/` 已实现 |
| 注入防护 | 未提及 | ✅ `security/prompt_injection/` 已实现 |
| 限流 | 未提及 | ✅ `security/ratelimit/` 已实现 |
| RBAC | 未提及 | ✅ `security/rbac/` 已实现 |
| OTel 可观测 | 未提及 | ✅ `observability/otel/` 已实现 |
| ExecutionResource | 未提及 | ✅ `resource/` 独立模块已实现（Agently 等价） |
| PolicyApproval | 未提及 | ✅ `approval/` 独立模块已实现（Agently 等价） |
| RecordStore | 未提及 | ✅ `orchestrator/recordstore/` 已实现（Agently 等价） |
| TaskContext | 未提及 | ✅ `orchestrator/taskcontext/` 已实现（Agently 等价） |
| TaskDAG | 未提及 | ✅ `orchestrator/taskdag/` 已实现（Agently 等价） |
| SkillLibrary | 未提及 | ✅ `orchestrator/skill/` 已实现（Agently 等价） |
| Blocks 框架 | 未提及 | ✅ `orchestrator/blocks/` 已实现（Agently 等价） |
| DAGActionFlow | 未提及 | ✅ `orchestrator/actionruntime/dag_flow.go` 已实现 |
| ExecutionStrategy | 未提及 | ✅ `orchestrator/agent/strategy.go` 已实现 |
| Workspace 增强 | 未提及 | ✅ `workspace/execution_access.go` + `identity.go` + `context_source.go` 已实现 |

本文档系列反映的是**实际代码状态**，不再使用「主模块待实现」的表述。
