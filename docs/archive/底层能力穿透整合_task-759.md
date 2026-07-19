# InferGlow 底层能力穿透整合 Spec

> 核心问题：底层有 team/ Coordinator、VectorStore、SummaryMemory、TokenBuffer、longmem 等完整能力，但 CLI/Server/Desktop 层完全或部分无法访问。
> 策略：**Server 层统一暴露 + CLI 直接接线 + Desktop GUI 预留接口**

---

## 一、架构断层现状

```
Desktop GUI (Wails+React, 规划中)
  └─ Server REST API
       ├─ Agent CRUD + Run ✅    ├─ Flow/Trigger ✅
       ├─ Memory CRUD ⚠️ (独立InMemoryStore, 非底层)
       ├─ Team/Coordinator ❌    ├─ VectorSearch ❌
       └─ TurnLoop状态 ❌
  └─ CLI (TUI/REPL/OneShot)
       ├─ spawn_agent ✅ (tool call)
       ├─ memory_bridge ⚠️ (仅JSONL, 无向量持久化)
       ├─ team/ ❌              ├─ TurnLoop ⚠️ (有但未暴露状态)
```

**断层清单**：

| 底层能力 | CLI | Server | 穿透状态 |
|---------|:---:|:------:|---------|
| team/ Coordinator | ❌ | ❌ | **完全断裂** |
| VectorStore 语义搜索 | ⚠️ 内存 | ❌ | **严重不足** |
| longmem 长期记忆 | ⚠️ JSONL | ❌ | **部分断裂** |
| SummaryMemory/TokenBuffer | ⚠️ session自动 | ❌ | **不可配置** |
| TurnLoop 状态 | ⚠️ 有但未暴露 | ❌ | **不可观测** |
| spawn_agent | ✅ tool call | ❌ | **部分穿透** |

---

## 二、整合方案总览（3 Phase）

### Phase 1：Server 层穿透（~500 行新代码，0 行修改）
### Phase 2：CLI 层增强（~350 行新代码）
### Phase 3：持久化 + 性能（~400 行新代码）

---

## 三、Phase 1 — Server 层穿透

采用 **"可选注入 + 新 endpoint"** 策略，仿照已有 `SetMemoryStore()` / `SetSessionEndHook()` 模式。所有变更**纯增量**，未注入时 endpoint 返回 `501 Not Implemented`。

### Step 1.1: Team Coordinator 穿透

**新建 `server/team_store.go`**：
- `TeamConfig` 结构体（Name, Members[], MaxRounds）
- `TeamMemberConfig`（AgentID, Role, Handoff[]）
- `TeamStore` 内存存储（Create/Get/List/Delete）

**新建 `server/team_runner.go`**：
- `TeamRunner` 将 `TeamConfig` + `AgentStore` → `team.Coordinator`
- 轻量 `agentLikeRunner` 桥接 `AgentLike` → `team.AgentRunner`（签名一致，5 行 adapter）

**新建 `server/handlers_team.go`**：
```
POST   /v1/teams              — 创建 team 定义
GET    /v1/teams              — 列出所有 teams
GET    /v1/teams/{id}         — 获取 team 详情
DELETE /v1/teams/{id}         — 删除 team
POST   /v1/teams/{id}/run     — 执行协调（同步）
POST   /v1/teams/{id}/stream  — 执行协调（SSE 流式）
```

**修改 `server/server.go`**：
- Server struct 新增 `teamStore *TeamStore` + `teamRunner *TeamRunner` 字段
- 新增 `SetTeamCoordinator(c *team.Coordinator)` setter

**修改 `server/router.go`**：
- 注册 6 条 team 路由

### Step 1.2: Context/Semantic Search 穿透

**新建 `server/handlers_context.go`**：
- 定义轻量 `ContextProvider` 接口（Search + Stats），避免 Server 直接依赖 context 模块完整类型
- `SetContextProvider(p ContextProvider)` setter
```
GET /v1/context/search?q=...&limit=10&scope=session|task_group|global
GET /v1/context/stats
```

### Step 1.3: Memory 端点增强

