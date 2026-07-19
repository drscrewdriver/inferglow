# Reasonix 上下文管理增强 — 实施计划

## 总体策略

采用 **分阶段渐进交付**，每阶段独立可合并、可测试、可回滚。核心设计原则：
- 复用 Reasonix 现有多模型配置模式（PlannerModel/RecoveryModel 先例）
- 保持 `session.Session` append-only 语义不变，contextmgr 平行双写
- **甜点区（Sweet Spot）**：上下文低于可调阈值（默认 256k）时，保持原有压缩逻辑（高缓存命中），不启动分级改写；超出甜点区后触发 compaction 时启用分级压缩（L1-L4）
- **step_id 标记**：分级压缩模式下，发给 API 的每条压缩消息块携带 `[step_N|role|Lx]` 前缀标记
- **子 Agent 兼容**：contextmgr 按 Session 实例化，子 Agent 继承配置但拥有独立 refs/level 文件
- **长记忆不偏离**：保留 Reasonix 现有 `memory.Store`（md 文件 + MEMORY.md 索引），L2 事实仅作为可选补充源
- **日志格式兼容**：contextmgr 文件与现有 session JSONL / archive JSONL 共存于同一目录体系
- **System Prompt 模板**：分级压缩启用时，在 system prompt 追加 `<context-management>` 静态模板
- **工具注册**：context_search/context_expand 采用运行时注册模式，与记忆工具同级

---

## 兼容性设计（贯穿所有 Phase）

### A. 子 Agent 兼容

**现状分析**：Reasonix 通过 `TaskTool` + `SubagentStore` 实现子 Agent：
- 每个子 Agent 有独立 `Session`、独立 transcript（`subagents/{ref}.jsonl` + `.meta.json`）
- `subagentOptions()` 从父级继承 compactRatio/archiveDir/keepPolicy 等压缩配置
- 支持 `continue_from` / `fork_from` 引用复用 transcript
- `MaxSubagentDepth` 控制递归层级（默认 2 层）

**兼容方案**：

1. **contextmgr 按 Session 实例化**：每个 Agent（含子 Agent）在 `New()` 时根据自身 Session 创建独立的 `ContextManager` 实例。子 Agent 的 contextmgr 拥有独立的 refs.jsonl 和 .lN.jsonl 文件，以子 Agent 的 transcript ref 为前缀。

2. **配置继承**：`subagentOptions()` 已继承压缩比率配置。新增 `CompressModel`、`SweetSpotTokens` 同样通过此路径传递给子 Agent：
   ```go
   // task.go — subagentOptions()
   func (t *TaskTool) subagentOptions(...) Options {
       return Options{
           // ... 现有字段 ...
           CompressModel:     t.compressModel,     // 新增
           SweetSpotTokens:   t.sweetSpotTokens,   // 新增
       }
   }
   ```

3. **子 Agent transcript 摘要回注父级**：子 Agent 完成后，`FormatSubagentResult()` 生成的摘要文本回到父级 session。父级的 contextmgr 将这条摘要作为一个整体 Ingest，标记 `task_boundary: true`，与 InferGlow 的 task_group_id 概念对齐。

4. **子 Agent 独立压缩域**：子 Agent 的 transcript 有自己的 context window 和压缩生命周期。父级 contextmgr 不直接管理子 Agent 的 refs/levels——子 Agent 的 contextmgr 自行管理。

### B. 长记忆兼容

**现状分析**：Reasonix 的 `memory.Store` 使用：
- 每条记忆一个 `.md` 文件（YAML frontmatter + Markdown body）
- `MEMORY.md` 索引文件（`[标题](文件名.md) - 描述`）
- 类型：user / feedback / project / reference
- 作用域：project / global
- 通过 `remember` / `memory` / `forget` 工具操作

**兼容原则**：**不替换、不偏离**，仅在数据流上做可选桥接。

1. **保留 memory.Store 为唯一长记忆入口**：contextmgr 不引入新的长记忆存储。L2 事实是**会话级**的压缩中间产物，不自动写入 memory.Store。

