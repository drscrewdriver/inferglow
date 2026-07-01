# 清理计划：Middleware 遗留桥接

> **状态（2026-07-30）**：✅ 已完成。废弃类型已删除，`WithUnifiedMiddleware` 已重命名为 `WithMiddleware`，桥接代码已消除，全部测试通过。

> 目标：清除 `orchestrator/agent/middleware.go` 中的两套类型系统并发现象，
> 统一为 `orchestrator/middleware/` 的单一路径。
>
> 背景：详见 `docs/system-analysis/09-middleware-and-orchestration-history.md`

---

## 现状

`orchestrator/agent/` 中维护了两套 middleware 类型系统，通过 adaptor 桥接：

| 类型 | 状态 | 所在文件 |
|------|------|----------|
| `agent.AgentHandler` | Deprecated | `agent/middleware.go` |
| `agent.Middleware` | Deprecated | `agent/middleware.go` |
| `agent.WithMiddleware()` | Deprecated | `agent/middleware.go` |
| `chainMiddleware()` | 内部实现 | `agent/middleware.go` |
| `adaptUnifiedToLegacy()` | 桥接 | `agent/middleware.go` |
| `agent.WithUnifiedMiddleware()` | 当前推荐 | `agent/middleware.go` |
| `middleware.Handler` / `middleware.Middleware` | 统一类型 | `orchestrator/middleware/` |

**外部消费检查**（已确认零外部依赖）：

- `agent.AgentHandler`：仅在注释中引用（`orchestrator/middleware/middleware.go:30`）
- `agent.Middleware` / `agent.WithMiddleware()`：仅测试文件使用
- `adaptUnifiedToLegacy` / `chainMiddleware`：仅 `agent/agent.go` 内部使用

---

## 清理步骤

### Step 1：修改 `agent/middleware.go` — 删除旧类型，重命名新入口

**操作**：

1. 删除以下声明：
   - `AgentHandler` 类型
   - `Middleware` 类型（旧的 `func(next AgentHandler) AgentHandler`）
   - `WithMiddleware` 函数（旧的，接收 `...Middleware` 的那个）
   - `chainMiddleware` 函数
   - `adaptUnifiedToLegacy` 函数

2. 将 `WithUnifiedMiddleware` 重命名为 `WithMiddleware`：
   - 原：`func WithUnifiedMiddleware(mw ...middleware.Middleware) RunOption`
   - 新：`func WithMiddleware(mw ...middleware.Middleware) RunOption`

3. 更新注释——去掉 Deprecated 标记，更新 doc 指向统一类型

**结果**：`agent/middleware.go` 从 ~100 行精简为 ~25 行（仅 `WithMiddleware` + 注释）。

### Step 2：修改 `agent/agent.go` — 消除桥接调用

**操作**：

在 `Agent.Run` 中，当前：

```go
var handler AgentHandler = coreHandler
allMiddlewares := make([]Middleware, 0, len(c.unifiedMiddlewares)+len(c.middlewares))
for _, umw := range c.unifiedMiddlewares {
    allMiddlewares = append(allMiddlewares, adaptUnifiedToLegacy(umw))
}
allMiddlewares = append(allMiddlewares, c.middlewares...)
if len(allMiddlewares) > 0 {
    handler = chainMiddleware(allMiddlewares)(coreHandler)
}
return handler(ctx, userMessage)
```

改为直接使用 `middleware.Middleware` + `middleware.Chain`：

```go
if len(c.middlewares) > 0 {
    coreHandler := func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
        resp, err := originalCore(ctx, extractMessage(input))
        if err != nil {
            return nil, err
        }
        return &middleware.Output{
            Messages: []middleware.Message{{Role: "assistant", Content: resp}},
        }, nil
    }
    handler := middleware.Chain(c.middlewares...)(coreHandler)
    out, err := handler(ctx, &middleware.Input{
        Messages: []middleware.Message{{Role: "user", Content: userMessage}},
    })
    if err != nil {
        return "", err
    }
    return lastMessageContent(out), nil
}
return originalCore(ctx, userMessage)
```

> **注**：上述方案需要将 `coreHandler` 的逻辑提取为 `originalCore`，并在无 middleware 时保持零开销直通。

### Step 3：清理 RunConfig 中的冗余字段

**操作**：