**修改 `server/server.go`**：
- 扩展 `MemoryStore` 接口（保持向后兼容）：
  ```go
  type MemoryStore interface {
      // 现有 4 方法不变
      UpsertMemory(rec MemoryRecord) error
      GetMemory(id string) (*MemoryRecord, error)
      SearchMemory(query string, category string, limit int) ([]MemoryRecord, error)
      DeleteMemory(id string) error
      // 新增（可选实现，通过类型断言检查）
      SemanticSearch(ctx context.Context, query string, limit int) ([]MemoryRecord, error)
      MemoryStats() map[string]any
  }
  ```

**新建 `server/handlers_memory_enhanced.go`**：
```
POST /v1/memories/search         — 语义搜索（body: {query, limit, use_semantic}）
GET  /v1/memories/stats          — 记忆统计
POST /v1/memories/{id}/validate  — 提升 confidence
POST /v1/memories/{id}/negate    — 清零 confidence
```

### Step 1.4: TurnLoop 状态暴露

**修改 `orchestrator/agent/agent.go`**：
- 新增 `TurnPhase() string` 方法（idle/planning/active/unknown）

**新建 `server/handlers_agent_status.go`**：
```
GET /v1/agents/{id}/status  — 返回 {turn_phase, cancel_pending, round, tool_call_rounds}
```

### Step 1.5: AgentConfig 支持 MemoryStrategy

**修改 `server/server.go`** `AgentConfig`：
```go
type AgentConfig struct {
    // ... 现有字段
    MemoryStrategy string `json:"memory_strategy,omitempty"` // "token_buffer" | "summary" | ""
}
```
AgentStore.Create 根据 strategy 注入 SummaryMemory 或 TokenBufferMemory 作为 ResizeHandler。

---

## 四、Phase 2 — CLI 层增强

### Step 2.1: CLI team 命令

**新建 `cli/team_factory.go`**：
- `buildTeam(cfg, bridge, sessionID, teamCfg)` → `*team.Coordinator`
- 为每个 member 调用 `buildAgent()` 创建独立 agent
- 用 `team.AdaptAgent()` 包装

**新建 `cli/cmd/inferglow-cli/team.go`**：
```bash
inferglow team run --roles=planner,coder,reviewer   # 多 Agent 协作
inferglow team run --config=team.yaml                # 从 YAML 加载
```

**修改 `cli/cmd/inferglow-cli/main.go`**：
- 新增 `team` 子命令入口

### Step 2.2: CLI memory 命令

**新建 `cli/cmd/inferglow-cli/memory.go`**：
```bash
inferglow memory search <query> [--semantic] [--scope=global]
inferglow memory list [--session X]
inferglow memory stats
inferglow memory validate <id>
```

### Step 2.3: MemoryBridge 增强

**修改 `cli/memory_bridge.go`**：
- 支持外部 Store 注入：`WithStepStore(store)` / `WithVectorBackend(vb)`
- 当提供 Postgres Store 时跳过 JSONL
- 暴露 scope 参数给 memory_search 工具

---

## 五、Phase 3 — 持久化 + 性能

### Step 3.1: VectorStore 接口抽象

**新建 `context/retrieval/vectorstore.go`**：
```go
type VectorStoreBackend interface {
    Add(ctx context.Context, id string, vec []float32, meta VectorMeta) error
    Search(ctx context.Context, query []float32, limit int) ([]SearchResult, error)
    Delete(ctx context.Context, id string) error
    BatchAdd(ctx context.Context, items []VectorItem) error
}
```
现有内存实现保留为 `InMemoryVectorStore`。

### Step 3.2: pgvector 后端

**新建 `context/store/postgres/pgvector_store.go`**：
- 实现 `VectorStoreBackend`
- 复用现有 `postgres.Store` 的 `*sql.DB` 连接池
- IVFFlat 索引，`<=>` 余弦距离操作符
- 与 LongMem 表共享 session_id/step_id 关联

### Step 3.3: Redis VSS 缓存后端

**新建 `context/store/redis/vss_cache.go`**：
- 实现 `VectorStoreBackend`（可丢弃缓存层）
- `FT.CREATE` + HNSW 索引
- `RebuildFromStore(persistentBackend)` 从 pgvector 重建
- TTL 1h 过期

### Step 3.4: 性能优化

- **冒泡排序 → sort.Slice**：`semantic.go` 和 `fusion.go` 中 O(n²) → O(n log n)
- **Embedding 缓存**：`context/retrieval/embed_cache.go`，Redis Hash 缓存 text→embedding
- **异步 Ingest**：`context/ingest_pipeline.go`，写入 channel → 后台 goroutine 批量写入