2. **可选的 L2→memory 桥接（Phase 4+）**：若用户显式启用，可提供 `/context promote` 命令，将高频事实（ref_count≥5, strength≥2.0）手动提升为 memory.Store 条目。这是纯增量功能，不影响现有 remember/memory 工具。

3. **memory.Store 在 BuildContext 中的位置**：BuildContext 的 head_buffer 区域已包含 `memory.Set.Index` 内容（通过 Compose 注入 system prompt）。无需额外处理。

### C. 日志格式兼容

**现状分析**：
- Session JSONL：`{timestamp}-{model}.jsonl`，每行一个 `provider.Message` JSON（含 role/content/tool_calls/reasoning_content 等字段）
- Archive JSONL：`{timestamp}.jsonl`，被折叠的原始消息（由 `archiveMessages()` 写入 `archiveDir`）
- Sub-agent：`subagents/{ref}.jsonl` + `{ref}.meta.json`
- Branch meta：`{session}.jsonl.meta`（CAS 版本追踪）

**兼容方案**：

1. **Session JSONL 不变**：contextmgr 不修改 session 的 Save/Load/Snapshot 逻辑。Session JSONL 始终保持 append-only 的 `provider.Message` 序列。分级压缩只影响**发送给 provider 的消息列表**（通过 BuildContext 覆盖），不影响磁盘存储。

2. **contextmgr 文件与 archiveDir 共存**：
   ```
   {session_dir}/
   ├── {timestamp}-{model}.jsonl          ← 现有 Session JSONL（不变）
   ├── {timestamp}-{model}.jsonl.meta     ← 现有 Branch meta（不变）
   ├── archive/                           ← 现有 archiveDir（不变）
   │   └── {timestamp}.jsonl              ← 现有 archive 消息
   ├── context/                           ← 新增 contextmgr 目录
   │   ├── {session_id}.refs.jsonl        ← refs 追踪
   │   ├── {session_id}.l1.jsonl          ← L1 压缩产物
   │   ├── {session_id}.l2.jsonl          ← L2 事实提取
   │   └── {session_id}.l3.jsonl          ← L3 行为掩码
   └── subagents/                         ← 现有子 Agent（不变）
       └── {ref}.jsonl + {ref}.meta.json
   ```

3. **refs.jsonl 格式与 Session JSONL 的映射**：refs.jsonl 的 `step_id` 对应 Session JSONL 中的消息序号（按 append 顺序递增）。`StepSessionLink` 记录双向映射，但不写入磁盘——仅在内存中维护。

4. **L0 即 Session JSONL**：contextmgr 不需要独立的 L0 文件。`Ingest()` 时直接从 Session.Messages 读取原文（通过 msg_index 映射），避免双写冗余。

### D. System Prompt 上下文管理模板

**现状分析**：Reasonix 的 system prompt 构建链路：
- `boot.go:428` → `memory.Compose(sysPrompt, mem)` 将 memory block 拼接到 system prompt
- `mem.Block()` = `BackgroundBlock()`（全局偏好 + MEMORY.md 索引）+ `instruction.Block()`（AGENTS.md 等文档）
- 自动召回：`input.go:192` → `c.memory.recall(source).Block()` 在用户输入尾部注入 `<memory-recall>` 块
- 关键：这些都在 **cache-stable prefix** 中，不能动态变化

**兼容方案**：

1. **上下文管理指令注入位置**：当启用分级压缩（`sweet_spot_tokens > 0` 或 contextmgr 已初始化）时，在 `memory.Compose()` 之后、skill index 注入之前，追加一段 `<context-management>` 模板：
   ```
   <context-management>
   当前上下文采用分级压缩管理。历史消息可能包含以下标记：
   - [step_N|role|Lx] 表示第 N 步、角色 role、压缩级别 Lx（L0=原文, L1=去噪, L2=事实, L3=行为掩码）
   - <compaction-summary> 包裹早期对话的单级摘要（甜点区内模式）
   使用 context_search 工具在压缩历史中搜索关键词。
   使用 context_expand 工具展开某个被压缩的 step 回原文。
   </context-management>
   ```

