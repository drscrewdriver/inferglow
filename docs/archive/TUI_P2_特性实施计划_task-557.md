# InferGlow TUI P2 特性实施计划

## 总览

10 项待实施特性按依赖深度分为 4 层，每层可独立部署。预估新增 ~800 行代码（6 个新文件）+ ~200 行修改（8 个现有文件）。

---

## Layer 0: 基础层（事件系统 + Transcript 模型扩展）

### Step 0.1: 扩展 EventKind 和 AgentEvent
**文件**: `orchestrator/agent/event_sink.go`
- 新增 `EventApproval EventKind`（工具被审批阻塞）
- 新增 `EventCompression EventKind`（上下文压缩发生）
- `AgentEvent` 新增字段:
  - `Status string` — 传递 ActionResult.Status（"blocked"/"success"/"error"）
  - `SandboxMode string` — 沙箱模式
  - `SideEffectLevel string` — 副作用级别
  - `ApprovalRecordID string` — 审批记录 ID
  - `Metadata map[string]string` — 通用扩展字段
- `CallbacksFromSink()` 映射新事件

### Step 0.2: 新增 Callback 钩子
**文件**: `orchestrator/agent/callbacks.go`
- 新增 `OnApprovalRequired func(ctx, toolName, recordID string)`
- 新增 `OnCompression func(ctx, stepsCompressed int)`
- 对应 `fireOnApprovalRequired()` / `fireOnCompression()` 辅助函数

### Step 0.3: Engine 传播审批/压缩/元数据事件
**文件**: `orchestrator/agent/engine.go`
- **关键修改点** `executeLoop()` 第 762-768 行:
  - `ActionResult.Status == "blocked"` 时，调用 `fireOnApprovalRequired` 而非仅设 toolErr
  - 从 `Error` 字段解析 recordID（`"pending approval: <id>"`）
- `fireOnToolCallStart` 前，从 ActionRegistry 查找 ActionSpec，传递 SideEffectLevel/SandboxRequired
- compactHook 后，调用 `fireOnCompression`
- **注意**: `Action` 运行时结构体不含 Spec 字段，需通过 `registry.Get(name)` 获取 `*Action` 再查 `ActionSpec`（如果 Action 无 Spec 字段则用 Metadata 传递）

### Step 0.4: Transcript 双轨模型
**文件**: `cli/tui_view.go`, `cli/tui_model.go`
- 定义 `transcriptBlock` 结构体:
  ```go
  type blockKind int
  const (blockText blockKind = iota; blockUser; blockAssistant; blockTool; blockError; blockApproval; blockReceipt; blockSystem)
  type transcriptBlock struct {
      Kind   blockKind
      Raw    string  // 渲染后的 ANSI 字符串（当前行为）
      Source string  // 纯文本源（用于复制/搜索/reflow）
  }
  ```
- `transcript []string` → `transcript []transcriptBlock`
- `commitLine()` 改为带 kind 参数的内部方法，保留公开 API 兼容
- `renderTranscript()` 遍历 `.Raw` 字段
- 新增 `commitApprovalCard()`, `commitReceipt()`, `commitSystemNote()` 等类型化方法

### Step 0.5: 新增主题样式
**文件**: `cli/tui_theme.go`
- 新增: `approvalText()`, `sandboxBadge()`, `sideEffectBadge()`, `compressionNote()`, `receiptDim()`
- 使用现有 palette 颜色（warn=琥珀色 for approval, info=青色 for sandbox）

---

## Layer 1: 安全交互（Approval UI + Sandbox 显示）

### Step 1.1: Approval 审批 UI
**新文件**: `cli/tui_approval.go` (~150 行)
- `approvalCard` 结构体: recordID, toolName, params, resolved, approved
- `chatTUI` 新增: `pendingApproval *approvalCard`
- `ingestEvent()` 处理 `EventApproval`: 创建卡片，渲染审批卡片到 transcript
- 键盘交互（`pendingApproval != nil` 时）:
  - `y` → approve → `approvalManager.ResolveRecord(id, true, "tui-user")`
  - `n` → deny → `ResolveRecord(id, false, "tui-user")`
  - `Escape` → dismiss
- 审批卡片渲染:
  ```
  ┌─ ⚠ Approval Required ─────────────────┐
  │ Tool: bash_execute                     │
  │ [Y] Approve  [N] Deny                  │
  └────────────────────────────────────────┘
  ```

