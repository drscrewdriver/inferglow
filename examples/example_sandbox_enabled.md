# example_sandbox_enabled - 沙箱模式 / Sandbox Mode

## 概述 / Overview

本示例演示 InferGlow 的沙箱模式，即通过 `-tags with_sandbox` 编译标签启用真实沙箱执行能力。沙箱模式下，`SandboxExecutor` 使用 `sandbox.Manager` 调用后端 Provider（如 TrustedLocalProvider）来执行隔离命令，确保 Action 执行在安全隔离的环境中。默认编译模式下此文件不参与编译，`SandboxExecutor` 为返回错误的 stub 实现。

This example demonstrates InferGlow's sandbox mode, enabled via the `-tags with_sandbox` build tag. In sandbox mode, the `SandboxExecutor` uses `sandbox.Manager` to call backend providers (such as TrustedLocalProvider) for isolated command execution, ensuring Actions run in a secure, isolated environment. In default build mode, this file is excluded from compilation, and `SandboxExecutor` is a stub implementation that returns errors.

## 核心概念 / Core Concepts

- **with_sandbox 编译标签 / with_sandbox Build Tag**: 通过 `go build -tags with_sandbox` 启用真实沙箱实现
- **SandboxExecutor / 沙箱执行器**: 在沙箱环境中执行命令的 Action 封装，隔离潜在危险操作
- **sandbox.Manager / 沙箱管理器**: 管理沙箱 Provider 的注册和调度
- **TrustedLocalProvider / 可信本地提供者**: 在本地但受限环境中执行命令的 Provider
- **条件编译 / Conditional Compilation**: 通过 `//go:build with_sandbox` 实现文件级条件编译，默认模式完全排除沙箱依赖

## 前置条件 / Prerequisites

- Go 1.21+
- 需要 `-tags with_sandbox` 编译标签运行
- 沙箱后端 Provider（如 TrustedLocalProvider）需要在系统中可用
- 本示例使用 MockLLM，不依赖真实 LLM API Key
- Requires the `-tags with_sandbox` build tag to run
- Sandbox backend provider (e.g., TrustedLocalProvider) must be available in the system
- This example uses MockLLM and does not require a real LLM API Key

## 使用示例 / Usage Example

代码演示了以下 5 个步骤：

1. **创建 SandboxExecutor**: 使用 `sandbox.NewManager` 创建管理器，注册 `TrustedLocalProvider`，然后通过 `action.NewSandboxExecutor` 构造沙箱执行器。
2. **直接沙箱执行**: 调用 `sandboxExec.Execute` 执行 `echo "hello from sandbox"` 命令，展示沙箱执行结果。
3. **注册沙箱 Action**: 将 SandboxExecutor 包装为 `action.Action`，注册到 `ActionExtension` 中，使其可被 LLM 调用。
4. **组装完整功能 Agent**: 创建 Session、ActionExtension，注册沙箱 Action，使用 `agent.New` 组装 Agent。
5. **运行 Agent**: 执行 `ag.Run`，MockLLM 返回 response 决策终止循环。

The code demonstrates 5 steps:

1. **Creating SandboxExecutor**: Create a manager with `sandbox.NewManager`, register `TrustedLocalProvider`, then construct the sandbox executor via `action.NewSandboxExecutor`.
2. **Direct Sandbox Execution**: Call `sandboxExec.Execute` to run the `echo "hello from sandbox"` command, displaying the sandbox execution result.
3. **Registering Sandbox Action**: Wrap the SandboxExecutor as an `action.Action` and register it with `ActionExtension`, making it callable by the LLM.
4. **Assembling a Full-Featured Agent**: Create Session, ActionExtension, register the sandbox Action, and assemble the Agent with `agent.New`.
5. **Running the Agent**: Execute `ag.Run`, where MockLLM returns a response decision to terminate the loop.

## 运行验证 / Running the Example

```
cd examples
go run -tags with_sandbox example_sandbox_enabled.go
```

预期输出会依次展示：

- SandboxExecutor 创建成功，显示为真实实现（非 stub）
- 沙箱执行命令成功，显示 stdout 输出 "hello from sandbox"
- 沙箱 Action 注册成功
- Agent 组装完成，显示配置信息
- Agent.Run 执行完成，返回沙箱模式下 Agent 的响应结果
- 沙箱模式总结，说明编译标签、Provider 注册和安全特性独立性的关系

Expected output shows:

- SandboxExecutor created successfully, shown as real implementation (not stub)
- Sandbox command execution succeeds, displaying stdout output "hello from sandbox"
- Sandbox Action registered successfully
- Agent assembled, showing configuration info
- Agent.Run completed, returning the sandbox-mode Agent response
- Sandbox mode summary, explaining the relationship between build tags, provider registration, and security feature independence

## 预期输出 / Expected Output

输出着重展示沙箱模式的隔离执行能力和条件编译设计：`with_sandbox` 编译标签确保沙箱依赖不会进入默认编译产物，保持核心库的轻量；SandboxExecutor 通过 Manager 调用 Provider 实现执行隔离，为 LLM 生成的代码/命令执行提供安全沙箱。

The output highlights the sandbox mode's isolated execution capability and conditional compilation design: the `with_sandbox` build tag ensures sandbox dependencies are not included in default build artifacts, keeping the core library lightweight; the SandboxExecutor achieves execution isolation by calling providers through the Manager, providing a secure sandbox for LLM-generated code/command execution.