2. **注入方式**：不修改 `memory.Compose()`，而是在 `boot.go` 中 `sysPrompt = memory.Compose(sysPrompt, mem)` 之后，根据配置条件追加：
   ```go
   if cfg.Agent.SweetSpotTokens > 0 || cfg.Agent.CompressModel != "" {
       sysPrompt += "\n\n" + contextmgr.SystemPromptHint()
   }
   ```

3. **cache-stable prefix 影响**：这段模板是**静态的**（不随 session 状态变化），所以不影响 prefix cache 稳定性。它在 boot 时一次性注入，与 memory block 同级。

4. **子 Agent 继承**：子 Agent 的 system prompt 通过 `t.sysPrompt` 传递（`task.go:973`），已包含父级的完整 system prompt（含上下文管理模板）。子 Agent 无需额外注入。

### E. 与现有记忆提取的兼容性

**现状分析**：Reasonix 的记忆提取分三层：
- **静态层**：`memory.Set.Block()` → 注入 system prompt（cache-stable prefix）
- **动态召回层**：`AutoRecall(store, query)` → 根据用户输入自动匹配相关记忆，注入 `<memory-recall>` 块
- **工具层**：`remember`/`forget`/`recall`/`memory` 工具 → 模型主动操作 memory.Store

**兼容方案**：

1. **L2 事实 ≠ 长记忆**：分级压缩的 L2 事实提取是**会话级**中间产物，用于 BuildContext 拼接。`memory.Store` 是**跨会话**持久化存储。两者是不同层次，不自动互通。

2. **AutoRecall 不受影响**：`AutoRecall` 根据用户输入匹配 `memory.Store` 中的记忆，与 contextmgr 的 refs/levels 完全独立。分级压缩改变的是 session 内的历史消息，不影响 memory.Store 的检索。

3. **memory.Store 在 BuildContext 中的位置**：BuildContext 的 head_buffer 区域已包含 `memory.Set.Index` 内容（通过 system prompt 注入）。无需额外处理。

4. **可选的 L2→memory 桥接（Phase 4+）**：若用户显式启用，可提供 `/context promote` 命令，将高频事实（ref_count≥5, strength≥2.0）手动提升为 memory.Store 条目。这是纯增量功能，不影响现有 remember/memory 工具。

5. **记忆提取与压缩的时序**：
   - 用户输入 → AutoRecall 匹配 memory.Store → 注入 `<memory-recall>` 块（不变）
   - 模型响应 → session.Add() → ctxManager.Ingest() → refs 追踪
   - 触发压缩 → 分级压缩（L1-L4）→ 生成 step_id 标记
   - 下次请求 → BuildContext 拼接（含 L2 事实 + 压缩历史 + step_id 标记）

### F. Session 工具注册

**现状分析**：Reasonix 的工具注册分两种模式：
- **内置工具**：`tool.RegisterBuiltin()` 在 `init()` 注册（如 bash/read_file/edit_file）
- **运行时工具**：`reg.Add()` 在 boot 时条件注册（如 remember/forget/recall、task/read_only_task）
- 记忆工具通过 `addMemoryTools()` 闭包条件注册（`boot.go:937`）

**兼容方案**：

1. **context_search / context_expand 注册方式**：采用运行时工具注册模式，与记忆工具同级。在 `addMemoryTools()` 同级新增 `addContextTools()` 闭包：
   ```go
   contextToolsAdded := false
   addContextTools := func() string {
       if contextToolsAdded {
           return "context tools are already enabled."
       }
       contextToolsAdded = true
       reg.Add(contextmgr.NewSearchTool(a.ctxManager))
       reg.Add(contextmgr.NewExpandTool(a.ctxManager))
       return "enabled context_search, context_expand."
   }
   ```

2. **注册条件**：当 `sweet_spot_tokens > 0` 或 `compress_model != ""` 时，在 boot 时自动注册。不需要 `connect_tool_source` 触发。

