# 程序记忆 + 情节记忆补强统一 Spec

> 对标 Reasonix Skill 渐进式披露 + 因果链追踪，统一解决五类记忆模型中两个最大缺口

---

## 目标

补齐 InferGlow 五类记忆模型中两个核心缺口：

1. **程序记忆（Procedural Memory）**：Skill Store + 渐进式披露 + 按需加载执行
2. **情节记忆（Episodic Memory）**：因果链追踪 + 增强上下文展开

两个方向通过**统一的 StepRecord 元数据扩展**和**共享的索引注入机制**协同工作。

---

## Part A: 程序记忆 — Skill Store

### A1. Skill 存储格式

**文件**: `skill/skill.go`（新建模块 `github.com/inferglow/skill`）

```go
type Skill struct {
    Name        string    // kebab-case 标识
    Description string    // 一行摘要（索引用）
    Body        string    // 完整 playbook（按需加载）
    Scope       Scope     // project | global | builtin
    RunAs       RunAs     // inline | subagent
    Path        string    // .md 文件路径
    CreatedAt   time.Time
    UpdatedAt   time.Time
    // 可选元数据（对标 Reasonix frontmatter）
    AllowedTools []string // subagent 工具限定
    ReadOnly     bool     // 强制只读
    Triggers     []string // 触发词
    AutoUse      string   // off | suggest | prefer | require
}
```

**.md 文件格式**（兼容 Reasonix）：
```markdown
---
name: go-test-fix
description: Run Go tests, diagnose failures, fix and re-run until green
runas: inline
triggers: run tests, test failure, 运行测试, 测试失败
autouse: suggest
---

# go-test-fix

1. Run `go test ./...` via bash
2. Parse failures: which tests, error message, file:line
3. Fix each failure (production bug → fix code; test bug → fix test)
4. Re-run until green or 2 attempts on same failure → STOP
```

### A2. Store 核心

**文件**: `skill/store.go`

```go
type Store struct {
    Dir       string // {data_dir}/projects/{slug}/skills
    GlobalDir string // {data_dir}/skills/global
}

// 核心方法
func (s *Store) List() []Skill                    // 列出所有活跃 skill
func (s *Store) Read(name string) (Skill, bool)   // 按名加载（含 Body）
func (s *Store) Save(sk Skill) error              // 写入 .md 文件（Write + Sync）
func (s *Store) Delete(name string) error         // 删除
func (s *Store) IndexBlock() string               // 渲染索引块（≤4000 chars）
```

### A3. 渐进式披露索引注入

**文件**: `cli/repl.go` 或 `cli/memory_bridge.go`

在 system prompt 中追加 Skills 索引：
```
# Skills — playbooks you can invoke

- go-test-fix — Run Go tests, diagnose failures, fix and re-run...
- code-review — Review pending changes, flag correctness/security...
- explore — Explore codebase in isolated subagent...

Call run_skill({ name: "<skill-name>", arguments: "<task>" })
```

### A4. `run_skill` builtin action

**文件**: `builtins/actions/run_skill.go`

```go
func NewRunSkillAction(cfg RunSkillConfig) *action.Action
// Name: "run_skill"
// Schema: { "name": string (required), "arguments": string }
// Executor:
//   1. store.Read(name) → 加载完整 Body
//   2. inline 模式: 返回 Body + arguments 作为 tool result
//   3. subagent 模式: 通过 FlowContext.RunAgent() 隔离子 agent 执行
```

### A5. 与现有 spawn_agent 的关系

```
run_skill (inline)    → Body 注入当前 turn，不隔离
run_skill (subagent)  → 等价于 spawn_agent + skill Body 作为 system prompt
spawn_agent           → 通用子 agent，无 skill 关联
```

`run_skill(subagent)` 内部调用 `FlowContext.RunAgent()`，复用 Phase 3 的 depth 控制。

### A6. 元记忆（AGENTS.md 等价）

**文件**: `skill/store.go` 的 `ProjectInstructions()` 方法