---

## 六、Desktop GUI 预留接口

Desktop GUI（Wails v2 + React 18）内嵌模式直接创建 Agent 实例（零 IPC），通过 Wails Binding 暴露：

```go
// desktop/bindings.go（规划）
type AgentBindings struct {
    agent     *agent.Agent
    team      *team.Coordinator   // Phase 1 后可用
    memBridge *cli.MemoryBridge   // Phase 2 后可用
}

func (b *AgentBindings) TeamRun(members []string) (*TeamResult, error)
func (b *AgentBindings) MemorySearch(query, scope string) ([]MemoryRecord, error)
```

**无需额外底层改动**——Phase 1+2 完成后，Desktop GUI 可直接通过 Binding 调用。

---

## 七、依赖关系

```
Phase 1 (Server 穿透)
  1.1 Team ──────┐
  1.2 Context ───┤── 独立可并行
  1.3 Memory ────┤
  1.4 TurnLoop ──┘
       │
Phase 2 (CLI 增强)
  2.1 team 命令 ──── depends on 1.1
  2.2 memory 命令 ── depends on 1.3
  2.3 Bridge 增强 ── depends on 3.1
       │
Phase 3 (持久化)
  3.1 VectorStore 接口 ──┐
  3.2 pgvector ──────────┤── depends on 3.1
  3.3 Redis VSS ─────────┤
  3.4 性能优化 ──────────┘── 独立
```

---

## 八、风险与缓解

| 风险 | 缓解 |
|------|------|
| AgentLike vs AgentRunner 接口不兼容 | 签名完全一致，5 行 adapter 桥接 |
| Team 执行超时 | 提供异步路径：POST 返回 run_id + SSE 事件流 |
| pgvector 扩展不可用 | fallback 到 tsvector 全文搜索 + 应用层 rerank |
| MemoryStore 接口扩展破坏 InMemoryStore | 新增方法通过类型断言可选实现，InMemoryStore 不实现则返回 501 |
| CLI team 模式 spawn_agent 缺 FlowContext | 注入 minimal FlowContext 或 fallback 到 Engine.RunLoop |
| Redis 单点故障 | Redis 定位为可丢弃缓存，所有数据可从 pgvector 重建 |

---

## 九、关键文件清单

| 文件 | 操作 | Phase |
|------|------|-------|
| `server/server.go` | 修改（加字段+setter+接口扩展） | 1 |
| `server/router.go` | 修改（加路由） | 1 |
| `server/handlers_team.go` | **新建** | 1 |
| `server/team_store.go` | **新建** | 1 |
| `server/team_runner.go` | **新建** | 1 |
| `server/handlers_context.go` | **新建** | 1 |
| `server/handlers_memory_enhanced.go` | **新建** | 1 |
| `server/handlers_agent_status.go` | **新建** | 1 |
| `orchestrator/agent/agent.go` | 修改（加 TurnPhase 方法） | 1 |
| `cli/team_factory.go` | **新建** | 2 |
| `cli/cmd/inferglow-cli/team.go` | **新建** | 2 |
| `cli/cmd/inferglow-cli/memory.go` | **新建** | 2 |
| `cli/memory_bridge.go` | 修改（支持外部 Store 注入） | 2 |
| `context/retrieval/vectorstore.go` | **新建**（接口抽象） | 3 |
| `context/store/postgres/pgvector_store.go` | **新建** | 3 |
| `context/store/redis/vss_cache.go` | **新建** | 3 |
| `context/retrieval/semantic.go` | 修改（冒泡→sort.Slice） | 3 |

---

## 十、Rejected Alternatives

1. **重写 MemoryStore 为全新接口** → 破坏现有 InMemoryStore 和所有调用方。改为向后兼容扩展 + 类型断言可选实现。
2. **CLI 通过 HTTP 调 Server 获取 team 能力** → 增加部署复杂度（必须先启动 Server）。改为 CLI 本地构建 Coordinator + Server 远程执行双路径。
3. **统一 AgentLike 和 AgentRunner 接口** → 需要修改 orchestrator 和 server 两个模块的公共类型。改为轻量 adapter 桥接，零侵入。
4. **直接依赖具体 VectorStore 类型** → Server 会拉入 context 模块的完整依赖链。改为定义轻量 ContextProvider 接口隔离。