3. **子 Agent 工具继承**：子 Agent 的工具集通过 `subagentOptions()` 继承。`ReadOnlySubagentToolRegistry` 会过滤掉写工具（与 remember/forget 同级）。context_search 是只读工具，context_expand 是写工具（修改 session），需要在只读注册表中过滤。

4. **工具 Schema 与 system prompt 模板的协同**：工具的 Description 本身就是 system prompt 的一部分（provider 会将工具 schema 发给模型）。因此 `<context-management>` 模板中的工具说明与工具 schema 保持一致，避免重复。

---

## Phase 1: 甜点区阈值 + 分级压缩开关（最小 diff，最高 ROI）

**目标**：引入甜点区阈值概念。上下文低于阈值时，保持原有压缩逻辑（soft→snip→compact 单级摘要），维持 prefix cache 高命中率；超出阈值后，compaction 时启用分级压缩（L1-L4）。

> **关键语义**：甜点区不是降低 thinking effort，而是控制**是否启动分级历史改写**。甜点区内 = 原有高命中模式；甜点区外 = 启用新压缩能力。

### 1.1 配置入口

**文件**: `internal/config/config.go` — `AgentConfig` 结构体（~line 1123）

```go
// SweetSpotTokens 设定甜点区阈值（token 数）。当 prompt tokens 低于此值时，
// 保持原有压缩逻辑（高缓存命中），不启动分级改写。0 或负值 = 始终启用分级压缩。
SweetSpotTokens int `toml:"sweet_spot_tokens"`
```

**文件**: `reasonix.example.toml` — `[agent]` 段

```toml
# sweet_spot_tokens = 256000   # 低于此 token 数时保持原有压缩逻辑（0 = 始终启用分级压缩）
```

### 1.2 Agent 字段与 Options

**文件**: `internal/agent/agent.go` — `Agent` 结构体（~line 254）和 `Options`（~line 882）

- 在 `Options` 中添加 `SweetSpotTokens int`
- 在 `Agent` 中添加对应字段 `sweetSpotTokens int`
- 在 `New()` 构造函数中从 Options 初始化

### 1.3 甜点区判断逻辑

**文件**: `internal/agent/compact.go` — `maybeCompact()`（~line 85）

在 `maybeCompact()` 中增加甜点区判断：

```go
func (a *Agent) maybeCompact(ctx context.Context, u *provider.Usage) {
    if a.contextWindow <= 0 || u == nil || u.PromptTokens == 0 {
        return
    }
    // 甜点区判断：低于阈值时走原有压缩路径（高缓存命中）
    inSweetSpot := a.sweetSpotTokens > 0 && u.PromptTokens < a.sweetSpotTokens
    if inSweetSpot {
        // 原有逻辑：soft notice → snip → compact（单级摘要）
        a.maybeCompactLegacy(ctx, u)
        return
    }
    // 超出甜点区：走分级压缩路径（Phase 3 实现）
    if a.ctxManager != nil {
        a.maybeCompactTiered(ctx, u)
        return
    }
    // 兜底：未配置 contextmgr 时仍走原有逻辑
    a.maybeCompactLegacy(ctx, u)
}
```

- 将现有 `maybeCompact()` 逻辑重命名为 `maybeCompactLegacy()`
- 新增 `maybeCompactTiered()` 在 Phase 3 实现
- 甜点区阈值可调，默认 256k tokens，0 = 始终启用分级压缩

### 1.4 测试

- `internal/agent/sweet_spot_test.go`: 验证低于阈值时走原有压缩路径、高于阈值时走分级路径
- 配置解析测试：验证 TOML 字段正确读取

---

## Phase 2: 压缩用小模型配置入口

**目标**：为分级压缩提供独立小模型配置，复用现有多模型模式。

### 2.1 配置入口

**文件**: `internal/config/config.go` — `AgentConfig`（~line 1123）

```go
// CompressModel 可选的压缩专用模型。为空时使用主模型。
CompressModel string `toml:"compress_model"`
```

