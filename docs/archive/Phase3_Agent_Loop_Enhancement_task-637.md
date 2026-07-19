# Phase 3: Agent Loop Enhancement

## 设计决策

- **Event Stream**: 在现有 `AgentCallbacks` 上新增 `OnToken`/`OnReasoning` 两个钩子（additive，零破坏），同时提供 `EventSink` 接口 + `CallbacksFromSink()` 适配器供 CLI/server 以单接口消费
- **Message Interleave**: 在 `executeFlow` 完成后添加 queue-drain 循环；修复 `executeLoop` L844 的 ResponseCh 死信 bug；给 InputQueue 添加 notification channel 避免轮询
- **Sub-Agent**: 扩展现有 `cloneEngineForParallel` + `AgentRunOptions`，添加 depth 字段和 `spawn_agent` builtin action
- **BM25**: 直接复用 `context/retrieval.BM25Index`，修复冒泡排序为 `sort.Slice`

---

## Part A: Event Stream（事件流）

### A1. 扩展 AgentCallbacks

**文件**: `orchestrator/agent/callbacks.go`

在 `AgentCallbacks` struct 添加两个字段：
```go
// OnToken is called for each text delta chunk from the LLM stream.
OnToken func(ctx context.Context, delta string)
// OnReasoning is called for each reasoning delta chunk (DeepSeek/MiMo).
OnReasoning func(ctx context.Context, delta string)
```

添加对应的 `fireOnToken` / `fireOnReasoning` helper（同现有 nil-safe 模式）。

### A2. 在 streamLoop 中发射 token 事件

**文件**: `orchestrator/agent/engine.go` (~L519-523)

在 `content.WriteString(chunk.Delta)` 后调用 `fireOnToken(e.callbacks, ctx, chunk.Delta)`；
在 `reasoning.WriteString(chunk.Reasoning)` 后调用 `fireOnReasoning(e.callbacks, ctx, chunk.Reasoning)`。

### A3. EventSink 接口 + 适配器

**文件**: `orchestrator/agent/event_sink.go`（新建，~90 行）

```go
type EventKind int
const (
    EventRunStart EventKind = iota
    EventRunEnd
    EventLLMStart
    EventLLMEnd
    EventToolStart
    EventToolEnd
    EventToken
    EventReasoning
    EventError
)

type AgentEvent struct {
    Kind     EventKind
    Text     string
    ToolName string
    Round    int
    Err      error
}

type EventSink interface { Emit(AgentEvent) }
type FuncEventSink func(AgentEvent)

// NewChannelSink creates an EventSink backed by a buffered channel.
func NewChannelSink(buf int) (EventSink, <-chan AgentEvent)

// CallbacksFromSink maps an EventSink to AgentCallbacks.
func CallbacksFromSink(sink EventSink) *AgentCallbacks
```

### A4. CLI 接入 EventSink

**文件**: `cli/repl.go`

在 `chatOnce` 中使用 `NewChannelSink(256)` + goroutine 实时打印 token delta（替代当前的同步等待最终响应）。

---

## Part B: Message Interleave（消息插队）

### B1. InputQueue 添加 notification channel

**文件**: `orchestrator/agent/input_queue.go`

```go
type InputQueue struct {
    mu       sync.Mutex
    pending  []InputRequest
    capacity int
    notify   chan struct{} // cap=1, non-blocking send on Enqueue
}
```

- `Enqueue`: 成功入队后 `select { case q.notify <- struct{}{}: default: }`
- 新增 `WaitCh() <-chan struct{}` 方法

### B2. 修复 executeLoop ResponseCh 死信

**文件**: `orchestrator/agent/engine.go` (~L834-847)

当前代码 `_ = req` 丢弃了 ResponseCh。修复为：
```go
if req, ok := e.inputQueue.Dequeue(); ok {
    e.session.AddUserMessage(req.Message)
    prefixSet = false
    halfwayWarned = false
    // 保存 req 以便本轮结束后发送响应
    e.pendingInterleave = &req
    continue
}
```

