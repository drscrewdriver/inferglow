# Context Manager 日志结构双写修复 Spec

## 问题描述

当前 contextmgr 的日志记录存在多个结构性缺陷，导致分级压缩无法正确追踪会话状态，子 agent 的 steps 混入主 context，且无法从日志中恢复完整的会话结构。

## 现状分析

### 1. Session JSONL 空文件
```bash
$ ls -la ~/.reasonix/projects/.../sessions/
-rw-r--r-- 1 joshua joshua      0 Jul 29 17:55 20260729-095549.359951970-session.jsonl
-rw------- 1 joshua joshua 187715 Jul 29 18:26 20260729-095549.359951970-session.events.jsonl
```
- `session.jsonl` 为空文件（0 字节）
- 消息只写入 `events.jsonl`
- **根因**：Session 结构可能只使用内存，未持久化到 JSONL

### 2. Context Manager 只写 main.refs.jsonl
```bash
$ ls ~/.reasonix/sessions/context/
main.refs.jsonl  # 唯一文件
```
- 没有按 session 名分文件（如 `session-xxx.refs.jsonl`）
- 子 agent 的 steps 混入主 context
- **根因**：`contextmgr.New()` 硬编码 `SessionID: "main"`

### 3. RefRecord 缺少 Role 字段
```json
{"step_id":2,"level":0,"ref_count":0,"strength":1,"task_group_id":0,"task_boundary":false}
```
- 无法从 refs 中区分 user/assistant/tool
- 压缩后丢失角色信息，影响上下文组装
- **根因**：`RefRecord` 结构体未包含 `Role` 字段

### 4. Step ID 冲突
```bash
$ cat main.refs.jsonl | jq -c '{step_id}'
{"step_id":2}  # 第一组
{"step_id":3}
...
{"step_id":19}
{"step_id":2}  # 第二组（重复）
{"step_id":3}
...
{"step_id":17}
```
- 两组 step_id 都从 2 开始
- 多实例共享同一文件，ID 冲突
- **根因**：
  - 子 agent 可能创建了自己的 contextmgr 实例
  - 或者 session 重启后 contextmgr 重新初始化，但 `loadRefs` 没有正确恢复 `nextStep`

### 5. 子 Agent 无独立 Context
- 子 agent 通过 `New()` 创建，但没有调用 `SetContextManager()`
- 子 agent 的 `ctxManager == nil`，`addAndIngest` 不执行 Ingest
- 子 agent 的 steps 不进入分级压缩
- **根因**：`subagentOptions()` 没有传递 contextmgr 配置

### 6. 双写桥接不完整
```go
// addAndIngest 只在 ctxManager != nil 时执行
func (a *Agent) addAndIngest(msg provider.Message) {
    a.session.Add(msg)
    if a.ctxManager != nil && !msg.LocalOnly {
        step := ContextStep{
            StepID:     len(a.session.Messages),  // 基于消息计数
            Role:       string(msg.Role),
            Content:    msg.Content,
            ToolName:   msg.Name,
            TokenCount: estimateMessageTokens(msg),
        }
        _ = a.ctxManager.Ingest(step)
    }
}
```
- 主 agent 的 steps 写入 contextmgr
- 子 agent 的 steps 不写入（ctxManager == nil）
- StepID 基于 `len(session.Messages)`，可能与 contextmgr 的 `nextStep` 不同步

## 修复方案

### Phase 1: RefRecord 补全 Role 字段

**文件**：`internal/contextmgr/contextmgr.go`

```go
type RefRecord struct {
    StepID         int     `json:"step_id"`
    Level          Level   `json:"level"`
    Role           string  `json:"role"`  // 新增
    RefCount       int     `json:"ref_count"`
    LastRefAtStep  *int    `json:"last_ref_at_step,omitempty"`
    Strength       float64 `json:"strength"`
    TaskGroupID    int     `json:"task_group_id"`
    TaskBoundary   bool    `json:"task_boundary"`
}
```