**文件**: `reasonix.example.toml`

```toml
# compress_model = "deepseek-v4-flash"  # 压缩用小模型（推荐 3B-7B 级别）
```

### 2.2 Provider 解析

**文件**: `internal/config/config.go` 或 controller boot 路径

- 复用 `PlannerModel` 的解析模式：通过 `ResolveProvider(model, effort)` 获取独立 provider 实例
- 小模型 provider 使用 temperature=0, 短超时（5s）

### 2.3 Agent 集成

**文件**: `internal/agent/agent.go`

- `Options` 添加 `CompressModel string` 和 `CompressProvider provider.Provider`
- `Agent` 添加 `compressProv provider.Provider` 字段
- 在 `summarize()` 中优先使用 `compressProv`（若配置），否则 fallback 到主模型 `a.prov`

### 2.4 修改 summarize()

**文件**: `internal/agent/compact.go` — `summarize()`（~line 671）

```go
func (a *Agent) summarize(ctx context.Context, region []provider.Message, instructions string) (string, error) {
    // 选择压缩 provider：小模型 > 主模型
    prov := a.prov
    if a.compressProv != nil {
        prov = a.compressProv
    }
    // ... 其余逻辑不变，只是 prov.Stream() 用选择的 provider
}
```

### 2.5 降级链路

在 `summarize()` 中实现小模型 → 主模型 → 机械降级的三级 fallback：
1. 小模型超时（5s）或返回空 → 重试 1 次
2. 仍失败 → 切换到主模型
3. 主模型也失败 → 使用 `mechanicalFoldDigest()`（已有）

### 2.6 Render 支持

**文件**: `internal/config/render.go`

- 添加 `compress_model` 的渲染逻辑（参考 `planner_model` 的 ~line 249 模式）

### 2.7 测试

- 配置解析测试
- `summarize()` 降级链路测试：mock 小模型超时 → 验证 fallback 到主模型

---

## Phase 3: 分级压缩核心引擎

**目标**：实现 L0-L4 分级压缩 + refs 追踪 + step_id 标记，替代当前的单级摘要。

### 3.1 新建包

**目录**: `internal/contextmgr/`

```
internal/contextmgr/
├── contextmgr.go       # ContextManager 接口 + 模式枚举
├── store.go            # StepStore 接口（JSONL 文件实现）
├── refs.go             # refs.jsonl 读写 + RefRecord 结构
├── compressor.go       # CompressModelClient 接口 + CompressModelChain
├── levels.go           # L1/L2/L3 压缩逻辑（LLM + 机械降级）
├── prompts.go          # L1/L2/L3 提示词模板
├── decay.go            # effective_decay 计算引擎
├── markers.go          # step_id 标记生成器
├── hint.go             # SystemPromptHint() 静态模板
├── tools.go            # context_search / context_expand 工具实现
└── contextmgr_test.go
```

### 3.2 核心接口

```go
// ContextManager 管理分级压缩
type ContextManager interface {
    Ingest(step StepRecord) error           // 新 step 写入 refs
    CompressStep(stepID int, targetLevel int) error  // 降级压缩
    BuildContext(windowTokens int) ([]RenderedBlock, error)  // 五区拼接
    Mode() string                           // passthrough | hybrid
    ActiveRefs() []RefRecord                // 当前活跃引用
}
```

### 3.3 存储层

- `StepStore` 接口：`GetStep()`, `GetRef()`, `UpsertRef()`, `AllActiveStepIDs()`
- 默认实现：`FileStore`（JSONL 文件，与 Reasonix 的 `archiveDir` 同目录）
- 文件格式：`{session_id}.refs.jsonl`、`{session_id}.l1.jsonl` 等

### 3.4 step_id 标记格式

分级压缩模式下，发给 API 的每条压缩消息块携带 `[step_N|role|Lx]` 前缀标记，让模型能区分不同步骤和压缩级别：