在 executeLoop 返回 finalResponse 前：
```go
if e.pendingInterleave != nil {
    e.pendingInterleave.ResponseCh <- InputResponse{Response: finalResponse}
    e.pendingInterleave = nil
}
```

### B3. executeFlow 添加 queue-drain

**文件**: `orchestrator/agent/flow_exec.go`

在步骤 10（AddAssistantMessage）后、return 前添加：
```go
// Drain input queue: process queued user messages by re-executing the flow.
const maxInterleaveTurns = 4
for i := 0; i < maxInterleaveTurns && e.inputQueue != nil; i++ {
    req, ok := e.inputQueue.Dequeue()
    if !ok {
        break
    }
    // 将排队消息作为新输入重新执行 flow
    e.session.AddUserMessage(req.Message)
    exec = f.Execute(ctx, req.Message)
    response = extractFlowResponse(exec.State.Result)
    e.session.AddAssistantMessage(response)
    if req.ResponseCh != nil {
        req.ResponseCh <- InputResponse{Response: response}
    }
}
```

---

## Part C: Sub-Agent Support（子 Agent）

### C1. Engine 添加 depth 字段

**文件**: `orchestrator/agent/engine.go`

```go
// depth tracks the nesting level for sub-agents. 0 = top-level.
depth int
```

在 `cloneEngineForParallel` 中：`depth: src.depth + 1`

### C2. 扩展 AgentRunOptions

**文件**: `flow/flow_context.go`

```go
type AgentRunOptions struct {
    MaxRounds        int
    SessionIsolation bool
    // ModelID 指定子 Agent 使用的模型。空字符串继承父 Agent。
    ModelID string
    // MaxDepth 最大嵌套深度。0 = 使用默认值 3。
    MaxDepth int
}
```

### C3. RunAgent 深度检查

**文件**: `orchestrator/agent/flow_context_impl.go`

在 `RunAgent` 入口：
```go
maxDepth := 3
if opts != nil && opts.MaxDepth > 0 {
    maxDepth = opts.MaxDepth
}
if fc.engine.depth >= maxDepth {
    return "", flow.ErrAgentDepthExceeded
}
```

在 `flow/` 包添加 sentinel error：`var ErrAgentDepthExceeded = errors.New("flow: agent depth exceeded")`

### C4. spawn_agent builtin action

**文件**: `builtins/actions/sub_agent.go`（新建，~120 行）

- Name: `"spawn_agent"`
- Schema: `{ "task": string (required), "system_prompt": string, "max_rounds": int }`
- Executor: 通过 `flow.FlowContextFrom(ctx)` 获取 FlowContext → 调用 `RunAgent(ctx, task, systemPrompt, opts)`
- 深度控制：当 `engine.depth >= maxDepth` 时不注册此工具（在注册时检查）

### C5. 注册到 agent_factory

**文件**: `cli/agent_factory.go`

```go
subAgent := actions.NewSubAgentAction(actions.SubAgentConfig{MaxDepth: 3, MaxRounds: 15})
actExt.Register(wrapWithIngest(subAgent, bridge))
```

---

## Part D: BM25 Search Upgrade

### D1. 修复 BM25Index 排序

**文件**: `context/retrieval/bm25.go` (L129-135)

替换冒泡排序为 `sort.Slice`：
```go
sort.Slice(results, func(i, j int) bool {
    return results[i].Score > results[j].Score
})
```

### D2. memory_recall.go 使用 BM25

**文件**: `builtins/actions/memory_recall.go`