- 删除 `runConfig.middlewares []Middleware`（旧类型）
- 将 `runConfig.unifiedMiddlewares` 重命名为 `runConfig.middlewares`
- 删除 `Agent` 结构体中对应的冗余字段（同理）

### Step 4：重写 `agent/middleware_test.go` — 测试适配新签名

**操作**：

所有测试 middleware 改为 `middleware.Middleware` 签名：

```go
// 旧
func loggingMiddleware(log *[]string) Middleware { ... }

// 新
func loggingMiddleware(log *[]string) middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
            log = append(log, "before:"+input.Messages[0].Content)
            out, err := next(ctx, input)
            log = append(log, "after:"+out.Messages[0].Content)
            return out, err
        }
    }
}
```

测试用例不变（语义等价），签名改为 `WithMiddleware(...)`。

### Step 5：删除 `orchestrator/middleware/middleware.go` 中的兼容注释

**操作**：

删除第 30 行注释：
```
// S3: This package coexists with the legacy agent.AgentHandler / agent.Middleware
// types. Adapters in agent/middleware.go bridge between the two signatures.
```

### Step 6：验证

```bash
cd orchestrator/agent && go test ./...        # 全部通过
cd orchestrator/team && go test ./...          # team 不受影响（已用统一类型）
go build ./...                                 # 编译无错误
```

---

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `orchestrator/agent/middleware.go` | **重写** | 删除旧类型（~75 行），保留 `WithMiddleware` 统一入口 |
| `orchestrator/agent/agent.go` | **编辑** | 消除桥接调用，改用 `middleware.Chain` |
| `orchestrator/agent/middleware_test.go` | **重写** | 测试适配新 `middleware.Middleware` 签名 |
| `orchestrator/middleware/middleware.go` | **编辑** | 删除兼容注释的引用行 |

**总变更量估算**：~80 行删除 + ~60 行新增 — 净减 ~20 行，消除桥接复杂性的收益远大于行数变化。

---

## 风险与回滚

| 风险 | 缓解 |
|------|------|
| `coreHandler` 提取后行为偏差 | 无 middleware 时走零开销直通路径，行为与现有 `NoMiddlewareZeroOverhead` 测试完全一致 |
| `middleware.Input/Output` 与原始 `(string, string)` 之间的消息提取错误 | `Input` 只有 `Messages` 字段，提取逻辑简单（取首个/末个），测试覆盖链式场景 |
| team coordinator 不受影响 | team 已直接使用 `middleware.Middleware`，不需要任何修改 |

回滚策略：一个 commit 完成所有变更，`git revert` 即可恢复。

---

## 未来扩展：清理后 middleware 系统的 preempt 感知

当前 `middleware.Middleware` 对底层 `CancelManager` 和 `PreemptMode` 无感知。清理完成后，可以通过 `middleware.Input.Metadata` 自然接入：

### 建议的接入方式

```go
// middleware 中注入 preempt mode
func priorityMiddleware(next middleware.Handler) middleware.Handler {
    return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
        if prio, ok := input.Metadata["priority"]; ok && prio == "urgent" {
            input.Metadata["preempt_mode"] = "force"
        }
        return next(ctx, input)
    }
}
```

### 需要的底层支持（不在本计划范围内，仅作为依赖列示）

1. **context key 约定**：`engine.go` 需在 executeLoop 入口检查 `context.Value(PreemptModeKey)`，使 middleware 注入的 preempt mode 生效
2. **WithPreemptMode RunOption**：同步 `Run()` 路径也能选 mode，不依赖 `SubmitInput`
3. **engine 侧适配**：清理后的 `coreHandler` 改为接收 `*middleware.Input` 后，可直接从 `input.Metadata["preempt_mode"]` 获取模式

### 依赖关系

```
┌──────────────────────────────┐     ┌──────────────────────────────┐
│  本计划：middleware 桥接清理   │────▶│  后续：preempt context 接入  │
│  (无 middleware 层行为变化)    │     │  (middleware → engine preempt)│
└──────────────────────────────┘     └──────────────────────────────┘
                                             ↑
                                      ┌──────┴──────────┐
                                      │ preempt context  │
                                      │ key 约定 (底层)  │
                                      └─────────────────┘
```

> **注**：本计划不阻塞 preempt 接入——清理前后 middleware 的 `Input.Metadata` 接口不变。两步可以并行实施，但建议顺序执行以减少一次性改动量。
