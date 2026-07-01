# FlowContext 接口隔离拆分计划

> **审定状态（2026-07-30）**：已确认 `FlowContext` 现有 14 个方法，其中 7 个为横切关注点。
> 本计划与 `cleanup-middleware-bridge.md`（middleware 类型清理）和
> `orchestration-layer-consolidation.md`（StepFunc/StageFunc 归并）**正交**：
> 三者修改的文件集合不重叠，可并行实施。
>
> **阶段 1-2 已完成（2026-07-30）**：✅
> - 阶段 1：新增 `AuditHook`/`SecurityHook`/`SpanStarterHook`/`KVStore` 四个小接口 + 8 个 context 函数 + 4 个 noop 实现（~112 行）
> - 阶段 2：`flowContextImpl` 添加编译期接口检查，`executeFlow` 注入 4 个 hook 到 context
> - 新增测试文件 `flow/flow_context_hooks_test.go`（8 个测试用例全部通过）
> - 旧 `FlowContext` 接口保持不变，零破坏

---

## 问题

`flow/flow_context.go` 中的 `FlowContext` 接口混合了两类职责：

| 类别 | 方法 | 性质 |
|------|------|------|
| 执行能力（step 主动调用） | `ExecuteAction`, `GenerateModel`, `RunAgent`, `RunAgentParallel` | 核心 |
| 会话能力（step 主动调用） | `SessionHistory`, `AppendSession` | 核心 |
| 生命周期（step 主动调用） | `RequestPause` | 核心 |
| 审计（框架注入） | `AuditAppend` | 横切 |
| 安全（框架注入） | `MaskInput`, `CheckOutput` | 横切 |
| 可观测（框架注入） | `StartSpan` | 横切 |
| KV 存储（通用设施） | `SetValue`, `GetValue` | 横切 |

**代价**：

1. **违反 ISP**：只做字符串转换的 step 被迫依赖 `AuditAppend`、`MaskInput` 等永不调用的方法
2. **Mock 成本**：测试任何 step 需实现全部 14 个方法
3. **扩展即破坏**：新增横切关注点必须改接口 → 所有实现编译报错
4. **内在矛盾**：`StartSpan` 返回 `(context.Context, Span)` 已暗示横切应走 context 传递，但其余横切方法仍挂在接口上

**与 R-SFC 的关系**：`SessionHistory()` / `AppendSession()` 是 FlowContext 与 R-SFC 的唯一接触点——step 通过这两个方法读写会话，R-SFC 在底层决定返回内容的折叠程度。FlowContext 对 R-SFC 完全透明，拆分不影响 R-SFC。

---

## 目标接口

### 核心 FlowContext（7 方法，稳定不变）

```go
type FlowContext interface {
    // 执行
    ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error)
    GenerateModel(ctx context.Context, system string, userMessage string) (string, error)
    RunAgent(ctx context.Context, msg string, sys string, opts *AgentRunOptions) (string, error)
    RunAgentParallel(ctx context.Context, agents []AgentSubTask) ([]string, error)
    // 会话
    SessionHistory() []map[string]any
    AppendSession(role string, content any)
    // 控制流
    RequestPause(reason string) error
}
```

### 横切关注点小接口（通过 context 值注入）

```go
// 审计 —— 未注入时 no-op
type AuditHook interface {
    AuditAppend(source, action string, input, output any)
}
func WithAuditHook(ctx context.Context, h AuditHook) context.Context
func AuditHookFrom(ctx context.Context) AuditHook  // 未注入返回 noopAuditHook

// 安全 —— 未注入时直通
type SecurityHook interface {
    MaskInput(input string) string
    CheckOutput(output string) error
}
func WithSecurityHook(ctx context.Context, h SecurityHook) context.Context
func SecurityHookFrom(ctx context.Context) SecurityHook  // 未注入返回 noopSecurityHook

// 可观测 —— 未注入时返回 noop span
type SpanStarter interface {
    StartSpan(ctx context.Context, kind SpanKind, name string) (context.Context, Span)
}
func WithSpanStarter(ctx context.Context, s SpanStarter) context.Context
func SpanStarterFrom(ctx context.Context) SpanStarter  // 未注入返回 noopSpanStarter

// KV 存储 —— 未注入时 GetValue 返回 false
type KVStore interface {
    SetValue(key string, value any)
    GetValue(key string) (any, bool)
}
func WithKVStore(ctx context.Context, kv KVStore) context.Context
func KVStoreFrom(ctx context.Context) KVStore  // 未注入返回 noopKVStore
```