扫描项目根目录的 `AGENTS.md` / `INFERGLOW.md` / `REASONIX.md`，读取内容作为项目级指令注入 system prompt。这不是 skill，是始终注入的持久上下文。

```go
func (s *Store) ProjectInstructions(root string) string
// 按优先级扫描: INFERGLOW.md > AGENTS.md > REASONIX.md
// 返回第一个找到的文件内容，空字符串表示无
```

---

## Part B: 情节记忆 — 因果链追踪

### B1. StepRecord 元数据扩展

**文件**: `context/step.go`

在 `StepRecord` 中添加因果追踪字段：

```go
type StepRecord struct {
    // ... 现有字段 ...
    
    // 因果追踪元数据（B1 新增）
    FilesRead     []string `json:"files_read,omitempty"`     // 本步骤读取的文件
    FilesModified []string `json:"files_modified,omitempty"` // 本步骤修改的文件
    DependsOn     []int    `json:"depends_on,omitempty"`     // 依赖的步骤 ID
    TaskGroup     string   `json:"task_group,omitempty"`     // 任务分组标识
}
```

**数据来源**：在 `builtins/actions/` 的工具执行后自动提取：
- `file_read` → 记录到 `FilesRead`
- `file_write` / `edit_file` → 记录到 `FilesModified`
- `bash` → 解析命令中的文件路径
- 同一 `task_group` 内的步骤自动建立 `DependsOn` 关系

### B2. 因果链提取器

**文件**: `context/causal.go`（新建，~120 行）

```go
type CausalChain struct {
    Steps     []StepRecord  // 因果链上的步骤（按时间排序）
    RootStep  int           // 链的起始步骤
    FocusStep int           // 链的焦点步骤
    Files     []string      // 链上涉及的所有文件
}

// TraceFile 返回所有涉及指定文件的步骤（因果链）
func TraceChain(store StepStore, stepID int) (*CausalChain, error)

// TraceFiles 返回涉及指定文件集合的步骤
func TraceFiles(store StepStore, files []string, limit int) ([]StepRecord, error)

// TraceTaskGroup 返回同一 task_group 的所有步骤
func TraceTaskGroup(store StepStore, group string) ([]StepRecord, error)
```

### B3. `context_trace` 工具

**文件**: `context/tools/tools.go` 追加

```go
// ContextTraceTool implements context_trace
type ContextTraceTool struct {
    mgr contextmgr.ContextManager
}

func (t *ContextTraceTool) Name() string        { return "context_trace" }
func (t *ContextTraceTool) Description() string  { return "沿文件依赖链追踪因果关系" }
// Schema:
//   file: string (optional) — 追踪涉及该文件的所有步骤
//   step_id: int (optional) — 从该步骤出发追踪因果链
//   task_group: string (optional) — 列出同组所有步骤
//   limit: int (optional, default 10)
```

### B4. 增强 `context_surround`

**文件**: `context/tools/tools.go` 修改

在 `ContextSurroundInput` 中添加：

```go
type ContextSurroundInput struct {
    StepID    int    `json:"step_id"`
    Before    int    `json:"before,omitempty"`
    After     int    `json:"after,omitempty"`
    Causal    bool   `json:"causal,omitempty"`     // 新增：因果模式
    TaskGroup string `json:"task_group,omitempty"` // 新增：按组聚合
}
```

当 `Causal=true` 时，不是展示连续的前后 N 步，而是：
1. 从目标 step 的 `FilesModified` / `FilesRead` 出发
2. 找到所有涉及相同文件的步骤
3. 按时间排序返回（跳跃式展开，非连续窗口）

### B5. TaskGroup 自动分组

**文件**: `orchestrator/agent/engine.go`

在 `executeLoop` 中，每轮 LLM 调用时设置 `task_group`：

```go
// 每个 user message 开启一个新的 task group
taskGroup := fmt.Sprintf("turn_%d", round)
// 该轮内所有 step 自动标记 taskGroup
```

---

## Part C: 统一索引注入

### C1. System Prompt 结构