### Step 1.2: 将 ApprovalManager 接入 TUI
**文件**: `cli/tui_model.go` (RunTUI), `cli/agent_factory.go`
- `buildAgent` 当前不构造 `PolicyApprovalManager`（使用 localBashRunner）
- 方案: `RunTUI()` 中构造 `approval.NewPolicyApprovalManager()`，注册 `AutoApproveHandler` 或 `FailClosedHandler`，传给 `chatTUI`
- `chatTUI` 新增 `approvalMgr *approval.PolicyApprovalManager` 字段
- 当 `buildAgent` 后续切换到 `SandboxExecutor` 时，共享同一个 manager

### Step 1.3: Sandbox ActiveType 显示
**文件**: `cli/tui_view.go` (commitToolCard), `cli/tui_model.go` (ingestEvent)
- `commitToolCard` 签名扩展: `(name, status, sandboxMode, sideEffect, output string)`
- 渲染格式:
  ```
  ⎿ [bash_execute] running  │ docker·exec
  ✓ [file_read] done        │ trusted·read
  ✗ [bash_execute] blocked  │ docker·exec  [Y/N]
  ```
- Sandbox badge 颜色: Docker=info, Trusted=dim, none=不显示
- SideEffect badge 颜色: read=success, write=warn, exec=error, network=info, none=dim

---

## Layer 2: 可观测性（压缩可视化 + Turn Receipt + 滚动）

### Step 2.1: 上下文压缩可视化
**文件**: `cli/tui_model.go` (ingestEvent), `cli/tui_footer.go`
- 处理 `EventCompression`: 提交压缩提示行 `⟳ compressed N steps (saved ~Xk tokens)`
- Footer `statusModelGroup()` 增强:
  - 显示 `L0:12 L1:5 L2:2` 级别分布（数据来自 `bridge.Stats().LevelCounts`）
  - `WindowPressure > 0.7` 时显示警告色

### Step 2.2: Turn Receipt
**新文件**: `cli/tui_receipt.go` (~120 行)
- `turnReceipt` 结构体: turnNum, duration, llmRounds, promptTokens, completionTokens, toolCalls
- `chatTUI` 新增: `receipt turnReceipt`, `sessionTokensIn/Out int`
- `ingestEvent()` 累积: `EventLLMEnd` → tokens, `EventToolStart` → toolCalls, `EventRunStart/End` → timing
- `EventRunEnd` 时提交 receipt 行:
  ```
  ─── Turn #3 · 12s · 3 rounds · ↓4.2k ↑1.8k · 5 tools ───
  ```
- `/receipt` 命令切换显示

### Step 2.3: Native Scrollback 模式
**新文件**: `cli/tui_scrollback.go` (~100 行)
- `Ctrl+S` 进入滚动模式（vim-like）
- 滚动模式下: j/k 逐行, Ctrl+U/D 半页, g/G 首尾, q/Esc 退出
- 状态栏显示 `Scrollback (42/128)`
- 复用现有 viewport 滚动能力，仅增加模式切换和键绑定

---

## Layer 3: 质量打磨（Session + 选择 + i18n + Git + 提交）

### Step 3.1: Session/Resume 系统
**新文件**: `cli/tui_session.go` (~100 行)
- `/session` 命令: 显示当前 session ID
- `/resume <id>` 命令: 切换到指定 session
- 无参数 `/resume`: 显示最近 session 列表（扫描 `{dataDir}/sessions/*.jsonl`）
- Session 持久化已通过 JSONL store 实现，只需 UI 层接入
- 选择后: 重建 MemoryBridge → 重建 Agent → 从 store 回放 transcript

### Step 3.2: 文本选择/复制
**新文件**: `cli/tui_selection.go` (~130 行)
- 鼠标拖拽选择 transcript 行（已有 `MouseModeCellMotion`）
- `v` 键进入视觉选择模式
- 复制机制: OSC 52 escape sequence（跨 SSH 兼容），fallback xclip/xsel
- 使用 `transcriptBlock.Source` 提取纯文本（stripAnsi）
- 选中区域反色显示（使用已有 `selection` palette 颜色）

### Step 3.3: i18n / Git status / Balance / Queue
**文件**: `cli/tui_footer.go`, `cli/tui_model.go`
- **Git status**: 异步执行 `git status --porcelain`（5s 缓存），状态栏显示分支 + dirty 标记
- **Balance**: 从 provider API 获取余额（如支持），footer 右侧显示
- **Queue**: Agent 已有 `InputQueue` + `SubmitInput`，TUI 显示排队深度 `thinking... [2 queued]`
- **i18n**: 定义 `tuiStrings` 结构体，先实现 en，结构支持 zh-CN 扩展

