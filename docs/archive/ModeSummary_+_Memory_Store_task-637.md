# Phase 1: ModeSummary + Memory Store

## 前置约束

- 所有文件持久化操作必须 Write 后立即 Sync（遵循项目规范）
- ModeSummary 不依赖 L0-L4 分级压缩，是独立的 session 级摘要压缩
- Memory Store 采用文件型 .md 格式，与 Reasonix 兼容
- 两个子系统通过 "standing facts" 在压缩摘要中协作

---

## Part A: Session Rewrite 基础能力

ModeSummary 需要重写 session 消息，当前 InferGlow 缺少此能力。

### A1. session 模块添加 Rewrite 方法

**文件**: `session/session.go`

在 `Session` 上添加：
```go
// Rewrite replaces ContextWindow with the given messages.
// FullContext is NOT modified (audit trail preserved).
// The original ContextWindow is returned for archival.
func (s *Session) Rewrite(msgs []ChatMessage) []ChatMessage {
    s.mu.Lock()
    defer s.mu.Unlock()
    old := s.ContextWindow
    s.ContextWindow = msgs
    return old
}
```

在 `SessionBackend` 接口中添加 `Rewrite` 方法（或作为可选接口 `RewritableBackend`）。

### A2. SessionExtension 暴露 Rewrite

**文件**: `orchestrator/agent/internal/extension/session.go`

```go
func (e *SessionExtension) Rewrite(msgs []session.ChatMessage) []session.ChatMessage {
    old := e.s.Rewrite(msgs)
    e.persist()
    return old
}
```

### A3. 归档函数

**文件**: `context/summary.go`（或 `context/archive.go`）

```go
// ArchiveMessages writes dropped messages to a timestamped .jsonl file.
// Each line is one ChatMessage JSON. Write + Sync per line.
func ArchiveMessages(dir string, msgs []session.ChatMessage) (string, error)
```

---

## Part B: ModeSummary 实现

### B1. SummaryManager 结构体

**文件**: `context/summary.go`（新建，~250 行）

```go
type SummaryManager struct {
    cfg        SummaryConfig
    store      StepStoreLike       // 复用现有 store 接口
    session    RewritableSession   // 新接口：PreparePrompt + Rewrite
    modelReq   model.StreamRequester // 用于生成摘要的 LLM
    mu         sync.RWMutex

    // 阈值配置（对标 Reasonix compact.go）
    softRatio      float64 // 0.5 — 通知
    snipRatio      float64 // 0.6 — 裁剪 tool results
    compactRatio   float64 // 0.8 — 触发压缩
    forceRatio     float64 // 0.9 — 强制压缩
    tailTokens     int     // 16384 — 保留尾部 token 预算

    // 卡住保护
    consecutiveCompacts int
    compactStuck        bool
    softNoticed         bool

    // 持久化路径
    archiveDir   string // 归档目录
    summaryPath  string // 压缩状态持久化路径
}
```

实现 `ContextManager` 接口：
- `Mode()` 返回 `ModeSummary`
- `Ingest()` 存储 step 到 store（Write + Sync）
- `BuildContext()` 组装上下文（standing facts + summary + tail）
- `TriggerCompression()` 手动触发压缩
- 其余方法（Search/Expand/Surround/Stats）委托给 store

### B2. 核心压缩流程

**文件**: `context/summary.go`

```go
// MaybeCompact 在每轮 LLM 调用后检查是否需要压缩。
// 对标 Reasonix Agent.maybeCompact()。
func (s *SummaryManager) MaybeCompact(ctx context.Context, promptTokens, windowTokens int) error

// Compact 执行压缩：
// 1. planCompaction — 确定 head/start 分割点
// 2. partitionFold — 分离 kept（用户消息）和 fold（可压缩）
// 3. foldEconomics — 经济性检查（< 400 tokens 跳过）
// 4. ArchiveMessages — 归档原文到 {timestamp}.jsonl（Write + Sync）
// 5. summarizeWithRetry — 调 LLM 生成结构化摘要
// 6. session.Rewrite — 重写 ContextWindow
// 7. persistState — 持久化压缩状态（Write + Sync）
func (s *SummaryManager) Compact(ctx context.Context, trigger string, force bool) error
```

### B3. 摘要 Prompt

**文件**: `context/summary_prompt.go`（新建，~60 行）

对标 Reasonix 的 `summarySystemPrompt`，包含 7 个段落：
- Standing facts & constraints（从 Memory Store 注入）
- Goal
- Decisions & rationale
- Files & code
- Commands & outcomes
- Errors & fixes
- Pending & next step

### B4. 压缩状态持久化

**文件**: `context/summary_state.go`（新建，~40 行）

```go
type SummaryState struct {
    LastCompactAt     int64  `json:"last_compact_at"`
    CompactCount      int    `json:"compact_count"`
    ConsecutiveCompacts int  `json:"consecutive_compacts"`
    CompactStuck      bool   `json:"compact_stuck"`
    LastArchivePath   string `json:"last_archive_path"`
}
// Save: Write + Sync
// Load: 从 JSON 文件恢复
```

### B5. 注册 ModeSummary

**文件**: `context/registry.go`

在 `NewContextManager()` 工厂中添加 `ModeSummary` 分支。

**文件**: `context/manager.go`

添加 `ModeSummary Mode = "summary"` 常量。

---

## Part C: Memory Store 实现

