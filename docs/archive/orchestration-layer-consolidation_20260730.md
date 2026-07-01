# 编排层架构归并计划

> **审定状态（2026-07-30）**：已确认 `StepFunc`/`StageFunc` 两套签名仍存在；`FlowContext` 现有 14 个方法；`executeFlow` 绕过 InputQueue 问题仍存在。**四个问题均有效，计划待执行。**
> **前置依赖 `cleanup-middleware-bridge.md` 已于 2026-07-30 完成。**
>
> **问题 1 已完成（2026-07-30）**：✅
> - 在 `stage` 包添加 `Adapt` 桥接函数，将 `StageFunc` 转为 `StepFunc`
> - 在 `StepFunc` 和 `StageFunc` 类型声明添加关系注释，明确推荐方向 A
> - `flowdef/adapter.go` 改用 `stage.Adapt` 简化代码
> - 新增 5 个桥接测试用例全部通过
> - `FlowContext` 接口膨胀问题已由 `flowcontext-interface-split.md` 阶段 1-2 解决

> 目标：消除 `flow/` 与 `orchestrator/` 之间执行单元接口的重复与不兼容，
> 为三模式抢占（PreemptQueue/SafePoint/Force）在上层落定铺平道路。
>
> 前置依赖：`cleanup-middleware-bridge.md`（middleware 桥接清理）需先完成，
> 因为编排层依赖 middleware 的 `Input/Output` 统一签名。

---

## 问题清单

| # | 问题 | 影响 | 建议时机 | 状态 |
|---|------|------|----------|------|
| 1 | `StepFunc` vs `StageFunc` 两套执行单元签名 | 开发者需二选一，不互操作 | 有消费需求时 | ✅ 已完成 |
| 2 | `FlowContext` 接口膨胀（12 方法） | 实现方负担重，mock 成本高 | 接口稳定后 | ✅ 阶段 1-2 完成 |
| 3 | `executeFlow` 绕过 InputQueue | Flow 模式下 queue 消费不可见 | Flow 成为主流路径前 | 待实施 |
| 4 | Middleware/Flow 嵌套角色未定义 | Flow 执行时 middleware 语义模糊 | 与 #1 同步 | 待实施 |

---

## 问题 1：StepFunc 与 StageFunc 两套签名

### 现状

```go
// flow/lcel.go — LCEL 链式编排
type StepFunc func(ctx context.Context, input any) (any, error)

// flow/stage/ — 注册中心
type StageFunc func(ctx context.Context, in Inputs, fctx FlowContext) (Outputs, error)
```

- `StepFunc` 是泛型 `any→any`，仅靠 `ctx` 传递全局状态
- `StageFunc` 是类型化的 `Inputs→Outputs`，有 `FlowContext` 参数可访问 `RunAgent`/`ExecuteAction`
- 两者不能互相调用——不能把 StageFunc 塞进 LCEL Chain，也不能用 LCEL 的 `Pipe` 串联 StageFunc
- 但 `FlowContext` 可通过 `FlowContextFrom(ctx)` 从 context 提取，所以 `StepFunc` 理论上也能做到 `StageFunc` 能做的所有事

### 路线图

**阶段一（共识）**：在注释和架构文档中标注两者的关系，明确推荐方向。

有两种可能的归并方向：

**方向 A：保留 StepFunc + LCEL Chain 为首要抽象，StageFunc 退化为注册别名**

```go
// StepFunc 从 ctx 中获取 FlowContext
type StepFunc func(ctx context.Context, input any) (any, error)

// StageFunc 是 StepFunc 的注册便捷包装
func StageFunc(name string, fn StageFunc) StepFunc {
    return func(ctx context.Context, input any) (any, error) {
        fctx, _ := FlowContextFrom(ctx)
        out, err := fn(ctx, input.(Inputs), fctx)
        return out, err
    }
}
```

**方向 B：统一到 StageFunc 签名，LCEL 链底层用 StageFunc 实现**

```go
// 唯一的执行单元签名
type StepFunc func(ctx context.Context, in Inputs, fctx FlowContext) (Outputs, error)
```

**推荐方向 A**——LCEL 的泛型 `any→any` 更灵活，`StageFunc` 是它的特化。不破坏现有 LCEL 链的代码。

**阶段二（实施）**：统一签名后，补 `StepFunc` 到 `StageFunc` 的桥接函数，同步更新测试。

---

## 问题 2：FlowContext 接口膨胀

### 现状

`FlowContext` 有 14 个方法：

| 类别 | 方法 | 说明 |
|------|------|------|
| 执行 | `RunAgent`, `RunAgentParallel`, `ExecuteAction`, `GenerateModel` | 核心 |
| 会话 | `SessionHistory`, `AppendSession` | 核心 |
| 审计 | `AuditAppend` | 横切 |
| 安全 | `MaskInput`, `CheckOutput` | 横切 |
| 可观测 | `StartSpan` | 横切 |
| KV 存储 | `SetValue`, `GetValue` | 横切 |
| 生命周期 | `RequestPause` | 横切 |

### 建议

横切关注点（审计、安全、可观测、KV 存储）应通过 context 值传递，而非接口方法：

```go
// 从 context 获取审计接口
func AuditFrom(ctx context.Context) (AuditHook, bool)

// 从 context 获取安全接口
func SecurityFrom(ctx context.Context) (SecurityHook, bool)
```

保留在接口中的核心方法（6 个）：

