# V7 能力补齐实施计划 — 五项对标能力

> 目标：横评覆盖率 72% → ~82%，补齐 6 项缺失对标能力中的 5 项
> 实施顺序：触发器 → LCEL → 持久化Memory → 状态检查 → 流式工具

---

## Phase 1: 外部触发器（Webhook/Cron/EventBus）

### 1.1 修复接口适配
- **文件**: `server/run_manager.go`
- 为 `RunHandle` 添加 `GetID() string` 和 `GetStatus() string` 方法
- `*RunHandle` 自动满足 `trigger.RunHandle` 接口
- `*RunManager` 自动满足 `trigger.RunStarter` 接口（Start 签名兼容）

### 1.2 修复 Webhook body 双读 bug
- **文件**: `server/trigger/webhook.go`
- `ServeHTTP` 中 body 只读一次，结果同时传给签名验证和 JSON 解析
- 当前代码 `verifySignature()` 再次 `io.ReadAll(r.Body)` 会得到空数据

### 1.3 Server 集成 trigger.Registry
- **文件**: `server/server.go`
- 添加 `triggerReg *trigger.Registry` 字段
- `NewServer`/`NewServerWithFlows` 中初始化 `trigger.NewRegistry(rm)`
- `Server.Start()` 调 `triggerReg.StartAll(ctx)`
- `Server.Shutdown()` 调 `triggerReg.StopAll()`

### 1.4 触发器 REST API
- **文件**: `server/router.go` — 新增路由
- **文件**: `server/handlers_trigger.go` — 新建 handler
```
POST   /v1/triggers          — 创建触发器
GET    /v1/triggers          — 列出所有
GET    /v1/triggers/{id}     — 获取详情
DELETE /v1/triggers/{id}     — 删除
POST   /v1/triggers/{id}/start — 启动
POST   /v1/triggers/{id}/stop  — 停止
POST   /v1/webhooks/{id}     — Webhook HTTP 入口
```

### 1.5 编译验证
- `cd server && go build ./...`
- 编写 `server/trigger/trigger_test.go` 单元测试

---

## Phase 2: LCEL 声明式链

### 2.1 创建 flow/lcel.go
- **文件**: `flow/lcel.go`（新建，~120 行）
```go
type Chain struct { steps []chainStep }
type chainStep struct { name string; fn StepFunc }
func LCEL(name string, fn StepFunc) *Chain
func (c *Chain) Pipe(name string, fn StepFunc) *Chain
func (c *Chain) Invoke(ctx context.Context, input any) (any, error)
func (c *Chain) Build() *Flow  // 内部用 FlowBuilder
```
- 定位：比 FlowBuilder 简单（纯线性管道），比 TriggerFlow 轻量（无泛型/信号）

### 2.2 组合器
- `Map(fn)` — 列表逐元素
- `Branch(cond, trueChain, falseChain)` — 条件分派
- `Parallel(chains...)` — 并行执行

### 2.3 单元测试
- **文件**: `flow/lcel_test.go`（新建）
- 覆盖 Pipe/Invoke/Build/Map/Branch/Parallel

---

## Phase 3: 持久化 Memory

### 3.1 Server 注入 StepStoreLike
- **文件**: `server/server.go`
- 添加 `memStore` 字段 + `SetMemoryStore()` setter
- 不修改构造器签名（向后兼容）

### 3.2 实现 Memory CRUD handler
- **文件**: `server/handlers.go`
- 替换 `handleCreateMemory`（当前是空桩）→ 调 `memStore.UpsertLongMem()`
- 替换 `handleSearchMemory`（当前是空桩）→ 调 `memStore.SearchLongMem()`
- 新增 `handleGetMemory` → `memStore.GetLongMem()`
- 新增 `handleDeleteMemory` → `memStore.RemoveLongMem()`