替换 `search()` 方法中的子串匹配：
```go
func (e *recallExecutor) search(query, typeFilter, scopeFilter string, limit int) (*action.ActionResult, error) {
    all := e.store.List()
    // 过滤
    var filtered []memory.Memory
    for _, m := range all {
        if matchesFilter(m, typeFilter, scopeFilter) {
            filtered = append(filtered, m)
        }
    }
    if len(filtered) == 0 {
        return &action.ActionResult{OK: true, Status: "no_results", ...}, nil
    }
    // BM25 索引
    idx := retrieval.NewBM25Index()
    for i, m := range filtered {
        idx.Add(i, m.Name+" "+m.Title+" "+m.Description+" "+m.Body)
    }
    results, _ := idx.Search(context.Background(), query, limit)
    if len(results) == 0 {
        // 回退到子串匹配（BM25 对极短查询可能无结果）
        return e.substringSearch(query, filtered, limit)
    }
    // 格式化
    var hits []string
    for _, r := range results {
        hits = append(hits, formatHit(filtered[r.StepID]))
    }
    return &action.ActionResult{OK: true, Status: "ok", Result: strings.Join(hits, "\n")}, nil
}
```

### D3. builtins/go.mod 添加 context 依赖

**文件**: `builtins/go.mod`

添加 `github.com/inferglow/context` 依赖 + replace 指令。

---

## 依赖关系

```
A1 → A2 → A3 → A4  (事件流)
B1 → B2 + B3        (消息插队)
C1 → C2 → C3 → C4 → C5  (子Agent)
D1 → D2 → D3        (BM25)
```

四个 Part 相互独立，可并行实施。

---

## 文件变更清单

| 文件 | 操作 | 行数估 |
|------|------|--------|
| `orchestrator/agent/callbacks.go` | 编辑：+OnToken/OnReasoning | ~20 |
| `orchestrator/agent/engine.go` | 编辑：发射 token 事件 + pendingInterleave + depth 字段 | ~30 |
| `orchestrator/agent/event_sink.go` | 新建：EventSink + ChannelSink + 适配器 | ~90 |
| `orchestrator/agent/input_queue.go` | 编辑：notify channel + WaitCh | ~15 |
| `orchestrator/agent/flow_exec.go` | 编辑：queue-drain 循环 | ~20 |
| `orchestrator/agent/flow_context_impl.go` | 编辑：depth check + cloneEngine depth+1 | ~15 |
| `flow/flow_context.go` | 编辑：AgentRunOptions +ModelID/MaxDepth + ErrAgentDepthExceeded | ~10 |
| `builtins/actions/sub_agent.go` | 新建：spawn_agent 工具 | ~120 |
| `builtins/actions/memory_recall.go` | 编辑：BM25 替换子串匹配 | ~30 |
| `builtins/go.mod` | 编辑：+context 依赖 | ~3 |
| `context/retrieval/bm25.go` | 编辑：sort.Slice 替换冒泡 | ~5 |
| `cli/repl.go` | 编辑：EventSink 实时输出 | ~25 |
| `cli/agent_factory.go` | 编辑：注册 spawn_agent | ~5 |

**总计：~390 行新增 + ~100 行编辑**

---

## 验证计划

1. `go build ./...` — orchestrator / builtins / cli / flow / context 全模块编译
2. `go test ./orchestrator/...` — 现有 11 包不回归
3. `go test ./builtins/...` — 现有测试不回归
4. `go test ./context/...` — BM25 排序修复验证
5. `go test ./flow/...` — AgentRunOptions 扩展编译验证
6. 手动验证：CLI 中触发 spawn_agent 工具调用，确认深度限制生效

---

## 被拒绝的替代方案

1. **新建独立 `event` 包**（Plan A/B）：增加模块数量和 go.mod 依赖链。当前 `agent` 包内的 EventSink 接口足够，若未来 server/GUI 需要跨包消费再提取。
2. **sync.Pool + ring buffer**（Plan B）：记忆数 <200、token 事件频率 <100/s，GC 压力可忽略。过早优化。
3. **倒排索引重构 BM25**（Plan B）：当前 BM25 每次查询 O(D×T) 对 <200 条记忆延迟 <5ms，不值得引入增量维护复杂度。
4. **executeFlow 中途中断**（Plan B 的 select+Pause）：flow.Pause 语义是 daemon 重启恢复，用于消息插队会混淆语义。post-completion drain 更安全。
5. **信号量调度器**（Plan B）：当前子 Agent 只有 depth 限制需求，并发控制由 goroutine + WaitGroup 已覆盖（RunAgentParallel）。独立调度器留到多租户 server 场景。