**影响**：
- `Ingest()` 时从 `StepRecord.Role` 复制到 `RefRecord.Role`
- `UpsertRef()` 时保留 Role
- `BuildContext()` 时从 RefRecord 读取 Role

### Phase 2: Context Manager 按 Session 分文件

**文件**：`internal/boot/boot.go`

#### 现状

```go
// 当前硬编码（boot.go:996）
sessionID := "main"
ctxMgr, err = contextmgr.New(contextmgr.Config{
    Dir:             filepath.Join(sessionDir, "context"),
    SessionID:       sessionID,
    ...
})
```

所有 session 共享同一个 `main.refs.jsonl`，导致 step_id 冲突、多 session 数据混杂。

#### 原生 Session ID 口径分析

原生 session 文件由 `agent.NewSessionPath()` 生成（`internal/agent/save.go:2069`）：
```go
// 格式: 20060102-150405.000000000-model.jsonl
// 示例: 20260729-095549.359951970-session.jsonl
func NewSessionPath(dir, model string) string {
    safe := strings.NewReplacer(...).Replace(model)
    return filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl",
        time.Now().UTC().Format("20060102-150405.000000000"), safe))
}
```

原生 session stem（去掉 `.jsonl`）= `20260729-095549.359951970-session`

**对齐难点**：`addContextTools()` 在 `Build()` 内部调用（boot.go:1033），此时 Controller 尚未返回，session path 由外部（CLI `cli.go:658` 或 Desktop `tabs.go:3762`）在 `Build()` 返回后才通过 `SetSessionPath()` / `SetFreshSessionPath()` 设置。因此 **无法在 contextmgr 初始化时直接拿到原生 session stem**。

#### 方案 A：延迟初始化，对齐原生 Session ID

将 contextmgr 初始化从 `Build()` 内的立即调用改为延迟到 session path 确定后：

```go
// boot.go: addContextTools 不再立即初始化 contextmgr
// 改为在 Controller.SetSessionPath() 时回调初始化
func (c *Controller) SetSessionPath(path string) {
    c.sessionPath = path
    // 提取 stem 作为 session ID
    sessionStem := strings.TrimSuffix(filepath.Base(path), ".jsonl")
    // 回调初始化 contextmgr
    if c.onSessionReady != nil {
        c.onSessionReady(sessionStem)
    }
}
```

```go
// boot.go: Build() 中注册回调
ctrl.OnSessionReady(func(sessionStem string) {
    ctxMgr, err = contextmgr.New(contextmgr.Config{
        Dir:             filepath.Join(sessionDir, "context"),
        SessionID:       sessionStem,  // 对齐原生: "20260729-095549.359951970-session"
        TailKeepSteps:   5,
        HotFactRefCount: 3,
        HotFactStrength: 1.3,
    })
    // ... 注册工具、设置 adapter ...
})
```

**优点**：context 文件名与 session 文件一一对应，可直接从文件名关联
```
~/.reasonix/sessions/context/
├── 20260729-095549.359951970-session.refs.jsonl
├── 20260729-095549.359951970-session.l1.jsonl
└── ...
```

**缺点**：需要改 Controller 增加回调机制，初始化流程变复杂；context tools 在 session path 设置前不可用。

#### 方案 B：自己生成唯一 ID（推荐）

不依赖原生 session path，contextmgr 自行生成唯一 ID：

```go
// boot.go: addContextTools 内
sessionID := "ctx-" + time.Now().UTC().Format("20060102-150405.000000000")
ctxMgr, err = contextmgr.New(contextmgr.Config{
    Dir:             filepath.Join(sessionDir, "context"),
    SessionID:       sessionID,  // 如: "ctx-20260729-095549.359951970"
    TailKeepSteps:   5,
    HotFactRefCount: 3,
    HotFactStrength: 1.3,
})
```