```
// 分级压缩后的消息格式示例：
[step_1|user|L0] 帮我把 Redis 配置迁移到独立文件
[step_3|tool|L2] [事实] grep redis: config.py:12 REDIS_HOST=localhost, :13 REDIS_PORT=6379
[step_5|assistant|L1] 需要先创建 redis_config.py...
[step_8|tool|L3] [掩码 step_8|原8.2K|grep|redis.*timeout] 搜索redis超时相关行
```

**实现位置**：在 `contextmgr` 的 `renderStep()` 中生成，作为 `provider.Message.Content` 的前缀。

**与现有格式的兼容**：
- 甜点区内（原有模式）：不加标记，保持 `<compaction-summary>` 标签包裹摘要
- 甜点区外（分级模式）：加 `[step_N|role|Lx]` 标记

**标记字段说明**：
- `step_N`：对应 Session.Messages 中的消息序号（append 顺序递增）
- `role`：原始角色（user/assistant/tool）
- `Lx`：当前压缩级别（L0=原文, L1=去噪, L2=事实, L3=掩码）

### 3.5 压缩提示词

从 InferGlow spec §4 直接移植 L1/L2/L3 提示词模板到 `prompts.go`。

### 3.6 System Prompt 模板

**文件**: `internal/contextmgr/hint.go`

```go
// SystemPromptHint 返回静态的上下文管理指令模板。
// 在 boot.go 中根据配置条件追加到 sysPrompt。
func SystemPromptHint() string {
    return `<context-management>
当前上下文采用分级压缩管理。历史消息可能包含以下标记：
- [step_N|role|Lx] 表示第 N 步、角色 role、压缩级别 Lx（L0=原文, L1=去噪, L2=事实, L3=行为掩码）
- <compaction-summary> 包裹早期对话的单级摘要（甜点区内模式）
使用 context_search 工具在压缩历史中搜索关键词。
使用 context_expand 工具展开某个被压缩的 step 回原文。
</context-management>`
}
```

### 3.7 工具注册

**文件**: `internal/contextmgr/tools.go`

```go
// NewSearchTool 返回 context_search 工具（只读）
func NewSearchTool(mgr ContextManager) tool.Tool {
    return searchTool{mgr: mgr}
}

// NewExpandTool 返回 context_expand 工具（写）
func NewExpandTool(mgr ContextManager) tool.Tool {
    return expandTool{mgr: mgr}
}
```

**boot.go 注册**：在 `addMemoryTools()` 同级新增 `addContextTools()` 闭包。

### 3.8 与 Session 的双写桥接

**文件**: `internal/agent/run_loop.go`

在每轮 `session.Add()` 后，同步调用 `ctxManager.Ingest()`：

```go
a.session.Add(msg)
if a.ctxManager != nil {
    a.ctxManager.Ingest(agent.StepRecord{...})
}
```

**BuildContext 回灌**：当 `ctxManager.Mode() != "passthrough"` 时，在 Stream 调用前用 `BuildContext()` 结果覆盖发送给 provider 的消息列表。

### 3.9 压缩触发

复用 `maybeCompact()` 的阈值检测，但在触发时调用分级压缩而非单级 summarize：
- 50%-60% → L1 去噪（批量）
- 60%-80% → L2 事实提取
- 80%-90% → L3 行为掩码
- >90% → L4 丢弃 + 机械折叠

---

## Phase 4: BuildContext 五区拼接 + 上下文工具

**目标**：完整的五区上下文拼接 + context_search/expand 工具。

### 4.1 BuildContext 五区拼接

按 InferGlow spec §4.5-4.6 实现：
1. head_buffer（永不压缩的系统提示 + 首条用户消息）
2. 高频事实注入区（从 .l2.jsonl 筛选 ref_count≥3 的事实）
3. 压缩历史区（按 step_id 升序，按 level 选文件渲染，带 `[step_N|role|Lx]` 标记）
4. tail 原文区（最近 N 步保持 L0）
5. HintBlock 注入区（上下文压力、当前任务组等动态信息）

### 4.2 Rendered Cache

参考 Agent B 的优化方案，对已渲染且 level 未变的 step 缓存渲染结果。

