# Phase 2: Memory Tool Chain

## 设计决策

- 工具放在 `cli/memory_actions.go`（单文件），不放 `builtins/actions/`
- 原因：`cli/go.mod` 已依赖 `action` + `memory`，零 go.mod 变更；当前仅 CLI 需要这些工具
- 搜索用简单子串匹配（Phase 2 记忆数量 < 50，BM25 过度工程）
- `memory`（recall）工具不 wrap `wrapWithIngest`（只读操作，避免噪声）

---

## Step 1: 创建 `cli/memory_actions.go`

**文件**: `inferglow/cli/memory_actions.go`（新建，~250 行）

三个工具遵循 `builtins/actions/file_read.go` 的 Config+Executor+Constructor 模式：

### 1.1 remember 工具

```go
func newRememberAction(store memory.Store) *action.Action
```

- Name: `"remember"`
- Schema 参数（对标 Reasonix remember.go）:
  - `name` (string, optional): kebab-case slug
  - `title` (string, optional): 人类可读标签
  - `description` (string, **required**): 一行摘要
  - `type` (string, enum: user/feedback/project/reference)
  - `scope` (string, enum: project/global)
  - `body` (string, **required**): 记忆内容 Markdown
- Execute 逻辑:
  1. 验证 description + body 必填
  2. `memory.NormalizeType()` / `memory.NormalizeFactScope()` 归一化
  3. 若提供 name，先 `store.Load(name)` 检查是否存在（存在则更新）
  4. `store.Save(m)` 写入
  5. 返回 `{id, revision, path}` 确认

### 1.2 memory 工具（recall）

```go
func newMemoryAction(store memory.Store) *action.Action
```

- Name: `"memory"`
- Schema 参数（对标 Reasonix recall.go）:
  - `operation` (string, **required**, enum: search/read/list)
  - `query` (string): search 查询
  - `name` (string): read 目标
  - `type` (string, optional filter)
  - `scope` (string, optional filter)
  - `limit` (int, default 8, max 20)
- Execute 逻辑:
  - `list`: `store.List()` + type/scope 过滤 + 格式化
  - `read`: `store.Load(name)` + 完整内容
  - `search`: `store.List()` + 子串匹配（Name/Title/Description/Body）+ limit 截断

### 1.3 forget 工具

```go
func newForgetAction(store memory.Store) *action.Action
```

- Name: `"forget"`
- Schema 参数:
  - `name` (string, **required**): 要归档的记忆名
- Execute 逻辑:
  1. 验证 name 必填
  2. `store.Archive(name)`
  3. 返回确认消息

---

## Step 2: 注册到 buildAgent

**文件**: `inferglow/cli/agent_factory.go`

在现有 action 注册后（~line 74）添加：

```go
// Memory tools (Phase 2): remember/memory/forget
memStore := bridge.MemStore()
actExt.Register(wrapWithIngest(newRememberAction(memStore), bridge))
actExt.Register(newMemoryAction(memStore)) // read-only, no ingest
actExt.Register(wrapWithIngest(newForgetAction(memStore), bridge))
```

注意：`memory`（recall）不 wrap `wrapWithIngest`，因为它是只读操作。

---

## Step 3: 编译验证

```bash
cd inferglow/cli && go build ./...
```

零 go.mod 变更（cli 已依赖 action + memory）。

---

## Step 4: 单元测试（可选）

**文件**: `inferglow/cli/memory_actions_test.go`（新建）

- 使用 `memory.StoreFor(t.TempDir(), ".")` 创建临时 store
- 测试 remember -> memory(list/read/search) -> forget 完整生命周期
- 测试必填字段缺失时的错误返回
- 测试 type/scope 归一化

---

## 依赖关系

```
Step 1 (memory_actions.go) → Step 2 (注册) → Step 3 (编译)
Step 1 → Step 4 (测试)
```

---

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 工具名冲突 | grep 确认无现有 remember/memory/forget action |
| 搜索质量低（子串匹配） | Phase 2 记忆少（<50），足够；Phase 3 升级 BM25 |
| 工具放 cli 不可复用 | 当前仅 CLI 需要；若 server 需要，后续迁移到 builtins |
| LLM 发送非法 type/scope | NormalizeType/NormalizeFactScope 优雅降级 |

---

## 被拒绝的替代方案

1. **放 builtins/actions/**：需要改 builtins/go.mod 添加 memory 依赖，增加回归面。当前仅 CLI 使用，不值得。
2. **BM25 缓存索引**：Phase 2 记忆数量极少（<50），子串匹配延迟 <1ms。BM25 是 Phase 3 的优化方向。
3. **expected_revision 乐观锁**：Reasonix 有但 InferGlow Store 没有。Phase 2 用 name 匹配更新即可，不引入并发控制复杂度。