**输出结构**：
```
~/.reasonix/sessions/context/
├── ctx-20260729-095549.359951970.refs.jsonl
├── ctx-20260729-095549.359951970.l1.jsonl
├── ctx-20260729-095549.359951970.l2.jsonl
└── ctx-20260729-095549.359951970.l3.jsonl
```

**优点**：改动最小，只需改一行 `sessionID :=` 赋值；无需改 Controller 初始化流程；时间戳纳秒精度保证唯一性。

**缺点**：context 文件名与 session 文件名无直接对应关系，需要通过时间戳或元数据关联。

#### 推荐：方案 B

理由：
1. 改动量极小，风险低
2. 纳秒时间戳与原生 session 文件时间戳同源（都是 `time.Now().UTC()`），可通过时间相近性关联
3. 不需要改 Controller 的初始化流程
4. 后续如果需要关联，可以在 context 目录写一个 `.meta.json` 记录对应的 session path

#### 最终实现：方案 A（延迟初始化）

**已实现**。通过 Controller 回调机制，在 session path 确定后才初始化 contextmgr，确保 session ID 与原生 session 文件 stem 完全对齐。

**改动文件**：
1. `internal/control/controller.go`:
   - 新增 `onSessionPathChange func(path string)` 字段
   - 新增 `Options.OnSessionPathChange` 配置
   - `setSessionPath()` 在更新路径后触发回调

2. `internal/contextmgr/tools.go`:
   - `SearchTool`、`ExpandTool`、`ReorganizeTool` 新增 `SetManager(*Manager)` 方法
   - 支持工具注册时 manager 为 nil，后续延迟绑定

3. `internal/boot/boot.go`:
   - `addContextTools()` 改为注册空 manager 的工具
   - 新增 `initContextMgr(sessionStem string)` 闭包，延迟创建 contextmgr
   - 通过 `ctrlOpts.OnSessionPathChange` 注册回调，从 session path 提取 stem 并调用 `initContextMgr`

**输出结构**：
```
~/.reasonix/sessions/context/
├── 20260729-095549.359951970-session.refs.jsonl
├── 20260729-095549.359951970-session.l1.jsonl
└── ...
```

**初始化流程**：
1. `Build()` 期间：`addContextTools()` 注册工具（manager=nil），设置 `ctrlOpts.OnSessionPathChange` 回调
2. `Build()` 返回后：调用方设置 session path（`SetSessionPath` / `SetFreshSessionPath`）
3. `setSessionPath()` 触发回调 → 提取 stem → `initContextMgr(stem)` → 创建 contextmgr → `SetManager` 更新工具 → 设置 executor adapter

### Phase 3: 子 Agent 独立 Context

**文件**：`internal/agent/task.go`

```go
func (t *TaskTool) subagentOptions(...) Options {
    return Options{
        ...
        // 新增：传递 contextmgr 配置
        ContextMgrDir:       t.contextMgrDir,
        ContextMgrSessionID: t.contextMgrSessionID + "_sub_" + subagentID,
        ...
    }
}
```

**子 Agent Context 结构**：
```
~/.reasonix/sessions/context/
├── 20260729-095549.359951970-session.refs.jsonl
├── 20260729-095549.359951970-session_sub_sa_20260729_102208.refs.jsonl  # 子 agent 独立
└── 20260729-095549.359951970-session_sub_sa_20260729_102416.refs.jsonl
```

**或者**：子 agent 共享主 agent 的 contextmgr，但通过 `TaskGroupID` 区分

### Phase 4: Session JSONL 持久化

**文件**：`internal/agent/session.go`

```go
// Session 结构添加 JSONL writer
type Session struct {
    Messages []provider.Message
    jsonlWriter *os.File  // 新增
}

// Add 时同步写入 JSONL
func (s *Session) Add(msg provider.Message) {
    s.Messages = append(s.Messages, msg)
    if s.jsonlWriter != nil {
        // 写入 JSONL 并 flush
        data, _ := json.Marshal(msg)
        s.jsonlWriter.Write(data)
        s.jsonlWriter.Write([]byte("\n"))
        s.jsonlWriter.Sync()  // 即操作即 flush
    }
}
```

