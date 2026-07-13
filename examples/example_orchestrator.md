# example_orchestrator - 编排器 / Orchestrator

## 概述 / Overview

本示例演示如何使用 `orchestrator` 模块组装 Agent 并执行 PLAN -> EXECUTE 循环。通过一个 Mock LLM 模拟决策流程，展示从 Session 创建、Action 注册、LoopGuard 配置到 AuditChain 审计的全链路集成，无需真实 LLM API Key 即可验证编排逻辑。

This example demonstrates how to use the `orchestrator` module to assemble an Agent and execute the PLAN -> EXECUTE loop. Using a Mock LLM to simulate the decision flow, it showcases the full pipeline integration from Session creation, Action registration, LoopGuard configuration, to AuditChain auditing, all verifiable without a real LLM API Key.

## 核心概念 / Core Concepts

- **Agent / 代理**: 整合 Session、ActionExtension 和 LLM 的高层抽象，通过 `agent.New` 快速组装
- **PLAN -> EXECUTE 循环 / PLAN -> EXECUTE Loop**: Agent 的核心执行模式，LLM 输出 Decision JSON 驱动下一步动作
- **Session / 会话**: 管理上下文窗口，记录对话历史
- **ActionExtension / 动作扩展**: 注册和管理可被 LLM 调用的 Action（工具调用）
- **LoopGuard / 循环守卫**: 检测 Agent 死亡循环，支持重复动作检测、输出停滞检测、时间和 Token 预算限制
- **AuditChain / 审计链**: 记录 Agent 执行的每一步，支持事后验证和查询
- **Engine / 引擎**: 底层执行引擎，支持 `NewEngineWithAuditAndLoopGuard` 高级构造

## 前置条件 / Prerequisites

- Go 1.21+
- 无需真实 LLM API Key，示例使用 MockLLM 返回固定 Decision JSON
- No real LLM API Key required; the example uses MockLLM returning a fixed Decision JSON

## 使用示例 / Usage Example

代码演示了以下 5 个场景：

1. **组装 Agent 组件**: 创建 Session、ActionExtension、注册 `greet` Action，以及创建 MockLLM。
2. **LoopGuard 配置**: 使用 `agent.NewLoopGuard` 配置重复动作检测窗口（3 轮）、时间预算（2 分钟）和 Token 预算（50000），并展示初始状态检查通过（VerdictContinue）。
3. **AuditChain 准备**: 创建启用了 HMAC-SHA256 签名的审计链，说明其作为 `AuditHook` 接口的实现可直接注入 Engine。
4. **Agent Run（端到端演示）**: 使用 `agent.New` 组装完整 Agent，通过 `ag.Run` 执行 PLAN -> EXECUTE 循环，MockLLM 在第一轮返回 response 决策终止循环。
5. **AuditChain 验证**: 手动追加审计条目，执行 `VerifyChain` 全链验证，并按 Source 字段查询过滤。

此外，代码末尾附带了手动组装带 Audit + LoopGuard 的 Engine 的高级用法说明，使用 `agent.NewEngineWithAuditAndLoopGuard` 构造器。

The code demonstrates 5 scenarios:

1. **Assembling Agent Components**: Create Session, ActionExtension, register a `greet` Action, and create MockLLM.
2. **LoopGuard Configuration**: Configure `NewLoopGuard` with a repeat action window (3 rounds), time budget (2 minutes), and token budget (50000), showing the initial state check passes (VerdictContinue).
3. **AuditChain Preparation**: Create an audit chain with HMAC-SHA256 signature enabled, explaining its role as an `AuditHook` interface implementation injectable into the Engine.
4. **Agent Run (End-to-End)**: Assemble a complete Agent with `agent.New`, execute the PLAN -> EXECUTE loop via `ag.Run`, where MockLLM returns a response decision in the first round to terminate the loop.
5. **AuditChain Verification**: Manually append an audit entry, run `VerifyChain` for full-chain verification, and query by Source field.

Additionally, the code includes advanced usage notes on manually assembling an Engine with Audit + LoopGuard using the `agent.NewEngineWithAuditAndLoopGuard` constructor.

## 运行验证 / Running the Example

```
cd examples
go run example_orchestrator.go
```

预期输出会依次展示：

- Session 创建成功，显示 ID 和 MaxLength
- greet Action 注册成功，列出已注册的 Action 列表
- LoopGuard 配置参数和初始状态检查结果
- AuditChain 创建成功，显示 IsEnabled 和 Len
- Agent.Run 执行完成，返回 MockLLM 的响应结果
- AuditChain 追加和验证成功，查询按 Source=agent 过滤的条目

Expected output shows:

- Session created successfully, showing ID and MaxLength
- greet Action registered, listing registered actions
- LoopGuard configuration parameters and initial state check result
- AuditChain created successfully, showing IsEnabled and Len
- Agent.Run completed, returning the MockLLM response
- AuditChain append and verification succeeded, querying entries filtered by Source=agent

## 预期输出 / Expected Output

输出着重展示 orchestrator 模块的编排能力：Agent 的 PLAN -> EXECUTE 循环如何工作，LoopGuard 如何防止死循环，AuditChain 如何记录和验证每一步执行。开发者可以通过此示例快速理解 Agent 的完整生命周期。

The output highlights the orchestration capabilities of the orchestrator module: how the Agent's PLAN -> EXECUTE loop works, how LoopGuard prevents infinite loops, and how AuditChain records and verifies each step of execution. Developers can quickly understand the complete Agent lifecycle through this example.