```go
type FlowContext interface {
    RunAgent(ctx, msg, sysPrompt, opts) (string, error)
    RunAgentParallel(ctx, agents) ([]string, error)
    ExecuteAction(ctx, name, params) (any, error)
    GenerateModel(ctx, system, user) (string, error)
    SessionHistory() []map[string]any
    AppendSession(role, content any)
}
```

### 实施依赖

- 需要定义 `context key` 和对应的 getter 函数
- 所有横切关注点需有 `noop` 默认实现（`context.WithValue` 未设置时行为可回退）
- 此变更与 `middleware.Input.Metadata` 模式互补——二者都通过 context 传递横切数据

---

## 问题 3：executeFlow 绕过 InputQueue

### 现状

```
Agent.Run
  └─ coreHandler
       ├─ if flow → executeFlow
       │              └─ FlowContext.RunAgent → 子 engine (独立 TurnLoop)
       │                  ← InputQueue 不可见
       └─ else → executeLoop → Point 7 消费 InputQueue
```

- `executeLoop` 的 Point 7 是 InputQueue 的唯一消费点
- 当 `executeFlow` 路径激活时，InputQueue 永远不会被 drain
- 通过 `SubmitInput(PreemptQueue)` 入队的消息在 flow 模式下**永远卡在队列中**

### 缓解方案

**短期**（架构文档标注）：在 `Agent` 和 `FlowContext` 的 doc comment 中标注此限制。

**长期**（Flow 成为主流路径时）：

选项 A：`executeFlow` 的每次 `RunAgent` 调用前也检查 InputQueue：
```go
// 在 flow_context_impl.go 的 RunAgent 入口处
if fc.engine.inputQueue != nil {
    if req, ok := fc.engine.inputQueue.Dequeue(); ok {
        fc.session.AddUserMessage(req.Message)
        // 用入队消息替代原始 userMessage 执行
        userMessage = req.Message
    }
}
```

选项 B：让 `FlowContext.AgentRunOptions` 增加 `InputQueue` 字段，由调用方决定是否启用队列消费。

选项 C：在 `executeFlow` 的 for 循环（每轮 step）之间也插入队列检查点。

---

## 问题 4：Middleware/Flow 嵌套未定义

### 现状

```
Agent.Run
  ├─ middleware 链（外）
  ├─ coreHandler
  │    ├─ executeFlow → RunAgent → 子 engine.executeLoop
  │    │    ← 子 engine 无 middleware
  │    └─ executeLoop
  │         ← 主 engine 有 middleware（已在外层 wrap 过）
```

当 flow 路径激活时：
1. middleware 链在 `Run()` 入口处 wrap 了 `coreHandler`
2. `coreHandler` 调用 `executeFlow`
3. `executeFlow` 内的 `RunAgent` 创建**子 engine**（独立 TurnLoop/CancelManager）
4. 子 engine 的 `executeLoop` **不在 middleware 链中**

这意味着：
- middleware 在 flow 模式下包裹的是 `executeFlow`，不是具体的 agent 循环
- `RunAgent` 内部的 agent 循环不受任何 middleware 约束
- logging/rate-limiting/auth middleware 只在外层生效一次

### 建议

当前问题不大——如果 middleware 的目的是"限制用户消息频率"（外层），那么不需要包裹内层 `RunAgent`。但如果 middleware 的目的是"审计每条工具调用"，则无法通过 flow 路径触及 `RunAgent` 内部。

**未来的统一方案**：为 `FlowContext.RunAgent` 也传入 middleware chain（从 `Agent.Run` 的 `runConfig` 继承）。但这需要 `AgentRunOptions` 扩展 `Middlewares` 字段。等出现具体审计缺口时再补。

---

## 实施顺序与依赖

```
cleanup-middleware-bridge.md        ← 先完成（无依赖）
  │
  ├── 问题 2：FlowContext 拆分      ← 依赖 middleware 清理完成后的 ctx 约定
  │
  ├── 问题 1：StepFunc vs StageFunc  ← 独立，可并行
  │
  ├── 问题 4：Middleware/Flow 嵌套  ← 依赖 问题 1 + middleware 清理
  │
  └── 问题 3：executeFlow 绕 InputQueue  ← 依赖 preempt spec 修复完成
       （确保 InputQueue 本身可用后再修 flow 模式的消费）
```

| 序号 | 任务 | 前置 | 工作量估 |
|------|------|------|----------|
| 1 | 归档 `StepFunc` 和 `StageFunc` 的关系，增加桥接函数 | 无 | ~30 行 + 文档 |
| 2 | 将 `FlowContext` 的横切方法移到 context 值 | middleware 清理 | ~80 行 |
| 3 | 标注 `executeFlow` 的 InputQueue 缺口 | preempt spec 修复 | ~10 行注释 |
| 4 | 为 `AgentRunOptions` 增加 `Middlewares` 字段（可选） | 问题 1 + 2 | ~20 行 |

---

## 不建议现在做的

- ❌ 合并 `flow/lcel.go` 和 `flow/stage/` 到同一个包——两个概念不同（LCEL 是链式调用，Stage 是注册中心），保持分离更清晰
- ❌ 删除 StepFunc——LCEL 的泛型 `any→any` 对简单场景有用，不销毁它
- ❌ 在 flow 模式中强制开启 InputQueue 消费——`executeFlow` 的 for 循环边界不明确，等有真实场景再补