### Phase 5: Step ID 全局唯一

**方案 A**：使用 UUID
```go
type RefRecord struct {
    StepID         string  `json:"step_id"`  // 改为 string
    ...
}

// 生成
stepID := uuid.New().String()
```

**方案 B**：使用 session-scoped 计数器
```go
// contextmgr.Manager 维护全局 nextStep
func (m *Manager) Ingest(step StepRecord) error {
    if step.StepID <= 0 {
        step.StepID = m.nextStep
    }
    m.nextStep = step.StepID + 1
    ...
}
```

**推荐方案 B**，保持整数 ID，但确保 `loadRefs()` 正确恢复 `nextStep`。

### Phase 6: 双写桥接同步

**文件**：`internal/agent/compact.go`

```go
func (a *Agent) addAndIngest(msg provider.Message) {
    a.session.Add(msg)
    if a.ctxManager != nil && !msg.LocalOnly {
        // 使用 contextmgr 的 nextStep，而非 len(session.Messages)
        step := ContextStep{
            StepID:     0,  // 让 contextmgr 自动分配
            Role:       string(msg.Role),
            Content:    msg.Content,
            ToolName:   msg.Name,
            TokenCount: estimateMessageTokens(msg),
        }
        _ = a.ctxManager.Ingest(step)
    }
}
```

## 验证标准

### 1. Session JSONL 完整性
```bash
$ wc -l ~/.reasonix/projects/.../sessions/*.jsonl
150 session.jsonl  # 非空
150 session.events.jsonl
```

### 2. Context Manager 按 Session 分文件（方案 A：对齐原生 Session ID）
```bash
$ ls ~/.reasonix/sessions/context/
20260729-095549.359951970-session.refs.jsonl
20260729-095549.359951970-session.l1.jsonl
20260729-095549.359951970-session.l2.jsonl
20260729-095549.359951970-session.l3.jsonl
```

### 3. RefRecord 包含 Role
```bash
$ head -1 *.refs.jsonl | jq '.'
{
  "step_id": 2,
  "level": 0,
  "role": "user",  # 新增
  "ref_count": 0,
  "strength": 1,
  ...
}
```

### 4. Step ID 无冲突
```bash
$ cat *.refs.jsonl | jq -c '{step_id}' | sort | uniq -d
# 无输出（无重复）
```

### 5. 子 Agent 独立 Context
```bash
$ ls ~/.reasonix/sessions/context/
20260729-095549.359951970-session.refs.jsonl
20260729-095549.359951970-session_sub_sa_20260729_102208.refs.jsonl  # 子 agent
```

## 实施优先级

| 优先级 | Phase | 影响 | 工作量 |
|--------|-------|------|--------|
| P0 | 1. RefRecord 补全 Role | 高（压缩后丢失角色） | 小 |
| P0 | 4. Session JSONL 持久化 | 高（日志完整性） | 中 |
| P1 | 2. Context Manager 按 Session 分文件 | 高（多 session 隔离） | 中 |
| P1 | 5. Step ID 全局唯一 | 高（避免冲突） | 小 |
| P2 | 3. 子 Agent 独立 Context | 中（子 agent 压缩） | 大 |
| P2 | 6. 双写桥接同步 | 中（ID 同步） | 小 |

## 相关文件

