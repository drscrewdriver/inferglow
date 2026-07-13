# example_session - 对话记忆管理示例 / Session Memory Management Example

## 概述 / Overview

本示例展示了如何使用 `session` 模块管理 Agent 的对话记忆。示例涵盖了 Session 的基本创建与消息添加、上下文窗口自动裁剪、摘要裁剪策略、Token 感知裁剪以及持久化（JSON/YAML 序列化）五种核心功能。

This example demonstrates how to use the `session` module to manage Agent conversation memory. It covers five core capabilities: basic Session creation and message addition, automatic context window resizing, summary-based resize strategy, token-aware resizing, and persistence (JSON/YAML serialization).

## 核心概念 / Core Concepts

- **Session（会话）**：管理对话历史和上下文窗口的核心数据结构
- **FullContext（完整上下文）**：记录所有消息，永不裁剪
- **ContextWindow（上下文窗口）**：根据 MaxLength 限制当前可见的对话片段，可按策略自动裁剪
- **AutoResize（自动裁剪）**：当消息数超过 MaxLength 时自动触发裁剪
- **ResizeHandler（裁剪策略）**：多种裁剪算法，包括 `SimpleCutResizeHandler`（简单截断）、`SummaryFirstResizeHandler`（摘要优先）和 `TokenAwareResizeHandler`（Token 感知）
- **持久化**：支持将 Session 序列化为 JSON 或 YAML 格式

- **Session**：The core data structure managing conversation history and the context window
- **FullContext**：Records all messages, never trimmed
- **ContextWindow**：Limits the currently visible conversation segment based on MaxLength, with automatic resizing by strategy
- **AutoResize**：Automatically triggers resizing when the message count exceeds MaxLength
- **ResizeHandler**：Multiple resizing algorithms, including `SimpleCutResizeHandler` (simple truncation), `SummaryFirstResizeHandler` (summary-first), and `TokenAwareResizeHandler` (token-aware)
- **Persistence**：Supports serializing Session to JSON or YAML format

## 前置条件 / Prerequisites

- Go 1.21+
- inferglow 项目依赖已安装（`go mod tidy`）
- 无需 LLM API Key

- Go 1.21+
- inferglow project dependencies installed (`go mod tidy`)
- No LLM API Key required

## 使用示例 / Usage Example

代码流程如下 / The code flow is as follows:

**示例 1：基本使用 / Example 1: Basic Usage**
- 创建 Session：`session.NewSession("demo-session", 1000)`
- 添加 5 条消息（system、user、assistant 角色交替）
- 调用 `sess.PreparePrompt()` 获取用于 LLM 的消息列表

**示例 2：上下文窗口自动裁剪 / Example 2: Context Window Auto-Resize**
- 创建 MaxLength=200 的 Session，启用 AutoResize
- 注册 `SimpleCutResizeHandler` 作为裁剪策略
- 添加 40 条消息，观察 ContextWindow 被自动裁剪

**示例 3：摘要裁剪策略 / Example 3: Summary Resize Strategy**
- 创建 MaxLength=300 的 Session，启用 AutoResize
- 注册 `SummaryFirstResizeHandler`（优先保留早期消息，裁剪中间部分）
- 添加 20 条消息，观察 ContextWindow 的裁剪效果

**示例 4：Token 感知裁剪 / Example 4: Token-Aware Resize**
- 创建 MaxLength=8000 的 Session，启用 AutoResize
- 注册 `TokenAwareResizeHandler`（基于 Token 数量而非消息条数进行裁剪）
- 添加 30 条较长消息，观察 Token 感知裁剪效果

**示例 5：持久化 / Example 5: Persistence**
- 添加 2 条消息后，调用 `sess.ToJSON()` 和 `sess.ToYAML()` 序列化 Session
- 输出 JSON 和 YAML 格式的内容

**Example 1: Basic Usage**
- Create a Session: `session.NewSession("demo-session", 1000)`
- Add 5 messages (alternating system, user, and assistant roles)
- Call `sess.PreparePrompt()` to get the message list for LLM consumption

**Example 2: Context Window Auto-Resize**
- Create a Session with MaxLength=200 and AutoResize enabled
- Register `SimpleCutResizeHandler` as the resize strategy
- Add 40 messages and observe the ContextWindow being automatically trimmed

**Example 3: Summary Resize Strategy**
- Create a Session with MaxLength=300 and AutoResize enabled
- Register `SummaryFirstResizeHandler` (prioritizes keeping early messages, trims the middle)
- Add 20 messages and observe the trimming effect on ContextWindow

**Example 4: Token-Aware Resize**
- Create a Session with MaxLength=8000 and AutoResize enabled
- Register `TokenAwareResizeHandler` (trims based on token count rather than message count)
- Add 30 longer messages and observe the token-aware trimming effect

**Example 5: Persistence**
- After adding 2 messages, call `sess.ToJSON()` and `sess.ToYAML()` to serialize the Session
- Output the JSON and YAML formatted content

## 运行验证 / Running the Example

```
cd examples
go run example_session.go
```

## 预期输出 / Expected Output

输出将包含以下关键信息 / The output will contain the following key information:

- **基本使用**：Session 创建成功，ID 和 MaxLength 显示正确，`PreparePrompt()` 返回 5 条消息
- **自动裁剪**：FullContext 包含全部 40 条消息，ContextWindow 被裁剪至约 200 条以内
- **摘要优先**：FullContext 包含全部 20 条消息，ContextWindow 保留了首尾消息
- **Token 感知**：30 条消息被裁剪至符合 8000 Token 上限的窗口大小
- **持久化**：输出 JSON 字符串的前 100 个字符和完整的 YAML 内容

- **Basic Usage**: Session created successfully with correct ID and MaxLength, `PreparePrompt()` returns 5 messages
- **Auto-Resize**: FullContext contains all 40 messages, ContextWindow is trimmed within the ~200 limit
- **Summary First**: FullContext contains all 20 messages, ContextWindow preserves the first and last messages
- **Token-Aware**: 30 messages trimmed to fit within the 8000 token limit
- **Persistence**: Outputs the first 100 characters of the JSON string and the full YAML content

该输出表明 session 模块支持完整的对话记忆管理功能，包括消息添加、多种裁剪策略和持久化序列化。

This output confirms that the session module supports complete conversation memory management capabilities, including message addition, multiple resize strategies, and persistence serialization.