### Step 3.4: Git Commit
- 分 4 次提交（每层一次）:
  1. Layer 0: 事件系统 + transcript 模型
  2. Layer 1: Approval UI + Sandbox 显示
  3. Layer 2: 压缩可视化 + Receipt + Scrollback
  4. Layer 3: Session + Selection + 打磨
- **首先**: 将当前未提交的 10 个文件作为 checkpoint 提交

---

## 依赖关系

```
Step 0.1 (EventKind) ──┬──→ 0.2 (Callbacks) ──→ 0.3 (Engine) ──┬──→ 1.1 (Approval UI)
                       │                                         └──→ 1.3 (Sandbox display)
Step 0.4 (Transcript) ─┼──→ 1.1, 1.3, 2.1, 2.2, 2.3, 3.2
Step 0.5 (Theme) ──────└──→ 1.1, 1.3

2.1 (Compression) ── 独立（使用现有 Stats()）
2.2 (Turn Receipt) ── 独立（使用现有 EventLLMEnd）
3.1 (Session) ── 独立（JSONL store 已存在）
3.3 (i18n/Git/Queue) ── 独立
```

**推荐实施顺序**: 0.1 → 0.4 → 0.5 → 0.2 → 0.3 → 1.3 → 1.1 → 1.2 → 2.1 → 2.2 → 2.3 → 3.1 → 3.2 → 3.3 → 3.4

---

## 风险与缓解

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| 修改 event_sink.go 影响其他消费者(server/desktop) | 高 | 纯增量变更（新 EventKind 追加到 iota 末尾，新字段零值安全）。现有 switch 有 default case |
| Transcript `[]string` → `[]transcriptBlock` 重构侵入性大 | 中 | Raw 字段保持当前渲染行为。commitLine 保留为 wrapper。~15 个调用点需更新 |
| Approval UI 阻塞 agent goroutine | 高 | 审批流程本身是异步的: SandboxExecutor 立即返回 blocked，agent loop 继续。TUI 只负责显示和 ResolveRecord |
| ApprovalManager 当前未在 TUI 路径构造 | 中 | RunTUI 中显式构造 manager。当 buildAgent 未使用 SandboxExecutor 时，approval 功能降级为只读显示 |
| OSC 52 剪贴板非所有终端支持 | 低 | 检测 TERM_PROGRAM，fallback xclip/xsel/pbcopy |

---

## 被否决的替代方案

1. **Plan C 的 error-string matching 方案**: 在 TUI 层通过 `strings.Contains(e.Err.Error(), "pending approval:")` 检测 blocked 状态。否决原因: 脆弱（依赖 error 文本格式），且无法传递 recordID 给 ResolveRecord。选择 Step 0.3 的 engine 层直接传播。

2. **Plan B 的 transcriptSources 平行 slice 方案**: 保留 `[]string` transcript 同时新增 `[]transcriptSource` 平行 slice。否决原因: 两个 slice 需严格同步，容易出 bug。选择 Plan A 的 `transcriptBlock` 结构体方案，一次重构解决。

3. **独立 approval channel 方案** (Plan B Step 3): 新建 `approvalCh chan approvalDecision` 做 TUI→agent 通信。否决原因: 过度复杂。直接调用 `approvalManager.ResolveRecord()` 更简单，manager 本身是线程安全的。

---

## 关键文件清单

| 文件 | 变更类型 | 影响 |
|------|----------|------|
| `orchestrator/agent/event_sink.go` | 修改 +40 行 | 所有 Layer 1/2 依赖 |
| `orchestrator/agent/callbacks.go` | 修改 +15 行 | 新 callback 钩子 |
| `orchestrator/agent/engine.go` | 修改 +30 行 | 事件传播核心 |
| `cli/tui_model.go` | 修改 +50 行 | 中央协调 |
| `cli/tui_view.go` | 修改 +60 行 | Transcript 模型重构 |
| `cli/tui_approval.go` | **新建** ~150 行 | 审批交互卡片 |
| `cli/tui_receipt.go` | **新建** ~120 行 | Turn 明细 |
| `cli/tui_scrollback.go` | **新建** ~100 行 | 滚动模式 |
| `cli/tui_selection.go` | **新建** ~130 行 | 文本选择 |
| `cli/tui_session.go` | **新建** ~100 行 | Session 管理 |
| `cli/tui_theme.go` | 修改 +20 行 | 新样式 |
| `cli/tui_footer.go` | 修改 +30 行 | 压缩/队列/Git 指示器 |
| `cli/tui_commands.go` | 修改 +20 行 | /receipt, /session, /resume |