---

## 迁移路径（四阶段，向后兼容）

### 阶段 1：定义小接口 + context getter（零破坏）

**操作**：

1. 在 `flow/flow_context.go` 中新增 `AuditHook`、`SecurityHook`、`SpanStarter`、`KVStore` 四个小接口
2. 新增对应的 `With*` / `*From` context 函数（共 8 个函数）
3. 新增 4 个 noop 默认实现（`noopAuditHook`、`noopSecurityHook`、`noopSpanStarter`、`noopKVStore`）
4. **不修改** `FlowContext` 接口本身

**结果**：新代码可以使用 context getter，旧代码不受影响。

### 阶段 2：flowContextImpl 双写（零破坏）

**操作**：

1. `orchestrator/agent` 中的 `flowContextImpl`（实现 `FlowContext` 的具体类型）在构造时同时：
   - 实现旧大接口的所有方法（保持不变）
   - 将自身注入到 context 值中（`WithAuditHook(ctx, impl)` 等）
2. `executeFlow` 入口处把横切设施注入到 step 的 ctx 中

**结果**：step 既可以通过 `fctx.AuditAppend(...)` 调用（旧路径），也可以通过 `flow.AuditHookFrom(ctx).AuditAppend(...)` 调用（新路径）。

### 阶段 3：step 代码渐进迁移（逐步推进）

**操作**：

1. 内置 step（`flow/stage/` 中的 StageFunc）逐步改用 context getter
2. 文档和示例推荐新路径
3. 旧接口方法标注 `// Deprecated: Use flow.AuditHookFrom(ctx) instead.`

**结果**：新 step 不再依赖大接口，旧 step 仍可编译。

### 阶段 4：收缩 FlowContext 接口（breaking change，下个大版本）

**操作**：

1. 从 `FlowContext` 接口中删除 `AuditAppend`、`MaskInput`、`CheckOutput`、`StartSpan`、`SetValue`、`GetValue` 共 7 个方法
2. 删除 `flowContextImpl` 中对应的旧路径实现
3. 更新所有测试

**结果**：`FlowContext` 从 14 方法收缩为 7 方法。

---

## 文件变更清单

| 阶段 | 文件 | 操作 | 说明 |
|------|------|------|------|
| 1 | `flow/flow_context.go` | **编辑** | 新增 4 个小接口 + 8 个 context 函数 + 4 个 noop 实现（~120 行） |
| 1 | `flow/flow_context_hooks_test.go` | **新建** | 测试 context 注入/提取/noop 回退 |
| 2 | `orchestrator/agent/flow_context_impl.go` | **编辑** | 构造时注入 context 值（~15 行） |
| 3 | `flow/stage/*.go` | **编辑** | 内置 step 迁移到 context getter |
| 4 | `flow/flow_context.go` | **编辑** | 删除 7 个横切方法（~40 行删除） |
| 4 | `orchestrator/agent/flow_context_impl.go` | **编辑** | 删除旧路径实现 |

**阶段 1-2 总变更量**：~135 行新增，0 行删除——纯增量，零破坏。

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| context 值未注入时 step panic | 所有 `*From` 函数返回 noop 默认实现，永不返回 nil |
| 阶段 2 双写增加运行时开销 | context.WithValue 是 O(1) 指针操作，noop 调用零开销 |
| 阶段 4 breaking change 影响外部用户 | 在大版本（v8.0）发布，提前一个版本标注 Deprecated |
| 与 orchestration-layer-consolidation.md 问题 2 重叠 | 该文档问题 2 是方向性描述，本计划是具体实施；本计划完成后问题 2 自动关闭 |

---

## 不建议做的

- ❌ 一次性删除横切方法——破坏所有现有 step 实现
- ❌ 把 KV 存储升级为独立的 context 模块——过度工程，noop 回退足够
- ❌ 为每个横切关注点创建独立的 Go module——它们只有 1-2 个方法，放在 flow 包内即可
- ❌ 用 mask/no-op 替代拆分——mask 只解决"不需要某功能时不用填真实实现"，不解决接口膨胀、mock 成本和扩展破坏问题
