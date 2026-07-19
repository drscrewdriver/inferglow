# InferGlow V6 S1+S2+S3 结构修复实施计划

## 概览

按 ARCHITECTURE_V6.md 第一梯队，实施三项结构修复：
- **S1**: `orchestrator/agent` → `observability/otel` 7 处直接 import 改为接口注入（~2h）
- **S2**: `session/contextmgr/` → 独立 `context/` module（~1d）
- **S3**: 新增统一 `middleware/` 包，旧签名 deprecated 共存（~1d）

执行顺序：S1 → S2 → S3（S1 最小风险先验证，S2 纯机械迁移，S3 依赖新包创建）

---

## S1: 解耦 `agent → otel`（接口注入）

### 核心设计

`SpanStarter` 接口签名匹配标准 `trace.Tracer.Start`（不含 SpanKind），语义 span name 通过 agent 包内 `semanticSpanName()` 函数映射。`*otel.Tracer` 天然满足该接口（其 `StartSpan` 内部就是调用 `t.tr.Start`）。

### 步骤

**1.1** 创建 `orchestrator/agent/ports.go`（新文件）
- 定义 `SpanStarter` 接口：`Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)`
- 定义 `SemanticSpanKind` 枚举 + 6 个常量（SpanAgentRun/SpanLLMCall/SpanToolCall/SpanFlowExecute/SpanPause/SpanResume）
- 定义 `semanticSpanName(kind SemanticSpanKind, name string) string`（从 `otel/tracer.go:74-94` 复制映射逻辑）
- 定义 `noopSpanStarter{}` 实现

**1.2** 修改 `observability/otel/tracer.go`
- 新增 `Start(ctx, spanName, opts...)` 方法（委托给 `t.tr.Start`），使 `*otel.Tracer` 满足 `SpanStarter`
- 保留 `StartSpan` 方法不变（向后兼容）

**1.3** 修改 `orchestrator/agent/agent.go`
- L31: 删除 `"github.com/inferglow/observability/otel"` import
- L105: `tracer *otel.Tracer` → `tracer SpanStarter`
- L155: `tracer *otel.Tracer` → `tracer SpanStarter`
- L208: `WithTracer(t *otel.Tracer)` → `WithTracer(t SpanStarter)`

**1.4** 修改 `orchestrator/agent/engine.go`
- L38: 删除 otel import
- L118: `tracer *otel.Tracer` → `tracer SpanStarter`

**1.5** 修改 `orchestrator/agent/callbacks_tracer.go`
- L28: 删除 otel import（保留 `go.opentelemetry.io/otel/trace`）
- L49: `tracer *otel.Tracer` → `tracer SpanStarter`
- L60: `NewCallbacksTracer` 参数改为 `SpanStarter`
- 所有 `otel.SpanXxx` 常量替换为 `semanticSpanName(SpanXxx, "")` 调用

**1.6** 修改 `orchestrator/agent/flow_context_impl.go`
- L32: 删除 otel import
- L51: `tracer *otel.Tracer` → `tracer SpanStarter`
- L80: `flowToOtelKind` 返回 `SemanticSpanKind`（本地类型）
- L210: `fc.tracer.StartSpan(ctx, kind, name)` → `fc.tracer.Start(ctx, semanticSpanName(flowToOtelKind(kind), name))`

**1.7** 修改 `orchestrator/agent/flow_exec.go`
- L30: 删除 otel import
- L166: `startFlowSpan` 签名改为 `(ctx, tracer SpanStarter, kind SemanticSpanKind, name string)`
- L170: `tracer.StartSpan(ctx, kind, name)` → `tracer.Start(ctx, semanticSpanName(kind, name))`

**1.8** 修改 `orchestrator/agent/resume_flow.go`
- L28: 删除 otel import
- L48: `otel.SpanResume` → `semanticSpanName(SpanResume, "")`

**1.9** 修改 `orchestrator/agent/flow_context_impl_a3_test.go`
- L33: 保留 otel import（测试需要 `otel.NewTracer`）
- `*otel.Tracer` 赋值给 `tracer SpanStarter` 字段（`*otel.Tracer` 满足接口，编译通过）

**1.10** 更新 `orchestrator/go.mod`
- `observability` 降为 indirect（测试文件仍需要）
- 保留 `replace` 指令

**1.11** 验证：`cd orchestrator && go build ./... && go test ./agent/...`

### 关键文件
- `orchestrator/agent/ports.go`（新建）
- `orchestrator/agent/agent.go`
- `orchestrator/agent/flow_exec.go`
- `orchestrator/agent/flow_context_impl.go`
- `observability/otel/tracer.go`

---

## S2: `session/contextmgr/` → 独立 `context/` module

### 核心设计

纯机械性目录迁移。`contextmgr` 已完全不 import `session` 包（已验证 0 处 session import），只需物理移动 + 更新 import path。包名保持 `contextmgr`（避免与 stdlib `context` 冲突），module path 为 `github.com/inferglow/context`。

### 步骤

**2.1** 创建 `context/` 目录结构
- `mkdir -p inferglow/context/`
- 移动 `session/contextmgr/*` → `context/`（含 compress/, retrieval/, store/, tools/ 子包）
- 删除 `session/contextmgr/` 空目录