### C1. 核心 Store

**文件**: `memory/store.go`（新建模块 `github.com/inferglow/memory`，~200 行）

```go
type Store struct {
    Dir       string // {data_dir}/projects/{slug}/memory
    GlobalDir string // {data_dir}/memory/global
}

type Memory struct {
    ID          string    // 不可变标识
    Revision    int       // 单调递增版本号
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Name        string    // kebab-case slug，也是文件名 ({name}.md)
    Title       string    // 人类可读索引标签
    Description string    // 一行摘要
    Type        Type      // user | feedback | project | reference
    Scope       FactScope // project | global
    Body        string    // 记忆内容（Markdown）
}
```

方法：
- `Save(m Memory) (string, error)` — 写入 .md 文件 + 更新 MEMORY.md 索引（Write + Sync）
- `List() []Memory` — 列出所有活跃记忆
- `Index() string` — 返回 MEMORY.md 内容（注入 system prompt）
- `Path(name string) string` — 解析记忆文件路径
- `Archive(name string) error` — 归档到 .archive/ 目录
- `Load(name string) (Memory, bool)` — 读取单条记忆

### C2. .md 文件格式

```markdown
---
id: mem-abc123
revision: 3
type: feedback
scope: project
title: Prefers tabs
description: User prefers tabs over spaces in Go code
created: 2026-07-30T12:00:00Z
updated: 2026-07-30T14:30:00Z
---

**Why:** User's editor is configured with tab width 4.
**How to apply:** Always use tabs for indentation in Go files.
```

### C3. MEMORY.md 索引

```markdown
# Memory Index

- [prefers-tabs] Prefers tabs — User prefers tabs over spaces in Go code (feedback)
- [project-goal] Project goal — Building CLI equivalent to Reasonix TUI (project)
```

启动时注入 system prompt 的 `<memory_index>` 区域。

### C4. 持久化规范

所有写操作遵循 Write + Sync：
```go
func (s Store) Save(m Memory) (string, error) {
    // 1. 写入 .md 文件
    f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
    f.Write(content)
    f.Sync()  // 必须 Sync
    f.Close()

    // 2. 更新 MEMORY.md 索引
    s.rebuildIndex()  // 内部也 Write + Sync

    return path, nil
}
```

---

## Part D: 集成

### D1. executeLoop 接入 ModeSummary

**文件**: `orchestrator/agent/engine.go`

在 `executeLoop` 的每轮 LLM 调用后（Point 5 附近），添加：
```go
if e.contextMgr != nil && e.contextMgr.Mode() == contextmgr.ModeSummary {
    if sm, ok := e.contextMgr.(*contextmgr.SummaryManager); ok {
        sm.MaybeCompact(ctx, data.Usage.PromptTokens, data.Usage.ContextWindow)
    }
}
```

### D2. CLI 接入 Memory Store

**文件**: `cli/memory_bridge.go`

在 `NewMemoryBridge` 中初始化 Memory Store：
```go
memStore := memory.StoreFor(cfg.DataDir, cfg.WorkDir)
// 启动时加载 MEMORY.md 索引注入 system prompt
```

### D3. 压缩摘要注入 Standing Facts

**文件**: `context/summary.go` 的 `Compact()` 方法

在生成摘要前，从 Memory Store 读取 standing facts：
```go
facts := memStore.Index()  // MEMORY.md 内容
// 注入到摘要 prompt 的 "Standing facts & constraints" 段
```

### D4. RunOption 模式切换

**文件**: `orchestrator/agent/agent.go`

```go
func WithContextMode(mode contextmgr.Mode) RunOption {
    return func(c *runConfig) { c.contextMode = mode }
}
```

---

## 文件变更清单

| 文件 | 操作 | 行数估 |
|------|------|--------|
| `session/session.go` | 编辑：添加 Rewrite 方法 | ~15 |
| `orchestrator/agent/internal/extension/session.go` | 编辑：暴露 Rewrite | ~10 |
| `context/manager.go` | 编辑：添加 ModeSummary 常量 | ~3 |
| `context/registry.go` | 编辑：注册 ModeSummary | ~10 |
| `context/summary.go` | 新建：SummaryManager 核心 | ~250 |
| `context/summary_prompt.go` | 新建：摘要 prompt 模板 | ~60 |
| `context/summary_state.go` | 新建：压缩状态持久化 | ~40 |
| `context/archive.go` | 新建：归档函数 | ~40 |
| `memory/store.go` | 新建：Memory Store 核心 | ~200 |
| `memory/memory.go` | 新建：Memory 类型 + Load | ~80 |
| `memory/index.go` | 新建：MEMORY.md 索引管理 | ~60 |
| `orchestrator/agent/engine.go` | 编辑：接入 MaybeCompact | ~10 |
| `orchestrator/agent/agent.go` | 编辑：WithContextMode | ~8 |
| `cli/memory_bridge.go` | 编辑：接入 Memory Store | ~20 |

**总计：~800 行新增 + ~60 行编辑**

---

## 验证计划

1. `go build ./...` — 全模块编译通过
2. `go test ./context/...` — SummaryManager 单元测试
3. `go test ./memory/...` — Memory Store 单元测试
4. `go test ./orchestrator/...` — 集成测试不回归
5. `go test ./session/...` — Rewrite 方法测试
6. 手动验证：CLI 中触发压缩，确认归档文件生成、session 重写正确