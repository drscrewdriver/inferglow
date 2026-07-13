# InferGlow Comprehensive Server Example

## Overview / 概述

This example demonstrates the **full system capabilities** of InferGlow via the server module's REST API. It covers all 10 functional layers, including server initialization, REST API endpoints, and the audit switch (enabled/disabled modes).

本示例通过 server 模块的 REST API 展示 InferGlow 的**完整系统能力**，涵盖全部 10 个功能层，包括服务器初始化、REST API 端点以及审计开关（启用/禁用模式）。

**File / 文件**: `example_server_comprehensive.go`
**Run / 运行**: `go run example_server_comprehensive.go`
**Sandbox mode / 沙箱模式**: `go run -tags with_sandbox example_server_comprehensive.go`

---

## Core Concepts / 核心概念

The example demonstrates the following layers in order:

本示例按顺序演示以下层次：

1. **Server Layer** -- Server initialization with `NewServerWithFlows`, `FlowStore`, `StageRegistry`, and `InMemoryStore`. / 服务器层：通过 `NewServerWithFlows`、`FlowStore`、`StageRegistry` 和 `InMemoryStore` 进行服务器初始化。
2. **Model Layer** -- MockLLM provider implementation (no API key required) demonstrating `GenerateRequestData` and `RequestModel`. / 模型层：MockLLM Provider 实现（无需 API Key），演示 `GenerateRequestData` 和 `RequestModel`。
3. **Action Layer** -- Tool registration with `action.NewRegistry()`, registering three tools (add, greet, weather). / 动作层：使用 `action.NewRegistry()` 注册三个工具（add、greet、weather）。
4. **Session Layer** -- Conversation memory management with context window, auto-resize, and `SimpleCutResizeHandler`. / 会话层：对话记忆管理，含上下文窗口、自动裁剪和 `SimpleCutResizeHandler`。
5. **Audit Layer** -- Audit switch: enabled mode with HMAC-SHA256 signing + chain verification, disabled mode with zero-overhead no-op. / 审计层：审计开关——启用模式使用 HMAC-SHA256 签名+链验证，禁用模式为零开销无操作。
6. **Flow Layer** -- Step orchestration with linear flow, conditional branching, and declarative `FlowDef` registration. / 流程层：步骤编排，含线性流程、条件分支和声明式 `FlowDef` 注册。
7. **Schema Layer** -- Output schema definition via `DefineOutput[T]`, JSON Schema conversion, and manual schema construction. / 模式层：通过 `DefineOutput[T]` 定义输出模式、JSON Schema 转换和手动构建模式。
8. **Sandbox Layer** -- Isolated execution with `sandbox.Manager` and `TrustedLocalProvider` (stub without `-tags with_sandbox`). / 沙箱层：使用 `sandbox.Manager` 和 `TrustedLocalProvider` 进行隔离执行（无 `-tags with_sandbox` 时为桩实现）。
9. **Workspace Layer** -- Safe file I/O with path traversal protection, `MkdirAll`/`WriteFile`/`ReadFile`, and file lineage tracking. / 工作区层：安全文件操作，含路径穿越防护、`MkdirAll`/`WriteFile`/`ReadFile` 和文件血缘追踪。
10. **Server REST API** -- 16 endpoint calls via `httptest` covering health check, agent CRUD, chat, tools, memories, flows, stages, audit, sessions, and OpenAPI spec. / REST API：通过 `httptest` 调用 16 个端点，涵盖健康检查、Agent CRUD、聊天、工具、记忆、流程、阶段、审计、会话和 OpenAPI 规范。

---

## Prerequisites / 前置条件

- Go 1.25+ installed
- InferGlow modules accessible via `go.mod` replace directives (all local modules under `../`)

- 已安装 Go 1.25+
- 通过 `go.mod` replace 指令可访问 InferGlow 模块（所有本地模块位于 `../` 下）

---

## Usage Example / 使用示例

The following snippet shows the server initialization pattern used in the example:

以下片段展示了示例中使用的服务器初始化模式：

```go
// Create default config
srvConfig := server.DefaultConfig()
srvConfig.Addr = ":8080"

// Create AgentStore
agentStore := newMockAgentStore()

// Create Stage Registry and register stage functions
stageReg := stage.NewRegistry()
stageReg.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
    return stage.Outputs{"message": in["message"]}, nil
})
stagebuiltin.RegisterAll(stageReg)

// Create FlowStore and Server
flowStore := server.NewFlowStore(stageReg)
srv := server.NewServerWithFlows(srvConfig, agentStore, flowStore)

// Configure memory store
memStore := server.NewInMemoryStore()
srv.SetMemoryStore(memStore)

// Create audit chain (enabled mode)
enabledChain, _ := audit.NewAuditChain(audit.AuditConfig{
    Enabled:      true,
    SignatureKey: []byte("demo-secret-key-12345"),
    MaxEntries:   100,
})
srv.SetAuditChain(enabledChain)
```

---

## Running Verification / 运行验证

Run the example to see all 10 layers demonstrated:

运行示例以查看全部 10 个层次的演示：

```bash
cd e:\test\rewrite-agently\inferglow\examples
go run example_server_comprehensive.go
```

Expected output structure (truncated):

预期输出结构（截断）：

```
======================================================================
  InferGlow 综合 Server 示例 / Comprehensive Server Example
======================================================================

------------------------------------------------------------------------
  [Part 1] Server 初始化与配置 / Server Init & Config
------------------------------------------------------------------------
  Server config: addr= :8080
  ...
  Server instance created with FlowStore

------------------------------------------------------------------------
  [Part 2] Model 层 / Model Layer -- MockLLM Provider
------------------------------------------------------------------------
  MockLLM created (no API key needed)
  ...

...

------------------------------------------------------------------------
  [Part 10] Server REST API 调用 / REST API Calls
------------------------------------------------------------------------
  --- 10.1 Health Check ---
  GET /health -> status: 200
  ...

======================================================================
  Summary / 总结
======================================================================
  Layers demonstrated:
    1. Server initialization and configuration
    ...
   10. Server REST API: 16 endpoint calls via httptest
  Audit switch: enabled=true (HMAC-SHA256), enabled=false (zero overhead)
  Sandbox mode: requires -tags with_sandbox for real execution

  All examples completed successfully!
```

For sandbox mode with real execution:

如需沙箱真实执行模式：

```bash
go run -tags with_sandbox example_server_comprehensive.go
```