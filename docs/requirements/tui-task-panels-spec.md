# 规格：InferGlow TUI 任务管理面板（分块布局）调研与设计

> **状态**：独立 Spec（调研 + 设计参考稿；不写实现代码）
> **关联**：代码/命令兼容计划见 `docs/plans/2026-08-24-slash-command-compat-autocomplete.md`（其 §12 反向引用本 spec）。
> 本 spec 聚焦「任务管理型 TUI 界面分块」，回答：codex / opencode 等是否具备任务管理分块面板，以及
> inferglow 如何借鉴。

---

## 1. Goal（目标）

调研并定义 InferGlow TUI 的**任务管理面板（分块 / Panels）**形态：
- 回答「codex / opencode 等是否有任务管理的 TUI 分块界面」→ **有**（见 §2 证据）。
- 以「主对话区 + 底部输入舱 + 侧边/覆盖层任务面板」为骨架，为 inferglow 规划一份可借鉴的分块设计。
- 与斜杠命令兼容计划解耦，本 spec 只定义「任务/会话/目标是按块组织的」这一横切面。

---

## 2. 证据：codex / opencode / claude 的任务管理分块

### 2.1 codex（读自工作区源码 `codex/codex-rs/tui/`）—— 功能最全，分层清晰

| 分块 | 模块（源码路径） | 说明 |
|---|---|---|
| 主对话区 | `chatwidget.rs`（+ `chatwidget/`）、`thread_transcript.rs`、`markdown_stream.rs` | transcript + 底部 composer |
| 底部输入舱（含弹窗） | `bottom_pane/`（`chat_composer.rs`、`command_popup.rs`、`skill_popup.rs`、`file_search.rs`、`approval_overlay.rs`、`list_selection_view.rs`、`multi_select_picker.rs`、`hooks_browser_view.rs`、`memories_settings_view.rs`、`experimental_features_view.rs`、`skills_toggle_view.rs`、`footer.rs`） | `/`命令弹窗、技能、文件搜索、审批、多选、hooks/memories 浏览——**均为在 composer 上方滑出的次级面板** |
| 侧边线程/Agent | `app/side.rs`（`SideParentStatus`、`SideThreadState`） | 侧边线程与父状态 |
| **任务/多 Agent 总览（左右分栏）** | `app/agents_overview_view.rs` | `Layout::horizontal([Min(46), Length(3), Length(38)])` → **左=agent/任务列表，右=详情**；是典型「任务管理 2 列分块」 |
| **任务目标面板** | `goal_display.rs`、`goal_files.rs`、`app_command.rs`（`/goal`） | `/goal` 渲染 objective / status(BudgetLimited, Paused…) / elapsed / token 预算使用（`goal_usage_summary`），并支持 clear/edit/pause/resume |
| 会话/线程管理 | `resume_picker.rs`（header/search/list/footer 4 块纵排）、`session_archive_commands.rs`、`session_queue_commands.rs`、`session_state.rs`、`session_start.rs`、`session_resume.rs`、`app_server_session.rs`、`named_session_lookup.rs` | 恢复/存档/删除/排队会话的 4 区块选择器 |
| 覆盖层/浮层 | `pager_overlay/`、`notifications/`、`pets/`、`onboarding/`、`resume_picker/` | 分页、通知、桌宠、首次引导、恢复选择器 |
| 状态行 | `status/`、`public_widgets/`、`status_indicator_widget.rs`、`token_usage.rs` | 底部状态与 token 用量 |
| 渲染/键盘/主题 | `render/`、`keymap/`、`theme_picker.rs`、`markdown_render.rs`、`diff_render.rs`、`inline_visualization.rs` | 渲染管线、键位、主题 |

**结论**：codex TUI 是**多区块面板化**结构（主对话 + 底部舱弹窗 + 侧边 + 任务/Agent 左右分栏 + 目标面板 + 会话管理器 + 覆盖层）。任务管理分块**明确存在**并已产品化。

