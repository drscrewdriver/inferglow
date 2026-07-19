# Agent Loop 三模式抢占改善 Spec

> 目标：让 Agent 在执行中支持三种用户输入介入模式
> 策略：最大化复用已有 TurnLoop + CancelManager，纯增量变更
> 预计变更：~300 行新增，4 文件修改，2 新文件

---

## 背景与现状

InferGlow 已具备完整的抢占基础设施：

| 组件 | 文件 | 现状 |
|------|------|------|
| **TurnLoop 状态机** | `orchestrator/agent/internal/turnloop/turnloop.go` | idle→planning→active 三态 + channel preempt |
| **CancelManager** | `orchestrator/agent/internal/cancel/cancel.go` | `CancelMode` 位运算组合（`CancelImmediate=0` / `CancelAfterChatModel` / `CancelAfterToolCalls`）+ 超时升级 |
| **executeLoop** | `orchestrator/agent/engine.go` (L279-810) | 7 个 safe-point（Point 1-7）+ streamLoop preemptCh select |
| **Agent.Run** | `orchestrator/agent/agent.go` | 同步阻塞，无输入队列 |

**缺失**：无输入队列 / 无模式选择 API / 工具执行不可抢占 / 无状态快照

<!-- REVIEW: 文件路径已从 `internal/` 修正为 `orchestrator/agent/internal/`。agent.go 行数去掉（473L 是旧版本）。 -->

---

## 三模式定义

| 模式 | 语义 | 类比 | 映射到已有 CancelMode |
|------|------|------|----------------------|
| | **Queue（排队）** | 等当前 turn 完成，下一轮处理新输入 | Qoder | 不 cancel，入 InputQueue |
<!-- REVIEW: "当前 turn 完成"的边界需要精确化。executeLoop 中 Point 7 (EnterIdle, L780-782) 在每轮底部。Queue 应在 EnterIdle 后消费（此时 agent 空闲且 session 状态对外一致），而非等 round 递增。decision.NextAction=="execute" 时不递增 round——一轮可能包含多次 tool-call 子循环。 -->
| **SafePoint（安全点抢占）** | 在 planning 阶段边界中断，保留状态 | Eino TurnLoop preempt | `CancelAfterChatModel \| CancelAfterToolCalls` |
| **Force（强制中断）** | 立即终止当前执行，丢弃并开始新轮 | Trae 停止 | `CancelImmediate` |

---

## Phase 1: PreemptMode API（基础层）

### 1.1 新增 PreemptMode 类型
- **文件**: `orchestrator/agent/internal/cancel/cancel.go`
- 在 CancelMode 常量后（~L51）新增：
```go
type PreemptMode int
const (
    PreemptQueue     PreemptMode = iota // 排队等待 turn 边界
    PreemptSafePoint                     // 下一 safe-point 中断
    PreemptForce                         // 立即中断
)
```

### 1.2 cancelRequest 增加 preemptMode 字段
- **文件**: `orchestrator/agent/internal/cancel/cancel.go`
- cancelRequest 结构体（~L105）新增 `preemptMode PreemptMode`

### 1.3 新增 CancelWithMode 方法
- **文件**: `orchestrator/agent/internal/cancel/cancel.go`
- CancelManager 新增方法，映射 PreemptMode → CancelMode：
```go
func (m *CancelManager) CancelWithMode(mode PreemptMode, reason string) {
    switch mode {
    case PreemptQueue:
        return // 不 cancel，由 InputQueue 处理
    case PreemptSafePoint:
        m.Cancel(CancelAfterChatModel|CancelAfterToolCalls, WithReason(reason))
    case PreemptForce:
        m.Cancel(CancelImmediate, WithReason(reason))
    }
}
```

### 1.4 类型重导出
- **文件**: `orchestrator/agent/cancel.go`
- 新增 PreemptMode 类型别名 + 常量重导出（遵循已有模式）

### 1.5 单元测试
- **文件**: `orchestrator/agent/internal/cancel/cancel_test.go`
- 覆盖三种模式的 cancel 行为

---

## Phase 2: InputQueue（排队模式核心）