**2.2** 创建 `context/go.mod`
```
module github.com/inferglow/context
go 1.25.0
```
- 检查各子包依赖（store/postgres 可能需要 pg 驱动等），按需添加

**2.3** 全局替换 import path
- `github.com/inferglow/session/contextmgr` → `github.com/inferglow/context`
- `github.com/inferglow/session/contextmgr/compress` → `github.com/inferglow/context/compress`
- `github.com/inferglow/session/contextmgr/retrieval` → `github.com/inferglow/context/retrieval`
- `github.com/inferglow/session/contextmgr/store` → `github.com/inferglow/context/store`
- `github.com/inferglow/session/contextmgr/tools` → `github.com/inferglow/context/tools`
- 涉及文件（已确认仅 contextmgr 内部自引用 + rag/embedding.go 注释）

**2.4** 更新 `session/go.mod`
- 确认无 contextmgr 相关依赖需清理（当前 session/go.mod 仅依赖 yaml.v3，无需改动）

**2.5** 更新 workspace 配置（如有 `go.work`）
- 添加 `./context` 到 workspace

**2.6** 验证
- `cd context && go build ./...`
- `cd session && go build ./...`
- `cd orchestrator && go build ./...`（确认无影响）

### 关键文件
- `context/go.mod`（新建）
- `context/manager.go`（从 session/contextmgr/manager.go 移来）
- `context/compress/engine.go`
- `context/store/store.go`
- `session/go.mod`

---

## S3: 统一 `middleware.Handler` 类型签名

### 核心设计

新建 `orchestrator/middleware/` 包（orchestrator module 内的子包，非独立 module），定义统一 Handler/Middleware 类型。Input/Output 使用轻量自定义 Message 类型（避免引入 session 依赖）。Adapter 函数放在 `agent/middleware.go` 中（避免 middleware → agent 循环依赖）。

### 步骤

**3.1** 创建 `orchestrator/middleware/middleware.go`（新文件）
```go
package middleware

type Handler func(ctx context.Context, input *Input) (*Output, error)
type Middleware func(next Handler) Handler

type Message struct {
    Role    string
    Content string
}

type Input struct {
    Messages  []Message
    SessionID string
    Metadata  map[string]any
}

type Output struct {
    Messages []Message
    Metadata map[string]any
}

func Chain(mws ...Middleware) Middleware { ... }
```

**3.2** 修改 `orchestrator/agent/middleware.go`
- 在 `AgentHandler` 和 `Middleware` 类型定义上方添加 `// Deprecated:` 注释
- 添加 adapter 函数：
  - `func adaptMiddleware(mw middleware.Middleware) Middleware` — 新→旧桥接
  - `func WithUnifiedMiddleware(mw ...middleware.Middleware) RunOption` — 新 RunOption
- 在 `runConfig` 添加 `unifiedMiddlewares []middleware.Middleware` 字段
- 在 `agent.go` Run() 中合并新旧 middleware（新的通过 adapter 转为旧格式）

**3.3** 添加 import
- `orchestrator/agent/middleware.go` 添加 `"github.com/inferglow/orchestrator/middleware"` import

**3.4** 创建 `orchestrator/middleware/middleware_test.go`（新文件）
- 测试 `Chain` 函数组合行为
- 测试 adapter 双向转换正确性

**3.5** 验证：`cd orchestrator && go build ./... && go test ./...`

### 关键文件
- `orchestrator/middleware/middleware.go`（新建）
- `orchestrator/middleware/middleware_test.go`（新建）
- `orchestrator/agent/middleware.go`
- `orchestrator/agent/agent.go`

---

## 依赖关系

```
S1 (otel 解耦) ─── 独立，无前置依赖
S2 (contextmgr 拆出) ─── 独立，无前置依赖
S3 (middleware 统一) ─── 独立，无前置依赖
```

三者之间无直接依赖，可独立实施。建议顺序 S1→S2→S3（按风险从低到高）。

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| S1: `*otel.Tracer` 不满足 `SpanStarter` | 在 otel/tracer.go 增加 `Start()` 方法；步骤 1.11 编译验证 |
| S1: `callbacks_tracer.go` SpanKind 常量迁移遗漏 | grep 全量扫描 `otel.Span` 确保零遗漏 |
| S2: 包名 `context` 与 stdlib 冲突 | 包名保持 `contextmgr`，module path 用 `github.com/inferglow/context` |
| S2: import path 替换遗漏 | grep 全量扫描 `session/contextmgr` 确认 |
| S3: middleware 与 agent 循环依赖 | middleware 包不 import agent；adapter 放在 agent 侧 |
| S3: 新旧 middleware 共存认知负担 | `// Deprecated` 注释 + 文档引导 |

---

## 被拒绝的方案

1. **SpanStarter 保留 SpanKind 参数**：需要 agent 定义 SpanKind 类型且 otel 也引用它，否则需要两个类型系统。用标准 `trace.Tracer.Start` 签名更简洁，语义名通过 `semanticSpanName()` 在调用侧映射。
2. **context/ 包名用 `context`**：与 Go stdlib `context` 包名冲突，导致 import 混淆。保持 `contextmgr` 包名。
3. **middleware 包放在独立 module**：增加 module 管理成本。放在 orchestrator module 内即可，未来需要时再拆出。
4. **adapter 放在 middleware 包内**：会导致 middleware → agent 循环 import。adapter 放在 agent 侧解决。
