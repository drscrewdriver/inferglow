# Inferglow 系统分析文档

> 基于源码分析 + Graphify 知识图谱（8017 节点，17577 边，414 社区）的全面系统分析
> 生成日期：2026-07-31

---

## 文档目录

| 编号 | 文档 | 核心内容 |
|:----:|------|---------|
| 01 | [项目概述与设计哲学](./01-overview-and-philosophy.md) | 项目定位、设计哲学、Go 适配策略、架构图 |
| 02 | [模块架构总览](./02-module-architecture-overview.md) | 四层 23 模块矩阵、依赖关系、模块清单 |
| 03 | [基础层详解](./03-foundation-layer.md) | 12 个零依赖模块：model/schema/session/sandbox/context/audit/approval/rag/rerank/observability/workspace/resource |
| 04 | [中间层详解](./04-middle-layer.md) | 5 个模块：flow/action/components/mcpserver/builtins |
| 05 | [编排层详解](./05-orchestration-layer.md) | 3 个模块：orchestrator/security/eval |
| 06 | [应用层详解](./06-application-layer.md) | 3 个模块：server/cli/examples |
| 07 | [横切关注点](./07-cross-cutting-concerns.md) | FlowContext 接口、小接口拆分、暂停信号、Otel 可观测 |
| 08 | [Graphify 知识图谱分析](./08-graphify-analysis.md) | 8017 节点图谱、God Nodes 排名、社区结构、跨模块桥接 |
| 09 | [调用链全景分析](./09-call-chains.md) | 端到端 Agent 循环序列图、13 条调用链、错误传播 |
| 10 | [设计模式与架构决策](./10-design-patterns.md) | 12 种设计模式、8 项关键决策、Go 语言适配对照 |
| 11 | [可插拔架构](./11-pluggable-architecture.md) | Build Tag 机制、接口注入模式、编译配置决策树 |
| 12 | [质量属性与演进路线](./12-quality-and-roadmap.md) | 质量属性度量、演进路线、待增强方向、附录 |

## 阅读建议

| 目标 | 推荐阅读路径 |
|------|-------------|
| 快速了解全局 | [01](./01-overview-and-philosophy.md) → [02](./02-module-architecture-overview.md) |
| 理解 Agent 主循环 | [05](./05-orchestration-layer.md) → [09](./09-call-chains.md) |
| 深入 Flow 引擎 | [04](./04-middle-layer.md) → [07](./07-cross-cutting-concerns.md) |
| 架构健康度评估 | [08](./08-graphify-analysis.md) → [12](./12-quality-and-roadmap.md) |
| 扩展与定制 | [10](./10-design-patterns.md) → [11](./11-pluggable-architecture.md) |
| 完整系统理解 | 按 01→12 顺序阅读 |

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

### 基础层 — 12 个模块，零内部依赖

| 模块 | 路径 | 代码量 | 核心职责 |
|------|------|--------|---------|
| `model` | `github.com/inferglow/model` | ~8000 LOC | LLM Provider 统一抽象 |
| `schema` | `github.com/inferglow/schema` | ~2800 LOC | 契约优先 Schema 引擎 |
| `session` | `github.com/inferglow/session` | ~1800 LOC | 对话记忆管理 |
| `sandbox` | `github.com/inferglow/sandbox` | ~6300 LOC | 8 种沙箱后端 |
| `context` | `github.com/inferglow/context` | ~6300 LOC | 三区压缩上下文管理 |
| `audit` | `github.com/inferglow/audit` | ~1100 LOC | 链表式审计链 |
| `approval` | `github.com/inferglow/approval` | ~700 LOC | HITL 审批 |
| `rag` | `github.com/inferglow/rag` | ~1500 LOC | RAG 管道 |
| `rerank` | `github.com/inferglow/rerank` | ~500 LOC | 重排序 |
| `observability` | `github.com/inferglow/observability` | ~700 LOC | OpenTelemetry |
| `workspace` | `github.com/inferglow/workspace` | ~1200 LOC | 工作区文件操作 |
| `resource` | `github.com/inferglow/resource` | ~750 LOC | 资源管理 |

### 中间层 — 5 个模块，依赖基础层

| 模块 | 路径 | 代码量 | 依赖 | 核心职责 |
|------|------|--------|------|---------|
| `flow` | `github.com/inferglow/flow` | ~7400 LOC | `schema` | DAG 编排引擎 |
| `action` | `github.com/inferglow/action` | ~2900 LOC | `approval`, `sandbox` | Action Runtime |
| `components` | `github.com/inferglow/components` | ~400 LOC | `model` | Prompt/Tool 接口 |
| `mcpserver` | `github.com/inferglow/mcpserver` | ~850 LOC | `action` | MCP 协议服务 |
| `builtins` | `github.com/inferglow/builtins` | ~2200 LOC | `action` | 内置 Action |

### 编排层 — 3 个模块，聚合中间层+基础层

| 模块 | 路径 | 代码量 | 核心依赖 | 核心职责 |
|------|------|--------|---------|---------|
| `orchestrator` | `github.com/inferglow/orchestrator` | ~7700 LOC | action/audit/flow/model/observability/session | Agent 编排引擎 |
| `security` | `github.com/inferglow/security` | ~2000 LOC | session/orchestrator（接口注入） | PII/注入/限流/RBAC |
| `eval` | `github.com/inferglow/eval` | ~750 LOC | model/session/action/orchestrator | 离线评估框架 |

### 应用层 — 3 个模块

| 模块 | 路径 | 代码量 | 依赖 | 核心职责 |
|------|------|--------|------|---------|
| `server` | `github.com/inferglow/server` | ~3100 LOC | flow/orchestrator | REST API 服务 |
| `cli` | `github.com/inferglow/cli` | ~1200 LOC | orchestrator/action/builtins/context | 终端 REPL |
| `examples` | `github.com/inferglow/examples` | ~2800 LOC | 多模块 | 示例代码 |

## 术语约定

| 术语 | 说明 |
|------|------|
| Provider | LLM 供应商适配器（OpenAI / Anthropic / Ollama） |
| Action | 可被 Agent 调用的工具单元 |
| Executor | Action 的执行后端（Local / Sandbox / MCP） |
| Decision | LLM 每轮返回的规划决策（`execute` 或 `response`） |
| 审计链 | 基于 SHA-256 哈希指针的不可篡改日志 |
| LoopGuard | Agent 死循环检测器 |
| FlowContext | 横切关注点注入接口（flow 包定义，orchestrator 实现） |
| Checkpoint | Flow 执行快照（Pause/Resume 机制） |
| God Node | Graphify 识别的架构枢纽节点（最高连接度） |
| Community | Graphify 聚类社区（模块级聚合） |

## 相关文档索引

| 文档 | 位置 | 内容 |
|------|------|------|
| 架构深度分析 | [ARCHITECTURE.md](../../ARCHITECTURE.md) | 12 章完整架构分析 |
| 扩展机制 | [EXTENDING.md](../../docs/EXTENDING.md) | 7 种扩展机制 |
| 上游缺口 | [upstream-gaps.md](../../docs/upstream-gaps.md) | inferglow 待完善能力清单 |
| README | [README.md](../../README.md) | 项目快速入门 |