### 4.3 上下文工具（可选）

- `context_search`：在压缩后的历史中搜索关键词
- `context_expand`：展开某个被压缩的 step 回原文

### 4.4 可选的 L2→memory 桥接

- `/context promote` 命令：将高频事实手动提升为 memory.Store 条目
- 纯增量功能，不影响现有 remember/memory 工具

---

## 依赖关系

```
Phase 1 (甜点区开关)   ─── 无依赖，可独立交付（拆分 maybeCompact 为 Legacy/Tiered）
Phase 2 (小模型配置)   ─── 无依赖，可独立交付
Phase 3 (分级压缩引擎) ─── 依赖 Phase 1（maybeCompactTiered 实现）+ Phase 2（compressProv）
Phase 4 (五区拼接)     ─── 依赖 Phase 3（refs + 分级文件 + step_id 标记）
```

Phase 1 和 Phase 2 可并行开发。

---

## 关键文件清单

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `internal/config/config.go` | 修改 | 添加 SweetSpotTokens/CompressModel 配置字段 |
| `internal/agent/agent.go` | 修改 | Options/Agent 新字段、New() 初始化 |
| `internal/agent/compact.go` | 修改 | maybeCompact 拆分 Legacy/Tiered；summarize() 支持 compressProv 降级链路 |
| `internal/agent/run_loop.go` | 修改 | ctxManager 双写调用点 |
| `internal/agent/task.go` | 修改 | subagentOptions() 继承新配置 |
| `internal/config/render.go` | 修改 | 渲染新配置字段 |
| `internal/boot/boot.go` | 修改 | 追加 `<context-management>` 模板到 sysPrompt；注册 context_search/context_expand 工具 |
| `reasonix.example.toml` | 修改 | 文档化新配置项 |
| `internal/contextmgr/*.go` | **新建** | Phase 3+4 的分级压缩核心包（含 step_id 标记生成、工具注册） |

---

## 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 甜点区阈值设置不当导致频繁触发分级压缩 | 默认 256k 保守值，用户可调；0 = 始终启用分级压缩 |
| 分级压缩后 step_id 标记占用额外 token | 标记格式精简（~15 tokens/条），仅分级模式启用 |
| `<context-management>` 模板占用 prefix cache 空间 | 模板是静态的（~200 tokens），仅在启用分级压缩时注入；与 memory block 同级，不影响动态 prefix |
| context_search/context_expand 工具 schema 占用 token | 两个工具 schema ~300 tokens；仅在启用分级压缩时注册 |
| 双写增加 I/O 开销 | Ingest 设计为 append-only + 异步 flush，不阻塞主循环 |
| 分级压缩质量不如单级摘要 | 保留 `mechanicalFoldDigest` 作为最终 fallback；质量校验规则（压缩后不能膨胀） |
| 小模型端点不可用 | 三级降级链路（小模型→主模型→机械），与现有 `mechanicalFoldDigest` 对接 |
| refs.jsonl 与 Session 不一致 | step_id ↔ msg_index 映射表 + 启动时一致性校验 |

---

## 被拒绝的方案

1. **一步到位完整实现（Agent A 原始方案）**：10 步完整 contextmgr 包一次性交付风险过高，无法中途验证价值
2. **O(1) decay 引擎 + 异步压缩（Agent B）**：首版不需要如此复杂的性能优化，rendered cache 留到 Phase 4
3. **替换现有 compact.go**：保留现有单级摘要作为 fallback，分级压缩作为增强模式并行存在，通过 mode 切换
4. **用 contextmgr 替代 memory.Store**：长记忆系统保持 Reasonix 原生的 md+MEMORY.md 方案，不引入新的持久化存储
5. **子 Agent 共享父级 contextmgr**：子 Agent 需要独立的压缩域和 refs 空间，共享会导致 step_id 冲突和压缩策略耦合
6. **动态修改 system prompt 注入上下文状态**：会破坏 prefix cache 稳定性；改为静态模板 + 运行时 BuildContext 覆盖