```
{base_system_prompt}

{memory_index}          ← MEMORY.md 内容（语义记忆，Phase 1 已实现）

{skills_index}          ← Skills 索引块（程序记忆，A3 新增）

{project_instructions}  ← AGENTS.md 内容（元记忆，A6 新增）
```

### C2. MemoryBridge 扩展

**文件**: `cli/memory_bridge.go`

```go
type MemoryBridge struct {
    // ... 现有字段 ...
    skillStore    *skill.Store    // 程序记忆
    projectRoot   string          // 元记忆扫描根
}

// BuildSystemPrompt 组装完整的 system prompt
func (b *MemoryBridge) BuildSystemPrompt(base, query string) string {
    prompt := base
    
    // 语义记忆注入
    if memText, _ := b.Recall(ctx, query); memText != "" {
        prompt += "\n\n" + memText
    }
    
    // 程序记忆索引注入
    if skills := b.skillStore.IndexBlock(); skills != "" {
        prompt += "\n\n" + skills
    }
    
    // 元记忆注入
    if instructions := b.skillStore.ProjectInstructions(b.projectRoot); instructions != "" {
        prompt += "\n\n" + instructions
    }
    
    return prompt
}
```

---

## 依赖关系

```
A1 → A2 → A3 → A4 → A5 → A6    (程序记忆)
B1 → B2 → B3 → B4 → B5          (情节记忆)
C1 ← A3 + A6 + Phase 1           (统一索引)
```

Part A 和 Part B 相互独立，可并行实施。Part C 依赖 A3/A6。

---

## 文件变更清单

| 文件 | 操作 | 行数估 |
|------|------|--------|
| `skill/skill.go` | 新建：Skill 类型 + 解析 | ~100 |
| `skill/store.go` | 新建：Store 核心 | ~150 |
| `skill/go.mod` | 新建：模块定义 | ~5 |
| `builtins/actions/run_skill.go` | 新建：run_skill 工具 | ~120 |
| `builtins/go.mod` | 编辑：+skill 依赖 | ~3 |
| `context/step.go` | 编辑：StepRecord 元数据扩展 | ~10 |
| `context/causal.go` | 新建：因果链提取器 | ~120 |
| `context/tools/tools.go` | 编辑：+context_trace + surround 增强 | ~80 |
| `orchestrator/agent/engine.go` | 编辑：task_group 自动分组 | ~10 |
| `cli/memory_bridge.go` | 编辑：+skillStore + BuildSystemPrompt | ~30 |
| `cli/agent_factory.go` | 编辑：注册 run_skill | ~5 |

**总计：~630 行新增 + ~30 行编辑**

---

## 验证计划

1. `go build ./...` — skill / context / builtins / cli / orchestrator 全模块编译
2. `go test ./skill/...` — Skill Store 单元测试（Save/Read/List/IndexBlock）
3. `go test ./context/...` — 因果链追踪测试（TraceFile/TraceTaskGroup）
4. `go test ./builtins/...` — run_skill 工具测试
5. `go test ./orchestrator/...` — 现有测试不回归
6. 手动验证：CLI 中调用 run_skill 加载 skill body，确认索引注入正确

---

## 被拒绝的替代方案

1. **复用 memory.Store 扩展 Skill**：memory 和 skill 的语义不同（事实 vs 流程），混在一起会导致索引混乱。独立模块更清晰。
2. **Reasonix 完整兼容**：Reasonix 的 skill 系统有 1435 行代码（含 plugin、profile、MCP binding 等）。InferGlow 当前不需要这么复杂，先实现核心（Store + Index + run_skill），未来按需扩展。
3. **StepRecord 独立因果表**：把因果元数据放在独立的 DB 而非 StepRecord 内。增加复杂度，且与 JSONL 存储不兼容。直接扩展 StepRecord 更简单。
4. **BM25 索引因果链**：用 BM25 搜索替代因果追踪。BM25 是关键词匹配，无法表达"步骤 3 读了文件 → 步骤 7 改了同一文件 → 步骤 12 测试失败"这种因果关系。