### 3.3 新增路由
- **文件**: `server/router.go`
```
GET    /v1/memories/{id}     — 获取单条记忆
DELETE /v1/memories/{id}     — 删除记忆
```
（POST/GET 路由已存在，只需实现 handler）

### 3.4 Session 结束自动提升
- **文件**: `server/run_manager.go`
- 在 `execute()` 成功后调 `LongMemPromoter.OnSessionEnd()`
- 需要 FlowContextFactory 提供 context manager 引用

---

## Phase 4: 运行时状态检查（只读）

### 4.1 RunHandle 持有执行状态
- **文件**: `server/run_manager.go`
- 添加 `execState atomic.Value` 字段（存 `*flow.ExecutionState` 快照）
- `execute()` 中每个 step 完成后更新快照

### 4.2 状态查询 API
- **文件**: `server/handlers_flow.go`
- 新增 `handleGetRunState` — 返回执行状态 JSON（step log、errors、当前 step）
- **文件**: `server/router.go`
```
GET /v1/runs/{id}/state  — 执行状态快照
GET /v1/runs/{id}/steps  — 逐步执行日志
```

### 4.3 暂不实现写入
- `PUT /v1/runs/{id}/state` 标记为 future work
- 只读方案零风险，不影响执行路径

---

## Phase 5: 流式工具调用

### 5.1 桥接 AgentCallbacks → SSE
- **文件**: `server/handlers_stream.go`（新建）
- 创建 `StreamingToolWrapper`：包装 `AgentCallbacks`，在 `OnToolCallStart/End` 中写 SSE event
- 不修改 `orchestrator/agent/` 核心代码

### 5.2 新路由
- **文件**: `server/router.go`
```
POST /v1/agents/{id}/stream-run  — 带工具反馈的流式执行
```

### 5.3 RunManager step 级事件
- **文件**: `server/run_manager.go`
- `execute()` 中每个 step 完成时 emit `RunEvent{Type: "step_done", Step: name}`
- 客户端通过已有 `GET /v1/runs/{id}/events` SSE 端点实时接收

---

## 依赖关系

```
Phase 1 (触发器) ─── 独立，最先实施
Phase 2 (LCEL)    ─── 独立，可与 Phase 1 并行
Phase 3 (Memory)  ─── 与 Phase 1 共享 server.go 修改，顺序实施
Phase 4 (状态)    ─── 依赖 Phase 1 的 RunHandle 改动
Phase 5 (流式)    ─── 依赖 Phase 4 的 step 级事件
```

实施顺序: **1 → 2 → 3 → 4 → 5**

---

## 风险与缓解

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| Webhook body 双读导致 HMAC 验签 100% 失败 | 高 | Phase 1.2 优先修复 |
| RunHandle 方法添加破坏现有调用 | 低 | 纯增量，不改签名 |
| 持久化 Memory 写操作阻塞热路径 | 中 | 仅在 session 结束时提升，非每步 |
| 状态检查的并发安全 | 中 | 用 atomic.Value 存快照，读端无锁 |
| LCEL Chain.Build() 语义丢失 | 低 | 文档说明限制：Chain 是线性管道，不支持分支 |

---

## 关键文件清单

1. `server/run_manager.go` — 接口适配 + 状态收集 + step 事件
2. `server/server.go` — trigger Registry + memStore 字段
3. `server/router.go` — 所有新路由
4. `flow/lcel.go` — LCEL 链（唯一全新文件）
5. `server/trigger/webhook.go` — body 双读 bug 修复

---

## 被否决的方案

| 方案 | 否决原因 |
|------|----------|
| Cron 共享 timer wheel | 当前规模（<10 trigger）不需要，每个 trigger 独立 goroutine 足够 |
| EventBus 分片锁 | subscriber 数量少时 RWMutex 足够，过度优化 |
| 状态修改（PUT state） | 首版只读足够，写入增加并发风险 |
| Chain 惰性编译 | 增加复杂度，Chain 编译本身是 O(n) 无性能瓶颈 |