### 2.1 创建 InputQueue
- **文件**: `orchestrator/agent/input_queue.go`（新建，~80 行）
```go
type InputRequest struct {
    Message    string
    Mode       PreemptMode
    ResponseCh chan<- InputResponse
    Ctx        context.Context
}
type InputResponse struct {
    Response string
    Error    error
}
type InputQueue struct {
    mu       sync.Mutex
    pending  []InputRequest
    capacity int              // 默认 8
}
// Enqueue / Dequeue / Peek / Len
```
- 有界 FIFO，满时返回 `ErrQueueFull`（server 翻译为 HTTP 429）
- 纯缓冲，不处理消息
<!-- REVIEW: ResponseCh 是同步阻塞的——HTTP handler 调用 SubmitInput 后阻塞等待 agent 完成。busy 时调用方需等当前 turn + 队列中所有前置请求。建议：(1) SubmitInput 应接受 context 用于超时控制；(2) 文档明确这是同步 API，异步场景用 WebSocket/SSE。 -->

### 2.2 Agent 集成
- **文件**: `orchestrator/agent/agent.go`
- Agent 结构体新增 `inputQueue *InputQueue` 字段
- 新增 `Agent.SubmitInput(ctx, msg, mode) (<-chan InputResponse, error)`：
  - 若 TurnPhase==Idle → 直接调 Run
  - 否则 → 入队，根据 mode 决定是否触发 cancel

### 2.3 队列消费
- **文件**: `orchestrator/agent/engine.go`（非 agent.go——Point 7 在 executeLoop 中，L778-782）
- executeLoop 的 Point 7（EnterIdle 后）新增队列检查：
```go
// drain input queue at turn boundary
if e.inputQueue != nil {
    if req, ok := e.inputQueue.Dequeue(); ok {
        e.session.AddUserMessage(req.Message)
        continue // 继续 for 循环，开始新 turn
    }
}
```

### 2.4 单元测试
- **文件**: `orchestrator/agent/input_queue_test.go`（新建）
- 覆盖并发 Enqueue/Dequeue、有界溢出、turn 边界消费
<!-- REVIEW: 缺少关键场景：(1) Enqueue 时 ctx 已取消——应返回 ctx.Err() 而非入队；(2) Dequeue 时请求的 ctx 已取消——consumer 应跳过该请求；(3) PreemptForce 入队后 consumer 退出 executeLoop 重新 dispatch 的行为验证。 -->

---

## Phase 3: 状态快照（抢占恢复基础）

### 3.1 TurnState 结构
- **文件**: `orchestrator/agent/internal/turnloop/turnloop.go`
- 新增（~25 行）：
```go
type TurnState struct {
    Phase          TurnPhase
    Round          int
    ToolCallRounds int
    MessageCount   int
    PreemptReason  string
    Timestamp      time.Time
}
func (l *TurnLoop) Snapshot() TurnState // 只读，线程安全
```

### 3.2 Engine 快照存储
- **文件**: `orchestrator/agent/engine.go`
- Engine 结构体新增 `lastPreemptState *TurnState`
- 在 preempt 退出路径（streamLoop preempt case ~L513, CancelImmediate ~L361）捕获快照
- 新增 `Engine.LastPreemptState() *TurnState` 访问器

### 3.3 浅拷贝策略
- 快照只记录元数据（int/string/time），不深拷贝 session 历史
- 内存开销 ~200 bytes/快照，仅保留最后一次
<!-- REVIEW: 快照的消费者未定义。如果仅用于外部查询"Agent 在什么状态被中断"——够用。若目标是 **resume**（从中断点恢复执行），光有 TurnState 不够——需要序列化 session 到最后一致状态。建议 Phase 3 仅定义快照结构但不承诺 resume 能力。 -->

---

## Phase 4: Server API 集成

### 4.1 扩展 ChatRequest
- **文件**: `server/handlers.go`
```go
type ChatRequest struct {
    Message     string `json:"message"`
    PreemptMode string `json:"preempt_mode,omitempty"` // "queue"|"safe_point"|"force"
}
```
- 缺省空字符串（不传 preempt_mode）走原路径——直接同步 Run，不经过 InputQueue
<!-- REVIEW: spec 原写"缺省 queue（向后兼容）"。但 Agent.Run 当前是同步阻塞，引入 InputQueue 后 queue 模式会通过 ResponseCh 阻塞等待，改变了调用语义。建议缺省 **空字符串** 走原同步路径（零影响）。只有显式传 "queue" 才启用排队。 -->

### 4.2 新增异步输入端点
- **文件**: `server/handlers.go`（~40 行新增）
```
POST /v1/agents/{id}/input
```
- 解析 PreemptMode
- Agent 空闲 → 直接 Run
- Agent 忙碌 → 根据 mode：
  - queue → InputQueue.Enqueue
  - safe_point → CancelManager.CancelWithMode(PreemptSafePoint) + 入队
  - force → CancelManager.CancelWithMode(PreemptForce) + 入队
