# example_pluggable - 可插拔架构 / Pluggable Architecture

## 概述 / Overview

本示例演示 InferGlow v2 可插拔架构的核心设计模式。通过接口注入的方式，在不引入额外依赖的情况下按需启用安全特性。展示了三种模式：零开销模式（无安全钩子）、接口注入模式（sessionhook + agenthook + PII 脱敏），以及部分启用模式（仅 PII 脱敏）。该架构允许 orchestrator/session 不直接依赖 security 子包，保持核心模块的轻量。

This example demonstrates the core design pattern of the InferGlow v2 pluggable architecture. Security features are enabled on demand through interface injection without introducing additional dependencies. It showcases three modes: zero-overhead mode (no security hooks), interface injection mode (sessionhook + agenthook + PII masking), and partial enablement mode (PII masking only). This architecture allows orchestrator/session to avoid direct dependencies on the security sub-packages, keeping the core modules lightweight.

## 核心概念 / Core Concepts

- **零开销模式 / Zero-Overhead Mode**: 默认编译模式，不引入任何安全钩子，所有检查跳过
- **接口注入 / Interface Injection**: 通过 `session.MessageHook`、`agent.OutputSecurityHook`、`agent.PIIMasker` 接口契约注入安全特性，无需编译标签
- **sessionhook.SecurityHook / 会话安全钩子**: 实现 `session.MessageHook`，在输入侧检测提示注入（Prompt Injection）
- **agenthook.OutputInjectionHook / 输出注入检测钩子**: 实现 `agent.OutputSecurityHook`，在输出侧检测注入攻击
- **agenthook.PIIMasker / PII 脱敏器**: 实现 `agent.PIIMasker`，对输出中的敏感信息（如邮箱）进行脱敏
- **独立安全接口 / Independent Security Interfaces**: 三个安全接口（MessageHook / OutputSecurityHook / PIIMasker）相互独立，可按需组合

## 前置条件 / Prerequisites

- Go 1.21+
- 无需特殊编译标签，默认 `go build` 即可运行
- 本示例使用 MockLLM，不依赖真实 LLM API Key
- No special build tags required; runs with default `go build`
- This example uses MockLLM and does not require a real LLM API Key

## 使用示例 / Usage Example

代码演示了 3 种模式：

1. **模式 A：零开销模式**: 不使用任何安全钩子，`session.NewSession` 和 `agent.New` 均不注入安全接口。`AddMessageChecked` 和 `Agent.Run` 跳过所有检查，性能开销为零。
2. **模式 B：接口注入安全特性**: 同时注入三个安全接口：
   - `sessionhook.NewSecurityHook` 注入输入侧提示注入检测
   - `agenthook.NewOutputInjectionHook` 注入输出侧注入检测
   - `agenthook.NewPIIMasker` 注入 PII 脱敏（示例输入包含邮箱 `test@example.com`）
3. **模式 C：仅启用 PII 脱敏**: 演示安全接口的独立性，仅注入 PIIMasker，不启用注入检测。

The code demonstrates 3 modes:

1. **Mode A: Zero-Overhead Mode**: No security hooks are used. `session.NewSession` and `agent.New` are called without any security interface injection. `AddMessageChecked` and `Agent.Run` skip all checks, resulting in zero performance overhead.
2. **Mode B: Interface Injection with Security Features**: Three security interfaces are injected simultaneously:
   - `sessionhook.NewSecurityHook` injects input-side prompt injection detection
   - `agenthook.NewOutputInjectionHook` injects output-side injection detection
   - `agenthook.NewPIIMasker` injects PII masking (example input includes email `test@example.com`)
3. **Mode C: PII Masking Only**: Demonstrates the independence of security interfaces by injecting only the PIIMasker without enabling injection detection.

## 运行验证 / Running the Example

```
cd examples
go run example_pluggable.go
```

预期输出会依次展示：

- 模式 A：Agent 响应正常，无安全检查开销
- 模式 B：三个安全接口注入成功，Agent 响应经过 PII 脱敏处理
- 模式 C：仅 PII 脱敏启用，Agent 响应正常
- 架构总结，说明默认模式、接口注入和沙箱模式的关系

Expected output shows:

- Mode A: Agent responds normally, no security check overhead
- Mode B: Three security interfaces injected successfully, Agent response is PII-masked
- Mode C: Only PII masking enabled, Agent responds normally
- Architecture summary explaining the relationship between default mode, interface injection, and sandbox mode

## 预期输出 / Expected Output

输出着重展示可插拔架构的灵活性和安全性：开发者可以根据场景需求零成本地启用所需的安全特性，而无需修改核心编排代码。安全特性的注入完全通过接口契约实现，保持了模块间的低耦合。

The output highlights the flexibility and security of the pluggable architecture: developers can enable required security features at zero cost based on scenario needs, without modifying core orchestration code. Security feature injection is entirely implemented through interface contracts, maintaining low coupling between modules.