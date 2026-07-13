# example_quickstart - 快速入门示例 / Quickstart Example

## 概述 / Overview

本示例演示了 inferglow 框架的核心概念，构建一个最小完整的 Agent 运行流程。它使用 MockLLM 模拟 LLM 响应，无需真实 API Key 即可验证 Agent 的 PLAN-EXECUTE 循环（规划-执行循环）是否正常工作。

This example demonstrates the core concepts of the inferglow framework by building a minimal complete Agent execution flow. It uses a MockLLM to simulate LLM responses, allowing verification of the Agent's PLAN-EXECUTE loop without requiring a real API Key.

## 核心概念 / Core Concepts

- **Session（对话记忆）**：管理 Agent 的对话历史，支持上下文窗口大小控制
- **Action（工具）**：将 Go 函数包装为 LLM 可调用的工具，自动生成 JSON Schema
- **MockLLM（模拟 LLM）**：无需真实 API Key 即可模拟 LLM 响应，适用于测试和演示
- **Agent（智能体）**：组装 Session、Action 和 LLM，执行完整的 PLAN-EXECUTE 循环
- **ActionExtension（工具注册表）**：管理可被 LLM 调用的 Action 集合

- **Session**：Manages the Agent's conversation history with configurable context window size
- **Action**：Wraps Go functions as LLM-callable tools with automatic JSON Schema generation
- **MockLLM**：Simulates LLM responses without a real API Key, suitable for testing and demonstration
- **Agent**：Assembles Session, Actions, and LLM to execute the full PLAN-EXECUTE loop
- **ActionExtension**：Manages the collection of Actions available for LLM invocation

## 前置条件 / Prerequisites

- Go 1.21+
- inferglow 项目依赖已安装（`go mod tidy`）
- 无需 LLM API Key（本示例使用 MockLLM）

- Go 1.21+
- inferglow project dependencies installed (`go mod tidy`)
- No LLM API Key required (this example uses MockLLM)

## 使用示例 / Usage Example

代码流程如下 / The code flow is as follows:

1. **创建 Session**：调用 `session.NewSession("quickstart-demo", 4000)` 创建一个最大上下文长度为 4000 的对话会话
2. **创建 ActionExtension**：通过 `agent.NewActionExtension()` 创建工具注册表
3. **注册 Action**：使用 `action.New()` 包装两个 Go 函数——`greet`（问候）和 `add`（加法），自动生成 JSON Schema 并注册到 ActionExtension
4. **创建 MockLLM**：实现 `model.LLM` 接口，返回固定的 Decision JSON，使 Agent 在第一轮就终止循环
5. **组装 Agent**：通过 `agent.New(sess, actExt, llm)` 组装 Agent，配置最大轮次为 5
6. **运行 Agent**：调用 `ag.Run(ctx, "Hello, please greet me.")` 执行 Agent
7. **验证 Session**：检查 `sess.FullContext` 和 `sess.ContextWindow` 确认对话已被记录

1. **Create Session**：Call `session.NewSession("quickstart-demo", 4000)` to create a conversation session with a max context length of 4000
2. **Create ActionExtension**：Create a tool registry via `agent.NewActionExtension()`
3. **Register Actions**：Use `action.New()` to wrap two Go functions -- `greet` and `add` -- with automatic JSON Schema generation, then register them into the ActionExtension
4. **Create MockLLM**：Implement the `model.LLM` interface to return a fixed Decision JSON, causing the Agent to terminate after the first round
5. **Assemble Agent**：Build the Agent via `agent.New(sess, actExt, llm)` with a max round setting of 5
6. **Run Agent**：Execute the Agent by calling `ag.Run(ctx, "Hello, please greet me.")`
7. **Verify Session**：Inspect `sess.FullContext` and `sess.ContextWindow` to confirm the conversation was recorded

## 运行验证 / Running the Example

```
cd examples
go run example_quickstart.go
```

程序将依次输出 Session 创建、Action 注册、MockLLM 创建、Agent 组装和运行结果，最后展示 Session 中记录的对话上下文内容。

The program will sequentially output Session creation, Action registration, MockLLM creation, Agent assembly and execution results, and finally display the conversation context recorded in the Session.

## 预期输出 / Expected Output

输出将包含以下关键信息 / The output will contain the following key information:

- `Session created: ID = quickstart-demo` -- Session 创建成功
- `Action registered: greet` 和 `Action registered: add` -- 两个 Action 注册成功
- `MockLLM created` -- MockLLM 创建成功
- `Agent created: maxRounds=5` -- Agent 组装完成
- `Agent response: Hello from inferglow Agent!...` -- Agent 执行结果
- Session 的 ContextWindow 中记录了 user 和 assistant 的对话消息

- `Session created: ID = quickstart-demo` -- Session created successfully
- `Action registered: greet` and `Action registered: add` -- Both Actions registered successfully
- `MockLLM created` -- MockLLM created successfully
- `Agent created: maxRounds=5` -- Agent assembled successfully
- `Agent response: Hello from inferglow Agent!...` -- Agent execution result
- The Session's ContextWindow records user and assistant conversation messages

该输出表明 Agent 的完整 PLAN-EXECUTE 循环已成功运行，对话历史被正确记录在 Session 中。

This output confirms that the Agent's full PLAN-EXECUTE loop has run successfully and the conversation history is correctly recorded in the Session.