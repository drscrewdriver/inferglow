# InferGlow Extension Mechanisms

> 本文档描述 InferGlow 的 7 种扩展机制及其正确使用场景。
> **核心原则：不增加第 8 种机制。** 新功能优先选择语义最匹配的已有机制。

---

## 决策树

```
需要拦截请求/响应？        → Middleware (#2)
需要解耦外部依赖？          → Interface Injection (#1)
需要在执行生命周期注入回调？ → Callbacks (#3)
需要条件编译/平台适配？      → Build Tags (#4)
需要切换压缩策略？          → Context Manager Modes (#5)
需要运行时动态注册流程块？   → Block Registry (#6)
需要切换 Session 压缩策略？  → Resize Handlers (#7)
```

---

## 1. Interface Injection

**使用场景**：依赖反转——编排层定义接口，能力层实现。编译期安全，零 import 耦合。

**示例**：

```go
// session.MessageHook — 安全检测拦截
type MessageHook interface {
    Check(text string) (pass bool, reason string)
}

// agent.PIIMasker — PII 脱敏
type PIIMasker interface {
    MaskInput(text string) string
    MaskOutput(text string) string
}

// session.MessageMasker — 同上，session 层的接口
type MessageMasker interface {
    MaskInput(text string) string
    MaskOutput(text string) string
}
```

**何时使用**：
- 编排层需要调用能力层的功能，但不想直接 import 能力层包
- 需要多个不同的实现（如不同的 PII 策略）
- 需要编译期类型安全

**何时不使用**：
- 简单的函数回调 → 用 Callbacks (#3)
- 需要运行时动态切换 → 用 Mode (#5)

---

## 2. Middleware Chain

**使用场景**：请求/响应拦截——OTel tracing、audit、recovery、rate limit。

**统一签名**（`orchestrator/middleware/`）：

```go
type Handler func(ctx context.Context, input *Input) (*Output, error)
type Middleware func(next Handler) Handler

// 使用 middleware.Chain 组合
handler := middleware.Chain(otelMW, auditMW, recoveryMW)(coreHandler)
```

**旧签名**（`orchestrator/agent/`，已 Deprecated）：

```go
type AgentHandler func(ctx context.Context, userMessage string) (string, error)
type Middleware func(next AgentHandler) AgentHandler
```

**何时使用**：
- 需要在请求进入/退出时执行横切逻辑
- 需要组合多个拦截器（链式）
- 需要同时用于 agent 和 team

**何时不使用**：
- 只需要在特定时机触发（如 "run 开始前"）→ 用 Callbacks (#3)
- 需要改变执行流程（如重试）→ 在 Engine 内部处理

---

## 3. Callbacks

**使用场景**：执行生命周期回调——在特定时机触发通知/记录。

```go
type AgentCallbacks struct {
    OnRunStart        func(ctx context.Context, msg string)
    OnRunEnd          func(ctx context.Context, result string, err error)
    OnLLMCallStart    func(ctx context.Context, model string)
    OnLLMCallEnd      func(ctx context.Context, usage *model.UsageInfo)
    OnToolCallStart   func(ctx context.Context, toolName string)
    OnToolCallEnd     func(ctx context.Context, toolName string, err error)
}
```

**何时使用**：
- 只需要 "fire and forget" 的通知
- 不需要改变执行流程
- 与特定执行阶段绑定

**何时不使用**：
- 需要拦截并修改输入/输出 → 用 Middleware (#2)
- 需要阻止执行 → 用 Interface Injection (#1) 的 Hook

---

## 4. Build Tags

**使用场景**：条件编译——平台特定代码、可选依赖。

```go
// features_sandbox.go
//go:build with_sandbox

// features_sandbox_stub.go
//go:build !with_sandbox
```

**何时使用**：
- 功能依赖特定系统库（如 sandbox 依赖 seccomp/bwrap）
- 需要在编译期完全排除代码
- 不同平台有不同实现

**何时不使用**：
- 运行时可切换的策略 → 用 Modes (#5)
- 可选但非平台绑定的功能 → 用 Interface Injection (#1)

---

## 5. Context Manager Modes

**使用场景**：策略选择——通过 mode 字符串切换压缩/管理策略。

```go
// context/ 模块支持的模式
const (
    ModePassthrough = "passthrough"  // 不压缩
    ModeThreeZone   = "three_zone"   // 三区模型（hot/warm/cold）
    ModeHybrid      = "hybrid"       // 混合策略
)
```

**何时使用**：
- 同一功能有多种实现策略
- 通过配置/环境变量切换
- 策略之间共享接口但行为不同

**何时不使用**：
- 编译期确定的平台差异 → 用 Build Tags (#4)
- 运行时动态注册 → 用 Registry (#6)

---

## 6. Block Registry

**使用场景**：运行时动态注册流程块——Blueprint → Operator 编译。

```go
// flow/ 模块
type FlowBlock interface {
    Name() string
    Execute(ctx context.Context, input any) (any, error)
}

type BlockRegistry struct { ... }
func (r *BlockRegistry) Register(block FlowBlock) { ... }
```

**何时使用**：
- 需要在运行时动态添加可执行块
- Blueprint 声明式定义 → 编译为可执行 DAG
- 插件式扩展 Flow 的能力

**何时不使用**：
- 编译期已知的固定步骤 → 直接用 `flow.NewStep()`
- 非流程编排的扩展 → 用对应的机制

---

## 7. Resize Handlers

**使用场景**：Session 压缩策略插拔。

```go
// session/ 模块
type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// 内置策略
session.RegisterResizeHandler("simple_cut", session.SimpleCutResizeHandler)
session.RegisterResizeHandler("summary_first", session.SummaryFirstResizeHandler)
session.RegisterResizeHandler("token_aware", session.TokenAwareResizeHandler)
```

**何时使用**：
- 需要自定义 Session 压缩行为
- 按 token 预算管理上下文窗口
- 需要不同的摘要策略

**何时不使用**：
- 不需要压缩（上下文很短）→ 用 `passthrough` mode (#5)
- 需要三区管理 → 用 `three_zone` mode (#5)

---

## 反模式

| 反模式 | 正确做法 |
|--------|---------|
| 创建新的 Plugin 接口 | 使用已有的 7 种机制之一 |
| 全局 EventBus | 用 `context.Context` 传播 + Interface Injection |
| Service Locator | 用构造函数注入 + Interface Injection |
| 在 agent 包直接 import otel | 用 ports.go 接口 + Middleware (#2) |
| 硬编码压缩策略 | 用 Resize Handler (#7) 或 Mode (#5) |

---

## 总结

InferGlow 的 7 种扩展机制覆盖了所有真实需求。添加新功能时，首先判断其本质：

- **拦截** → Middleware (#2)
- **解耦** → Interface Injection (#1)
- **通知** → Callbacks (#3)
- **条件编译** → Build Tags (#4)
- **策略切换** → Mode (#5) 或 Registry (#6)
- **压缩** → Resize Handler (#7)

**如果以上都不匹配，先与核心维护者讨论，再考虑是否需要新机制。**