- `internal/contextmgr/contextmgr.go` — RefRecord, Ingest, loadRefs
- `internal/contextmgr/store.go` — FileStore, AppendRef, LoadRefs
- `internal/contextmgr/tools.go` — SearchTool, ExpandTool, ReorganizeTool (SetManager 延迟绑定)
- `internal/agent/compact.go` — addAndIngest, ContextStep
- `internal/agent/task.go` — subagentOptions, RunSubAgentWithSession
- `internal/boot/boot.go` — contextmgr.New 初始化（延迟到 OnSessionPathChange 回调）
- `internal/control/controller.go` — OnSessionPathChange 回调机制
- `internal/agent/session.go` — Session 结构

## 备注

当前阶段（maybeSliceCompact 实现）先保证核心逻辑跑通，日志结构双写完整性作为后续改进项。修复时需要大改，涉及多个模块的协调。

## 修复实施记录（2026-07-29）

### 已完成 Phase

| Phase | 描述 | 状态 | 改动文件 |
|-------|------|------|----------|
| 1 | RefRecord 补全 Role 字段 | ✅ | `contextmgr.go` |
| 2 | Context Manager 按 Session 分文件 | ✅ | `boot.go`, `controller.go`, `tools.go` |
| 4 | Session JSONL 持久化 | ⏭️ 取消 | — （events.jsonl 是权威记录，session.jsonl 是兼容性锚点） |
| 5 | Step ID 全局唯一 | ✅ | `compact.go` |
| 6 | 双写桥接同步 | ✅ | `compact.go` |

### 未实施 Phase

| Phase | 描述 | 原因 |
|-------|------|------|
| 3 | 子 Agent 独立 Context | P2 优先级，暂不实施。子 agent 可通过共享主 agent 的 contextmgr + TaskGroupID 区分 |

### 关键改动

1. **RefRecord.Role**：`Ingest()` 时从 `StepRecord.Role` 复制到 `RefRecord.Role`
2. **延迟初始化**：`OnSessionPathChange` 回调 → `initContextMgr(stem)` → contextmgr session ID 对齐原生 session 文件
3. **StepID 自动分配**：`addAndIngest()` 传 `StepID: 0`，contextmgr 用 `nextStep` 自动分配

---

## 预期观察点

### 1. Context Manager 文件结构

**预期**：每个 session 生成独立的 context 文件

```bash
$ ls ~/.reasonix/sessions/context/
20260729-095549.359951970-session.refs.jsonl
20260729-095549.359951970-session.l1.jsonl
20260729-095549.359951970-session.l2.jsonl
20260729-095549.359951970-session.l3.jsonl
```

**验证命令**：
```bash
# 检查 context 文件是否按 session 分文件
ls ~/.reasonix/sessions/context/*.refs.jsonl

# 检查文件名是否与 session 文件 stem 一致
ls ~/.reasonix/projects/.../sessions/*.jsonl | xargs -I{} basename {} .jsonl
```

### 2. RefRecord 包含 Role 字段

**预期**：refs.jsonl 每条记录包含 `role` 字段

```bash
$ head -3 ~/.reasonix/sessions/context/*.refs.jsonl | jq '.'
{
  "step_id": 1,
  "level": 0,
  "role": "user",        # 新增字段
  "ref_count": 0,
  "strength": 1.0,
  ...
}
{
  "step_id": 2,
  "level": 0,
  "role": "assistant",   # 新增字段
  "ref_count": 0,
  "strength": 1.0,
  ...
}
```

**验证命令**：
```bash
# 检查 role 字段是否存在且非空
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '{step_id, role}' | head -10

# 统计各 role 的 step 数量
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -r '.role' | sort | uniq -c
```

### 3. Step ID 无冲突

**预期**：同一 session 的 refs.jsonl 中 step_id 唯一递增

```bash
$ cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '{step_id}'
{"step_id":1}
{"step_id":2}
{"step_id":3}
...
{"step_id":19}
{"step_id":20}  # 无重复，连续递增
```

**验证命令**：
```bash
# 检查是否有重复 step_id
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '.step_id' | sort | uniq -d

# 检查 step_id 是否从 1 开始连续
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '.step_id' | sort -n | head -5
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '.step_id' | sort -n | tail -5
```