### 2.2 opencode（`opencode.ai/docs/tui` + `opencode-primer` 命令表 + 用户截图）
- TUI 采用 **Tab 化**主视图（聊天 / Agent / Plan 等）与底部命令 **popup**（输入 `/` 弹出，Tab 补全）。
- 会话相关命令 `/new, /sessions(/resume,/continue), /compact(/summarize), /export, /share, /undo, /redo`。
- **Todo 侧边栏**（用户截图证据）：右侧常驻任务列表面板，格式如下：
  ```
  ┌────────────────────┬──────────────┐
  │                    │  Todo        │
  │   主对话区          │  ──────────  │
  │                    │  [✓] 任务1   │
  │                    │  [*] 任务2   │
  │                    │  [ ] 任务3   │
  │                    │  [ ] 任务4   │
  └────────────────────┴──────────────┘
  ```
  - 状态图标：`[✓]` 已完成、`[*]` 进行中、`[ ]` 待处理
  - 任务由 AI 通过 `todowrite` 工具更新
  - 面板标题可自定义（如「OpenCode内置待办任务板」）
- **Message Actions 菜单**（用户截图证据）：点击历史用户输入弹出上下文菜单：
  ```
  ┌─────────────────────────────────┐
  │  Message Actions           esc  │
  │  ─────────────────────────────  │
  │  Search                         │
  │  Revert  undo messages and file │
  │          changes                │
  │  Copy    message text to clip   │
  │  Fork    create a new session   │
  └─────────────────────────────────┘
  ```
  - 触发：点击历史用户消息（非 AI 回复）
  - 菜单项：Search（搜索）、Revert（撤销）、Copy（复制）、Fork（分支）
  - 快捷键：Esc 关闭菜单
- 侧重「会话切换 + 会话压缩」分块，任务面板粒度低于 codex 的 goal/agents，但 Todo 侧边栏是轻量任务管理的典型实现。

### 2.3 claude（官方内置斜杠命令 + `/tasks`, `/plan`, `/agents`, `/context`）
- `/tasks` 列后台任务，`/plan` 进入计划模式，`/context` 可视化上下文占用（彩色 grid），`/status`。
- 偏「命令式」触发，未内置 codex 式的常驻多列任务面板。

### 2.4 对比小结（供 inferglow 取舍）

| 维度 | codex | opencode | claude |
|---|---|---|---|
| 任务面板常驻分块 | ✅ 强（agents 左右分栏 + goal 面板 + 会话管理） | ✅ Todo 侧边栏（轻量任务列表） | ⚠️ 命令式 |
| 命令弹窗（`/`联想） | ✅ 强（command_popup + 前缀） | ✅ 有 | ✅ 有 |
| 消息操作菜单 | ⚠️ 部分（通过 checkpoint） | ✅ Revert/Copy/Fork | ⚠️ 部分 |
| 工作空间/目录切换 | ✅ `/cd` + 状态栏显示 | ✅ `project.workspace` + `WorkspaceLabel` | ⚠️ 有限 |
| 覆盖层（审批/通知/分页） | ✅ 多 | 部分 | 部分 |
| 状态行含 token/成本 | ✅ | ✅ | ✅ |

---

## 3. 为 InferGlow 的分块设计（方案）

### 3.1 分块骨架（由现状演进）
inferglow 现状 `View()` 已具备「transcript 视口 + 底部（working line / 状态栏 / 输入框）」两区。
按 codex 思路演进为**弹窗/面板化分块**：

```
┌───────────────────────────────┐
│   transcript 视口（主对话）       │
├───────────────────────────────┤   ← (A) 任务/目标面板（可切换）
│   [working spinner / goal]     │
│   [status bar]                 │
│   [completion popup]           │   ← (B) / 命令联想弹窗（slash-compat 计划）
│   [ input box ]                │
└───────────────────────────────┘
(侧边/覆盖层用于 agents 总览、会话管理、审批、通知)
```

