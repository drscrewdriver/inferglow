# InferGlow 系统分析文档

> 本文档系列对 InferGlow 项目（`github.com/inferglow/*`）进行完整的系统分析，覆盖模块职责、核心类型、函数调用逻辑、模块间调用关系与关键调用链。
> 所有内容基于实际源码（截至 2026-07-22）梳理，不依赖外部推测。

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

## 阅读建议

- **快速了解全局**：先读 [01-architecture-overview.md](./01-architecture-overview.md)
- **理解 Agent 主循环**：读 [07-orchestrator.md](./07-orchestrator.md) + [08-call-chains.md](./08-call-chains.md)
- **深入某一模块**：直接跳转到对应编号文档

## 模块速查表

| 模块 | 路径 | 职责 | 依赖 |
|------|------|------|------|
| `model` | `github.com/inferglow/model` | LLM Provider 统一抽象 | 无（仅 stdlib + yaml） |
| `schema` | `github.com/inferglow/schema` | 契约优先 Schema 引擎 | `model` |
| `flow` | `github.com/inferglow/flow` | 步骤编排引擎 | `schema` |
| `action` | `github.com/inferglow/action` | Action Runtime（含 MCP） | `sandbox`（仅 SandboxExecutor） |
| `session` | `github.com/inferglow/session` | 对话记忆管理 | 无 |
| `sandbox` | `github.com/inferglow/sandbox` | 沙箱执行框架（7 种后端） | 无 |
| `audit` | `github.com/inferglow/audit` | 链表式审计链 | 无 |
| `security` | `github.com/inferglow/security` | PII / 注入 / 限流 / RBAC | 无 |
| `observability` | `github.com/inferglow/observability` | OpenTelemetry 集成 | 无 |
| `workspace` | `github.com/inferglow/workspace` | 工作区文件操作（血缘追踪为独立可选组件） | 无 |
| `orchestrator` | `github.com/inferglow/orchestrator` | Agent 编排层（上层胶水） | `action` `audit` `model` `security` `session` `sandbox` |
| `components` | `github.com/inferglow/components` | Prompt/Tool 通用接口 | 无 |
| `builtins` | `github.com/inferglow/builtins` | 内置 Action/Policy/Tool | 无 |

## 术语约定

- **Provider**：LLM 供应商适配器（OpenAI / Anthropic / Ollama）
- **Action**：可被 Agent 调用的工具单元
- **Executor**：Action 的执行后端（Local / Sandbox / MCP）
- **Decision**：LLM 每轮返回的规划决策（`execute` 或 `response`）
- **审计链**：基于 SHA-256 哈希指针的不可篡改日志
- **LoopGuard**：Agent 死循环检测器
