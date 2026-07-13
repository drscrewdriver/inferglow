# InferGlow 系统分析文档

> 本文档系列对 InferGlow 项目（`github.com/inferglow/*`）进行完整的系统分析，覆盖模块职责、核心类型、函数调用逻辑、模块间调用关系与关键调用链。
> 所有内容基于实际源码（截至 2026-07-30）梳理，不依赖外部推测。

## 文档索引

| 编号 | 文档 | 内容 |
|:----:|------|------|
| 01 | [架构总览与调用关系图](./01-architecture-overview.md) | 分层架构、模块依赖、全景调用关系图、数据流 |
| 02 | [model 与 schema 模块](./02-model-and-schema.md) | LLM Provider 抽象、流式传输、Schema 引擎、ContractEngine |
| 03 | [flow 模块](./03-flow.md) | Flow/TriggerFlow 双引擎、13 种算子、生命周期、Pause/Resume |
| 04 | [action 与 MCP 模块](./04-action-and-mcp.md) | Action Runtime、三种 Executor、MCP 协议层 |
| 05 | [session、sandbox 与 audit 模块](./05-session-sandbox-audit.md) | 对话记忆、沙箱框架、链表式审计链 |
| 06 | [security、observability 与 workspace 模块](./06-security-observability-workspace.md) | PII 脱敏、注入防护、限流、RBAC、OTel、工作区血缘 |
| 07 | [orchestrator 模块](./07-orchestrator.md) | Agent 入口、PLAN-EXECUTE 引擎、ActionDispatcher、LoopGuard |
| 08 | [关键调用链](./08-call-chains.md) | 端到端函数调用链追踪（含行号引用） |
| 09 | [编排层与中间层：历史成因与发展分析](./09-middleware-and-orchestration-history.md) | 两个聚类的演化史、设计动机、消费现状与遗留复杂性 |

## 阅读建议

- **快速了解全局**：先读 [01-architecture-overview.md](./01-architecture-overview.md)
- **理解 Agent 主循环**：读 [07-orchestrator.md](./07-orchestrator.md) + [08-call-chains.md](./08-call-chains.md)
- **深入某一模块**：直接跳转到对应编号文档

## 面向新开发者

如果你是第一次接触 inferglow，建议按以下路径学习：

### 1. 先跑起来

先执行 [`examples/example_quickstart.go`](../../examples/example_quickstart.go) 感受全貌：

```bash
cd examples
go run example_quickstart.go
```

这个示例不需要任何外部依赖或 API Key，使用内置 MockLLM 即可运行。

### 2. 理解核心概念

| 概念 | 对应文档 | 示例代码 | 一句话说明 |
|------|---------|---------|-----------|
| **Agent** | [07-orchestrator.md](./07-orchestrator.md) | `example_orchestrator.go` | 编排层的入口，负责调度 LLM 和 Action 的交互循环 |
| **Action** | [04-action-and-mcp.md](./04-action-and-mcp.md) | `example_action.go` | 将 Go 函数包装为 LLM 可调用的工具 |
| **Session** | [05-session-sandbox-audit.md](./05-session-sandbox-audit.md) | `example_session.go` | 对话记忆管理器，维护上下文窗口 |
| **Flow** | [03-flow.md](./03-flow.md) | `example_flow.go` | 步骤编排引擎，支持线性/条件/并行执行 |
| **Schema** | [02-model-and-schema.md](./02-model-and-schema.md) | `example_schema.go` | 契约优先的 LLM 输出格式校验 |
| **Model** | [02-model-and-schema.md](./02-model-and-schema.md) | `example_model.go` | LLM Provider 统一抽象（OpenAI/Anthropic/Ollama） |
| **Audit** | [05-session-sandbox-audit.md](./05-session-sandbox-audit.md) | `example_audit.go` | 基于 SHA-256 哈希链的不可篡改审计日志 |
| **LoopGuard** | [07-orchestrator.md](./07-orchestrator.md) | `example_orchestrator.go` | Agent 死循环检测器 |

### 3. 理解模块依赖关系