- 队列满 → HTTP 429 + Retry-After header
<!-- REVIEW: 需要补充完整错误码映射。除 429 外还应定义：(1) ctx 取消 → HTTP 499；(2) agent 不存在 → HTTP 404；(3) agent 空闲直接 Run → HTTP 200；(4) PreemptForce 入队 → HTTP 202 Accepted（结果通过 ResponseCh 异步等待）。 -->

### 4.3 路由注册
- **文件**: `server/router.go`
- 新增 `POST /v1/agents/{id}/input` 路由

---

## Phase 5: 工具执行可中断（可选增强）

### 5.1 ExecuteInterruptible
- **文件**: `orchestrator/actionruntime/dispatcher.go`（~35 行新增）
```go
func (d *ActionDispatcher) ExecuteInterruptible(
    ctx context.Context,
    calls []ActionCall,
    preemptCh <-chan struct{},
) ([]*ActionResult, bool)
```
- 在 wg.Wait() 基础上增加 preemptCh select
- preempt 触发 → 取消 context → 收集已完成结果 → 返回 `(results, true)`
- 现有 Execute 不变，Interruptible 是 opt-in

### 5.2 executeLoop 接入
- **文件**: `orchestrator/agent/engine.go`（~L680）
- 有 preemptCh 时调 ExecuteInterruptible，否则调 Execute
- preempt 返回后保存快照并退出
<!-- REVIEW: "有 preemptCh 时调 ExecuteInterruptible"——但 preemptCh 在 Point 2 (L368-371) 获取。只要 TurnLoop 存在就非 nil。条件始终为 true，Interruptible 变成强制的而非 opt-in。建议：新增 ExecuteMode 枚举 {Blocking, Interruptible}，默认 Blocking（向后兼容），仅 PreemptForce 用 Interruptible。 -->

---

## 依赖关系

```
Phase 1 (PreemptMode API) ─── 基础，最先实施
  ├── Phase 2 (InputQueue) ─── 依赖 Phase 1 的 PreemptMode
  │     └── Phase 4 (Server API) ─── 依赖 Phase 1 + 2
  ├── Phase 3 (状态快照) ─── 依赖 Phase 1
  └── Phase 5 (工具中断) ─── 独立，可并行或最后实施
```

实施顺序: **1 → 2 → 3 → 4 → 5**

---

## 风险与缓解

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| InputQueue 并发竞争 | 高 | Enqueue（外部调用）用 mutex；Dequeue（内部 Point 7）用 TurnLoop phase 守卫消费时机——两层不同的锁机制 |
<!-- REVIEW: 原表述"TurnLoop phase 作为 gate"过于模糊。TurnLoop 是 agent 内部组件，外部调用方无法可靠读 phase。实际 gate 分两层：(1) Enqueue 侧用 mutex 保护 pending slice；(2) Dequeue 在 Point 7 (EnterIdle 后) 执行——此时 phase 刚从 idle→planning，是安全消费点。 -->
| 工具不响应 context 取消 | 中 | 文档要求工具遵守 ctx.Done()；Phase 5 增加 grace period |
| goroutine 泄漏 | 中 | InputQueue drain 用 idle timeout 自动退出；Agent 关闭时 force cancel |
| 向后兼容破坏 | 低 | 所有变更纯增量；PreemptMode 缺省 queue（等同当前行为） |
| 快照内存膨胀 | 低 | 浅拷贝 ~200 bytes，仅保留最后一次 |

---

## 关键文件清单

1. `orchestrator/agent/internal/cancel/cancel.go` — PreemptMode 类型 + CancelWithMode
2. `orchestrator/agent/agent.go` — InputQueue 集成 + SubmitInput API
3. `orchestrator/agent/engine.go` — 队列消费 + 快照捕获
4. `orchestrator/agent/input_queue.go` — **新建**：InputQueue 实现
5. `server/handlers.go` — API 集成 + /input 端点

---

## 被否决的方案

| 方案 | 否决原因 |
|------|----------|
| PreemptMode 放在 turnloop 包 | 语义上 PreemptMode 是 cancel 行为的变体，放在 cancel 包更自然 |
| channel generation 计数器优化 | 性能收益微小（<20 channels/run），可延迟到后续版本 |
| PreemptibleDispatcher 替代 Execute | 改动面太大；ExecuteInterruptible 作为 opt-in 更安全 |
| 深拷贝 session 历史到快照 | 内存开销与 session 长度成正比，浅拷贝足够支持 resume |
| 新增独立 Agent 方法（RunQueue/RunPreempt/RunForce） | 三个方法语义重叠；SubmitInput(mode) 更简洁 |