### 4. Context Tools 延迟绑定

**预期**：context_search / context_expand / context_reorganize 工具在 session path 设置后可用

**验证方法**：
1. 启动 reasonix-ctx，开始新会话
2. 发送几条消息
3. 调用 `context_search` 工具，应返回搜索结果
4. 检查日志：`context manager initialized session_id=20260729-...`

**日志观察点**：
```bash
# 检查 contextmgr 初始化日志
grep "context manager initialized" ~/.reasonix/logs/*.log | tail -5

# 检查 session path 变更日志
grep "OnSessionPathChange" ~/.reasonix/logs/*.log | tail -5
```

### 5. 双写桥接同步

**预期**：session messages 数量 = contextmgr steps 数量

**验证方法**：
```bash
# 统计 session events 数量
wc -l ~/.reasonix/projects/.../sessions/*.events.jsonl

# 统计 contextmgr steps 数量
wc -l ~/.reasonix/sessions/context/*.refs.jsonl

# 两者应大致相等（允许少量差异：system prompt 不 ingest，local-only 消息不 ingest）
```

### 6. 压缩后 Role 保留

**预期**：L1/L2/L3 压缩后，BuildContext 返回的 blocks 仍包含 role

**验证方法**：
1. 触发压缩（context 超过甜点区）
2. 调用 `context_search` 或 `context_expand`
3. 检查结果中是否包含 `§N/user`、`§N/assistant` 等标记

**日志观察点**：
```bash
# 检查压缩日志
grep "compressed step" ~/.reasonix/logs/*.log | tail -10

# 检查 BuildContext 返回的 blocks
grep "BuildContext" ~/.reasonix/logs/*.log | tail -5
```

### 7. 子 Agent Context（未实施）

**当前状态**：子 agent 不创建独立 contextmgr，steps 不进入分级压缩

**预期行为**（未来实施后）：
```bash
# 子 agent 应有独立的 context 文件
ls ~/.reasonix/sessions/context/*_sub_*.refs.jsonl
```

**当前验证**：
```bash
# 确认子 agent 没有独立 context 文件
ls ~/.reasonix/sessions/context/ | grep sub
# 应无输出
```

---

## 故障排查

### 问题 1：context 文件未生成

**可能原因**：
- `OnSessionPathChange` 回调未触发
- contextmgr 初始化失败

**排查步骤**：
```bash
# 检查日志
grep "context manager" ~/.reasonix/logs/*.log | tail -20

# 检查 session path 是否设置
grep "setSessionPath" ~/.reasonix/logs/*.log | tail -5
```

### 问题 2：RefRecord.Role 为空

**可能原因**：
- 旧数据未迁移
- `Ingest()` 时 `StepRecord.Role` 为空

**排查步骤**：
```bash
# 检查 refs.jsonl 中 role 字段
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '{step_id, role}' | head -10

# 检查 addAndIngest 是否传递 Role
grep "addAndIngest" ~/.reasonix/logs/*.log | tail -5
```

### 问题 3：Step ID 冲突

**可能原因**：
- `addAndIngest` 仍传递非零 StepID
- `loadRefs()` 未正确恢复 `nextStep`

**排查步骤**：
```bash
# 检查 step_id 是否有重复
cat ~/.reasonix/sessions/context/*.refs.jsonl | jq -c '.step_id' | sort | uniq -d

# 检查 loadRefs 日志
grep "loadRefs" ~/.reasonix/logs/*.log | tail -5
```

### 问题 4：Context Tools 不可用

**可能原因**：
- `SetManager()` 未调用
- contextmgr 初始化失败

**排查步骤**：
```bash
# 检查工具注册日志
grep "context_search\|context_expand\|context_reorganize" ~/.reasonix/logs/*.log | tail -10

# 检查 SetManager 调用
grep "SetManager" ~/.reasonix/logs/*.log | tail -5
```