### 3.2 panelless 增强点（每次改动单一职责）
| 面板 | 触发 | 数据来源 | 借鉴 |
|---|---|---|---|
| `/goal` 任务目标条 | 会话启动或 `/goal`（如已有 goal） | `orchestrator/agent`（goal/plan 能力） | codex `goal_display` 的 objective/status/token |
| `/tasks` 任务列表面板 | `/tasks` 或 `T5 任务进度`（tui_model.go 已有 T5 钩子） | 内置 TaskTracker（`builtins/.../task_tracker.go`） | codex agents_overview 左右分栏（列表+详情）；OpenCode Todo 侧边栏 |
| `/workspace` 工作空间切换 | `/workspace` 或 `/cd` | `os.Getwd()` + 历史记录 | OpenCode `project.workspace` + `WorkspaceLabel` |
| 会话切换器 | `/resume`, `/sessions` | `~/.inferglow/sessions/index.jsonl`（已有 `session_index.go`、`resume` 处理） | codex `resume_picker` 四区（header/search/list/footer） |
| 命令联想弹窗 | `/` 输入 | `cli/tui_cmd_registry.go` 目录 | codex `command_popup` + 前缀（见 slash-compat 计划） |
| 消息操作菜单 | 点击历史用户输入 | transcript 消息列表 | OpenCode Message Actions（Revert/Copy/Fork） |
| 覆盖层（审批/选择） | 工具审批、多选 | `action/approval`、`builtins` approval | codex `approval_overlay`、`list_selection_view` |

### 3.3 推荐实现最小集（优先级）
- **P0**：(B) `/` 命令联想弹窗（与 slash-compat 计划同批落地）。
- **P0**：消息操作菜单（Revert/Copy/Fork），借鉴 OpenCode Message Actions。
- **P1**：`/tasks` 只读任务面板（复用 TaskTracker + T5 钩子；先做列表，后做详情分栏）；借鉴 OpenCode Todo 侧边栏。
- **P1**：`/workspace` 工作空间目录切换（目录切换 + 历史记录 + 状态栏显示）；借鉴 OpenCode `project.workspace`。
- **P2**：会话切换器（复用 `session_index`）；`/goal` 目标条（若 orchestrator 已暴露 goal）。

---

## 4. 原则 / 兼容边界

- **单块单职责**：每个面板独立文件/状态，互不耦合；沿用 codex「每次改动单一职责」。
- **不破坏现有转录**：面板只在输入区上方/覆盖层出现，不挤占 transcript 语义。
- **可开关**：面板可按需启用（对齐 slash-compat 计划的 `features.slash_popup`，新增 `features.task_panel` 类开关）。
- **数据不重复**：任务/会话数据仍以 `session`、`builtins/task_tracker`、`session_index` 为事实源，面板只读渲染。

---

## 5. Acceptance Criteria（验收）

- [ ] 能明确回答「opencode/codex 是否有任务管理分块」——本 spec 已给出证据（§2）。
- [ ] 每个新增面板可独立开关、独立渲染、不与 transcript/输入框布局冲突（高度计入 `bottomRows/transcriptHeight`）。
- [ ] 任务/会话数据来源不变，面板仅为视图层。
- [ ] 与 slash 命令兼容计划解耦，可单独拆分实现。

---

## 6. Roadmap / 归属

- **代码**：全部落在 `cli` TUI（`tui_*.go`），不触 `codex/`。
- **关联计划链路**：`docs/plans/2026-08-24-slash-command-compat-autocomplete.md` 已实现 `/` 联想；
  本 spec 定义其上的**任务/会话/目标面板**（P1/P2）。
- **实现**：待用户确认后按 P0→P2 粘合实现，每面板一个独立 commit。

---

## 7. 参考资料（证据链接）
- Codex TUI 源码：`codex/codex-rs/tui/`（`app.rs`、`chatwidget.rs`、`bottom_pane/`、`app/agents_overview_view.rs`、`goal_display.rs`、`resume_picker.rs`、`app/side.rs`）
- OpenCode TUI：https://opencode.ai/docs/tui（及 `opencode-primer` 命令表）
- Claude Code 内置命令：https://docs.anthropic.com/en/docs/claude-code/overview
- **代码参考仓库**：
  - `anomalyco/opencode` — OpenCode TUI 源码（`packages/tui/src/`）
  - `huiliyi37/dsh-tianshu-tui` — dsh-tianshu-tui 任务面板实现（`src/format/task-panel.ts`）
  - `huiliyi37/Tianshu-harness` — Tianshu harness 框架
  - `ccch1mneyyy/dsh-TUI` — dsh-TUI 组件实现