```
应用层 (server, cli)  →  编排层 (orchestrator)  →  中间层 (flow, action)  →  基础层 (model, session, ...)
```

- 基础层模块零内部依赖，可独立使用
- 中间层依赖基础层，提供上层能力
- 编排层聚合所有模块，形成完整 Agent
- 应用层面向用户，提供 REST API 和 CLI

## 模块速查表

22 个独立 Go module，按依赖深度分为三层。

### 基础层 — 13 个模块，零内部依赖

| 模块 | 路径 | 职责 | 依赖 |
|------|------|------|------|
| `model` | `github.com/inferglow/model` | LLM Provider 统一抽象 (~8000 LOC) | 无 |
| `schema` | `github.com/inferglow/schema` | 契约优先 Schema 引擎 (~2800 LOC) | 无 |
| `session` | `github.com/inferglow/session` | 对话记忆管理 (~1800 LOC) | 无 |
| `sandbox` | `github.com/inferglow/sandbox` | 沙箱执行框架 · 8 种后端 (~6300 LOC) | 无 |
| `context` | `github.com/inferglow/context` | 上下文管理引擎 · 三区压缩+缓存预算 (~6300 LOC) | 无 |
| `audit` | `github.com/inferglow/audit` | 链表式审计链 (~1100 LOC) | 无 |
| `approval` | `github.com/inferglow/approval` | HITL 审批 (~700 LOC) | 无 |
| `rag` | `github.com/inferglow/rag` | RAG 管道 · 6 种加载器 (~1500 LOC) | 无 |
| `rerank` | `github.com/inferglow/rerank` | 重排序 · Cohere/LLM/Fallback (~500 LOC) | 无 |
| `observability` | `github.com/inferglow/observability` | OpenTelemetry 集成 (~700 LOC) | 无 |
| `workspace` | `github.com/inferglow/workspace` | 工作区文件操作 (~1200 LOC) | 无 |
| `resource` | `github.com/inferglow/resource` | 资源管理 (~750 LOC) | 无 |
| `server` | `github.com/inferglow/server` | REST API 服务 (~700 LOC) | 无 |

### 中间层 — 5 个模块，依赖基础层

| 模块 | 路径 | 职责 | 依赖 |
|------|------|------|------|
| `components` | `github.com/inferglow/components` | Prompt/Tool 通用接口 (~400 LOC) | `model` |
| `flow` | `github.com/inferglow/flow` | DAG 编排引擎 (~6100 LOC) | `schema` |
| `action` | `github.com/inferglow/action` | Action Runtime (~2900 LOC) | `approval`, `sandbox` |
| `mcpserver` | `github.com/inferglow/mcpserver` | MCP 协议服务 · 三传输 (~850 LOC) | `action` |
| `builtins` | `github.com/inferglow/builtins` | 内置 Action/Policy/Tool (~2200 LOC) | `action` |

### 编排层 — 4 个模块，聚合中间层+基础层

| 模块 | 路径 | 职责 | 依赖 |
|------|------|------|------|
| `orchestrator` | `github.com/inferglow/orchestrator` | Agent 编排层 · 用户入口 (~7700 LOC) | `action` `audit` `flow` `model` `observability` `session` |
| `security` | `github.com/inferglow/security` | PII / 注入 / 限流 / RBAC (~2000 LOC) | `session` `orchestrator`（接口注入） |
| `eval` | `github.com/inferglow/eval` | 离线评估框架 (~750 LOC) | `model` `session` `action` `orchestrator` |
| `examples` | `github.com/inferglow/examples` | 示例代码 (~2800 LOC) | 多模块 |

## 术语约定

- **Provider**：LLM 供应商适配器（OpenAI / Anthropic / Ollama）
- **Action**：可被 Agent 调用的工具单元
- **Executor**：Action 的执行后端（Local / Sandbox / MCP）
- **Decision**：LLM 每轮返回的规划决策（`execute` 或 `response`）
- **审计链**：基于 SHA-256 哈希指针的不可篡改日志
- **LoopGuard**：Agent 死循环检测器
