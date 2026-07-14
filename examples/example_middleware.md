# example_middleware.go — 洋葱模型中间件

## 中文说明

演示 `orchestrator/middleware` 包的洋葱模型中间件链，支持日志、计时、元数据注入等横切关注点。

### 核心概念
- **Handler**：`func(ctx, *Input) (*Output, error)` — 统一处理函数签名
- **Middleware**：`func(next Handler) Handler` — 中间件包装器
- **Chain**：compose 多个中间件为洋葱链（外层先进入、后退出）
- **Input/Output**：结构化的输入输出消息载体

### 运行方式
```bash
cd examples
go run example_middleware.go
```

### 示例输出
```
=== Example: Middleware Chain ===
Final output: [LOG] [TIMING: 0ms] echo: Hello, Middleware!
Metadata: [echo middleware_count=3 pipeline=logging->timing->metadata]
```

---

## English Description

Demonstrates the onion-model middleware chain from `orchestrator/middleware` for cross-cutting concerns like logging, timing, and metadata injection.

### Key Concepts
- **Handler**: Unified function signature `func(ctx, *Input) (*Output, error)`
- **Middleware**: Wrapper `func(next Handler) Handler`
- **Chain**: Compose middlewares into onion chain (outermost enters first, exits last)
- **Input/Output**: Structured message carrier

### Run
```bash
cd examples
go run example_middleware.go
```