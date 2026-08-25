# 计划：InferGlow TUI 斜杠命令兼容（claude/pi/opencode/codex）+ `/` 前缀联想（输入法式）

> 状态：**计划稿（规划 + 收集规格证据，尚未写实现代码）**
> 目标：让 inferglow TUI 的命令系统兼容 Claude Code / pi(pi-coding-agent) / OpenCode / Codex 的常用斜杠命令，
> 并在输入框输入 `/` 时提供**输入法式实时前缀联想**（候选弹窗 + 上/下选择 + Tab 补全 + Enter 触发 + Esc 关闭）。

---

## 1. Goal（目标）

1. **命令兼容**：`SlashRegistry` 通过「别名 + 兼容命令」目录，接受 claude/pi/opencode/codex 的常用斜杠命令，
   映射到 inferglow 既有 handler；无法映射的（如 `/vim`、`/pets`）注册为「已识别但未实现」并给出友好提示。
2. **`/` 前缀联想**：输入框以 `/` 开头且在首行光标处编辑命令名时，实时在输入框上方弹出候选列表；
   ↑/↓ 选择、Tab 补全（唯一时直接补全）、Enter 触发所选、Esc 关闭。类似 codex 的 `command_popup`。
3. **任务面板（Todo Panel）**：右侧常驻或可切换的任务列表面板，展示当前会话的任务状态：
   `[✓]` 已完成、`[*]` 进行中、`[ ]` 待处理。任务数据来自 TaskTracker，面板为只读视图。
4. **历史消息操作菜单**：点击历史用户输入时弹出上下文菜单，提供 `Revert`（撤销消息和文件变更）、
   `Copy`（复制消息文本）、`Fork`（创建新会话分支）等操作。
5. **工作空间目录切换**：提供 `/workspace` 或 `/cd` 命令，支持切换当前工作目录，
   并在状态栏显示当前工作目录路径。支持目录补全和历史记录。
6. **可控**：新增配置开关 `features.slash_compat`（默认 on）、`features.slash_popup`（默认 on）、
   `features.task_panel`（默认 on）、`features.message_actions`（默认 on）、
   `features.workspace_switch`（默认 on），可关闭。

### 非目标（Non-goals）
- 不改动 codex 源码（`codex/` 仅作命令集参考）。
- 不实现 `/vim`、`/pets`、`/theme` 等纯外围命令的真实行为，仅识别并提示。
- 不引入 `@文件`、`!bash` 前缀（属于另一功能面）。
- 不做自定义命令/技能的热加载（当前 `/skill:*` 仅识别）。

---

## 2. Architecture（设计）

### 2.1 命令模型
扩展 `cli/tui_cmd_registry.go` 的 `SlashCommand`，增加 `Source`（来源标签）与「已识别但未实现」语义。

```go
type SlashCommand struct {
    Name        string
    Aliases     []string            // 兼容别名（含别家命令）
    Description string
    Usage       string
    Source      string              // "inferglow" | "claude" | "pi" | "opencode" | "codex" | "compat"
    Implemented bool                // false = 识别但未实现（友好提示）
    Handler     func(m *chatTUI, args string) (tea.Cmd, bool)
}
```

`SlashRegistry` 增加：

```go
// Suggest 返回匹配 prefix 的候选（前缀优先，其次子序列模糊，去重别名，limit 截断）。
func (r *SlashRegistry) Suggest(prefix string, limit int) []*SlashCommand
```

- 匹配算法：`prefix=""` → 返回全部（按 `DefaultOrder`，由注册顺序决定）前 limit 个；
  `prefix != ""` → **前缀匹配优先**（`strings.HasPrefix(name, prefix)`，别名也算），
  其次 **子序列模糊匹配**（`isSubsequence(prefix, name)`，如 `/m`→model/memory/mcp，`/bt`→btw）。
  按 `name` 长度升序、字母序排序。
- 去重：一个 `SlashCommand` 出现多次时（多个别名命中）只保留一次。

### 2.2 兼容命令目录（数据）
内置一张 `compatCatalog`（`cli/tui_cmd_registry.go` 或新文件 `cli/tui_compat.go`），
每项 `{name, aliases, description, source, implemented:false}`。`buildSlashRegistry` 在注册 inferglow
原生命令后，遍历目录注册兼容项（不 `panic` 冲突，已存在的原生命令跳过）。

**关键映射建议（可落地到 inferglow 既有 handler 的用 `Implemented:true` + handler）：**

| 别家命令 | 映射到 inferglow | Source | 说明 |
|---|---|---|---|
| `/clear` `/reset` `/new`(claude/pi/opencode) | → 原生 `/clear` | compat | clear 转录 |
| `/compact` `/summarize`(opencode) | → 原生 `/compact` | compat | 压缩上下文 |
| `/resume` `/continue`(claude/opencode) `/sessions`(opencode) | → 原生 `/resume` | compat | 恢复会话 |
| `/model` `/models`(opencode) `/scoped-models`(pi) | → 新增 `/model` handler（见 P1） | compat | 切换模型 |
| `/memory`(claude) | → 原生 `/memory` | compat | 记忆 |
| `/quit` `/exit` `/q`(opencode) `/logout`(pi/codex) | → 原生 `/quit` | compat | 退出 |
| `/config` `/settings`(pi/opencode) | → 原生 `/config` | compat | 配置 |
| `/session`(pi/codex) `/title`(codex) | → 原生 `/session` | compat | 会话信息/标题 |
| `/status`(claude/codex) `/usage`(opencode/pi) `/cost`(claude) `/receipt` | → 原生 `/receipt`(或新增 `/status`) | compat | 状态/用量 |
| `/help` `/hotkeys`(pi) `/keybindings`(claude/codex) | → 原生 `/help` | compat | 帮助 |
| `/undo` `/redo`(opencode) `/rewind`(/checkpoint)(claude) | → 新增 `/rewind` stub | compat | 未实现（提示） |
| `/fork`(claude/codex/pi) `/clone`(pi) `/new`(codex) | → 新增 `/new`/`/fork` stub | compat | 未实现 |
| `/init`(claude/opencode/codex) `/import`(codex) | → stub | compat | 生成 AGENTS.md（未实现） |
| `/mcp` `/hooks` `/agents` `/plugins` `/apps` `/skills`(claude/codex) | → 识别为未实现 | compat | 提示 |
| `/vim` `/keymap` `/theme`(pi/opencode/codex) `/editor` `/details` `/thinking`(opencode) | → 识别为未实现 | compat | 提示 |
| `/export` `/share`(opencode/codex/pi) `/diff` `/review` `/plan` `/btw` | → 识别为未实现 | compat | 提示（`/btw` 可后续映射） |

> 完整参考命令集见 §3 证据（需实现时直接引用）。计划落地最小集 = 上表 `Implemented:true` 行 + 其余全部注册为 `Implemented:false` 的 stub。

### 2.3 输入法式弹窗（`cli/tui_completion.go` 新建）
`chatTUI` 新增状态：

```go
type completionPopup struct {
    active    bool
    query     string          // "/"+prefix（去掉前导 /）
    items     []*SlashCommand // 当前候选
    selected  int             // -1 = 未选择
}
```

在 `chatTUI` 增加 `completion completionPopup` 字段。判定激活：
输入 `m.input.Value()` 以 `/` 开头、光标位于首行（无换行）、state==tuiIdle、`len(/**/token)<limit`。

交互（`tui_model.go` 现有 Update 的按键分支内新增；优先级在 `tab`/`enter` 之前判断 `completion.active`）：

| 键 | 行为 |
|---|---|
| `/` 等任意字符 | 若处于完成上下文则刷新 `completion.items = Suggest(query)` |
| Tab | `len(items)==1` → 补全为 `/name `（进入「已选命令」态）；多选 → 选中/循环候选 |
| ↑ / ↓ | 上下移动 `selected`（激活时才拦截） |
| Enter | 若 `selected>=0` → 直接触发该命令（无参）；否则保持原 Enter 提交逻辑 |
| Esc | 关闭弹窗 |
| 其它（普通编辑键） | 若不处于 `/` 命令名编辑上下文，关闭弹窗并放行 |

**渲染**（`View` 中，`statusBar` 与 `inputBox` 之间插入）：
候选行 `name + "  " + description`，选中项高亮（`selection` 色反显），宽度 `boxW`。
空候选不渲染任何行。

**光标/高度**：`bottomRows()` 与 `transcriptHeight()` 需计入弹窗行数（弹窗打开时 `+= rows(completion)`）。

### 2.4 配置开关（`cli/config.go`）
`FeatureFlags` 新增：

```go
SlashCompat      bool  `json:"slash_compat"`      // 兼容 claude/pi/opencode/codex 命令（default true）
SlashPopup       bool  `json:"slash_popup"`       // 输入法式 / 前缀联想（default true）
TaskPanel        bool  `json:"task_panel"`        // 右侧任务面板（default true）
MessageActions   bool  `json:"message_actions"`   // 历史消息操作菜单（default true）
WorkspaceSwitch  bool  `json:"workspace_switch"`  // 工作空间目录切换（default true）
```

`buildSlashRegistry(cfg)`：`!SlashCompat` → 不注册兼容项；TUI 中 `!SlashPopup` → 不进入弹窗状态（保留 Tab 补全）。

### 2.5 工作空间目录切换（`cli/tui_workspace.go` 新建）

借鉴 OpenCode 的工作空间管理（`packages/tui/src/context/project.ts`、`packages/tui/src/component/workspace-label.tsx`）：

**命令**：
- `/workspace` 或 `/cd` — 切换当前工作目录
- `/workspace list` 或 `/cd --list` — 列出最近使用的工作目录
- `/workspace <path>` 或 `/cd <path>` — 切换到指定目录
- `/workspace ~` 或 `/cd ~` — 切换到用户主目录
- `/workspace -` 或 `/cd -` — 切换到上一个工作目录

**布局参考**：
- **OpenCode** (`packages/tui/src/context/project.ts`)：
  - 工作空间状态通过 `project.workspace.current()` 获取
  - 工作空间标签显示在侧边栏（`WorkspaceLabel` 组件）
  - 支持多种工作空间类型：`local`、`remote`、`container`
  - 工作空间状态：`connected`、`disconnected`、`error`

- **dsh-tianshu-tui**：
  - 通过 `session.workspaceID` 关联工作空间
  - 工作空间信息显示在状态面板

**状态模型**：

```go
type WorkspaceInfo struct {
    Path        string    // 当前工作目录绝对路径
    Previous    string    // 上一个工作目录（用于 `-` 切换）
    History     []string  // 最近使用的工作目录历史（最多 10 条）
    ProjectName string    // 项目名称（从目录名或配置文件读取）
}

type WorkspaceSwitch struct {
    active    bool
    current   WorkspaceInfo
    mode      WorkspaceSwitchMode  // normal / history / browse
}

type WorkspaceSwitchMode int

const (
    WorkspaceNormal  WorkspaceSwitchMode = iota  // 普通模式：输入路径切换
    WorkspaceHistory                              // 历史模式：显示最近使用目录
    WorkspaceBrowse                               // 浏览模式：目录树浏览
)
```

**交互**：
- `/workspace` 或 `/cd` — 显示当前工作目录，进入输入模式
- `/workspace <path>` — 直接切换到指定目录
- `/workspace list` — 显示最近使用的工作目录列表
- Tab 补全 — 输入路径时提供目录名补全
- ↑/↓ — 在历史记录中选择
- Enter — 确认切换
- Esc — 取消

**渲染**：
- 状态栏显示：`📁 /path/to/current/directory`（可配置显示格式）
- 目录过长时截断显示：`📁 /path/to/.../current/directory`
- 切换成功提示：`✓ 已切换到 /path/to/new/directory`
- 切换失败提示：`✗ 目录不存在或无权限访问`

**数据来源**：
- 当前工作目录：`os.Getwd()` 或会话配置
- 历史记录：持久化到 `~/.inferglow/workspace_history.json`
- 项目名称：从 `.git/config`、`package.json`、`go.mod` 等读取

**配置**：
- `features.workspace_switch`（默认 true）：关闭后禁用工作空间切换
- `workspace_max_history`（默认 10）：历史记录最大条数
- `workspace_status_format`（默认 `"📁 %s"`）：状态栏显示格式

### 2.5 任务面板（`cli/tui_task_panel.go` 新建）

借鉴 OpenCode 的 Todo 侧边栏布局（右侧常驻任务列表，42 列宽）：

```
┌────────────────────────────────┬──────────────┐
│                                │  Todo        │
│   transcript 视口（主对话）       │  ──────────  │
│                                │  [✓] 任务1   │
│                                │  [•] 任务2   │
│                                │  [ ] 任务3   │
├────────────────────────────────┤  [ ] 任务4   │
│   [working spinner / goal]     │              │
│   [status bar]                 │              │
│   [completion popup]           │              │
│   [ input box ]                │              │
└────────────────────────────────┴──────────────┘
```

**布局参考**（来自实际代码分析）：
- **OpenCode** (`packages/tui/src/routes/session/sidebar.tsx`)：
  - 侧边栏固定宽度 42 列，`height="100%"`，背景色 `theme.backgroundPanel`
  - 内部使用 `scrollbox` 实现滚动，支持 `scrollAcceleration`
  - Todo 面板通过插件槽 `sidebar_content` 注册
  - 宽终端（>120 列）自动显示，窄终端使用覆盖层（`position="absolute"`）
  - 内容区宽度 = `dimensions().width - (sidebarVisible() ? 42 : 0) - 4`

- **dsh-tianshu-tui** (`src/format/task-panel.ts`)：
  - 任务状态图标：`[x]` 已完成、`⏳` 进行中、`[ ]` 待处理
  - 面板标题：`📋 任务`
  - 支持宽度截断（`truncateToLiveWidth`）

**状态模型**：

```go
type TaskStatus int

const (
    TaskPending    TaskStatus = iota // [ ] 待处理
    TaskInProgress                   // [•] 进行中
    TaskCompleted                    // [✓] 已完成
)

type TaskItem struct {
    ID          string
    Title       string
    Status      TaskStatus
    CreatedAt   time.Time
    CompletedAt *time.Time
}

type TaskPanel struct {
    active  bool
    tasks   []*TaskItem
    width   int  // 默认 30 列（可配置）
}
```

**数据来源**：复用 `builtins/task_tracker.go` 的 `TaskTracker`，面板只读渲染。

**交互**：
- `/tasks` 命令切换面板显隐
- 面板宽度可通过 `config.json` 的 `task_panel_width` 配置（默认 30）
- 任务状态由 AI 通过 `task_update` 工具更新（类似 OpenCode 的 `todowrite`）
- 宽终端（>120 列）自动显示，窄终端可手动切换

**渲染**：
- 每行格式：`  {status_icon} {title}`
- 状态图标：`[✓]` 绿色（已完成）、`[•]` 黄色（进行中）、`[ ]` 灰色（待处理）
- 面板标题：`Todo` 或自定义（通过 `/tasks rename <name>`）
- 支持折叠：任务数 > 2 时可点击标题折叠/展开

### 2.6 历史消息操作菜单（`cli/tui_message_actions.go` 新建）

借鉴 OpenCode 的 Message Actions 上下文菜单（点击用户消息触发）：

```
┌─────────────────────────────────┐
│  Message Actions           esc  │
│  ─────────────────────────────  │
│  Revert  undo messages and file │
│          changes                │
│  Copy    message text to clip   │
│  Fork    create a new session   │
└─────────────────────────────────┘
```

**布局参考**（来自实际代码分析）：
- **OpenCode** (`packages/tui/src/routes/session/dialog-message.tsx`)：
  - 使用 `DialogSelect` 组件渲染菜单
  - 触发方式：用户消息的 `onMouseUp` 事件
  - 菜单项包含 `title`、`value`、`description`、`onSelect` 回调
  - 操作通过 SDK 客户端调用：`sdk.client.session.revert()`、`clipboard.write()`、`sdk.client.session.fork()`
  - 菜单打开时替换当前对话框（`dialog.replace()`）

- **dsh-tianshu-tui**：
  - 消息操作通过命令面板（Command Palette）触发
  - 支持快捷键绑定

**触发方式**：
- 在 transcript 视口中，按 `m` 或 `a` 进入消息选择模式
- ↑/↓ 移动选择光标，高亮当前用户消息
- Enter 或 `o` 打开操作菜单
- 或直接点击用户消息（鼠标支持）

**菜单项**：

| 操作 | 行为 | 实现状态 |
|---|---|---|
| Revert | 撤销该消息及其后所有消息，恢复文件变更 | P1（需 checkpoint 支持） |
| Copy | 复制消息文本到剪贴板 | P0（直接实现） |
| Fork | 从该消息创建新会话分支 | P1（需 `/fork` 命令支持） |
| Edit | 编辑并重新发送该消息 | P2（需消息编辑能力） |

**状态模型**：

```go
type MessageAction int

const (
    ActionRevert MessageAction = iota
    ActionCopy
    ActionFork
    ActionEdit
)

type MessageActionsMenu struct {
    active      bool
    messages    []*ChatMessage  // 可选择的消息列表
    selected    int             // 当前选中的消息索引
    menuVisible bool            // 菜单是否展开
    menuItems   []MessageAction
    menuCursor  int             // 菜单内光标位置
}
```

**渲染**：
- 选择模式：高亮当前消息行，底部显示操作提示 `[↑↓] 选择  [o] 操作  [Esc] 退出`
- 菜单展开：在选中消息右侧或下方弹出菜单，高亮当前操作项
- 菜单项格式：`  {icon} {name}  {description}`
- 菜单宽度固定，内容过长时截断

**配置**：
- `features.message_actions`（默认 true）：关闭后禁用消息选择模式

---

## 3. 规格证据（收集）

### 3.1 证据来源
- **codex 完整命令集**：`codex/codex-rs/tui/src/slash_command.rs`（读取自工作区源码）
  Model/Ide/Permissions/Keymap/Vim/ElevateSandbox/SandboxReadRoot/Experimental/AutoReview/Memories/Skills/
  Import/Hooks/Review/Rename/New/Archive/Delete/Resume/Fork/App/Init/Compact/Plan/Goal/Agents/Side/Btw/
  Copy/Export/Raw/Diff/Mention/Status/Cd/Pwd/Usage/DebugConfig/Title/Statusline/Theme/Pets/Mcp/Apps/Plugins/
  Logout/Quit/Exit/Feedback/Rollout/Ps/Stop/Clear/Personality/TestApproval/MultiAgents/MemoryDrop/MemoryUpdate
  + 别名（`setup-default-sandbox` 等）。其弹出的前缀联想行为见 `command_popup.rs` 与
  `slash_popup_model_first_for_mo` 等测试（`/mo`→model、`/res`→resume、`/bt`→btw、`/si`→side）。
- **Claude Code**：`docs/guides/.gobuild/claude-commands.md`（内置斜杠命令参考）
- **OpenCode**：`docs/guides/.gobuild/opencode-slash.md`（TUI 命令表与别名）
- **pi (pi-coding-agent)**：`docs/guides/.gobuild/pi-usage.md`（`/model,/scoped-models,/settings,/resume,/new,/name,/session,/tree,/fork,/clone,/compact,/copy,/export,/share,/reload,/hotkeys,/changelog,/quit,/login,/logout`）

> 抓取文件存于 `inferglow-github/.gobuild/`（仅调研缓存，不入库）。

### 3.2 inferglow 现有命令（基线）
原生 `/help, /mode, /memory(search|stats), /compact, /async-compress, /clear, /verbose, /receipt, /session,
/resume, /vision, /sandbox, /config, /showbackground, /rebackground, /quit, /exit`。
（`tui_commands.go` legacy switch + `buildSlashRegistry` 的 `/mode`。）

---

## 4. Baseline / Authority 约束

- 不改 `codex/`。不改其它模块。全部改动集中在 `cli` 模块（`tui_cmd_registry.go`、`tui_commands.go`、
  `tui_model.go`、`config.go`、新 `tui_completion.go`、新测试）。
- 现有 inferglow 命令名保持原语义，兼容命令只做别名/新增；冲突时以 inferglow 原生命令为准（注册跳过重复）。
- 手动 `/rebackground`、`/mode` 保留，不受 `slash_popup` 影响。

---

## 5. Compatibility Boundary（兼容边界）

- 命令名：inferglow 原生命令 = 事实源；外部命令经别名/目录解析，**不覆盖**原生同名。
- 输入：保证普通输入（非 `/` 开头、多行、光标在第二行）不受弹窗影响；空候选不渲染。
- 配置：`features.slash_compat` / `features.slash_popup` 缺省 `true`，关闭后行为回退到「仅原生命令 + Tab 补全」。

---

## 6. TDD Route

```text
TDD Route:
- Mode: auto
- Decision: light
- Strict authority: not applicable（无显式 user/project 严格 TDD 请求）
- Test posture: 兼容目录/模糊匹配的单元测试 + 回归（构建/vet/test）
- Reason: 核心改动是纯函数（Suggest/匹配）与 UI 状态机；UI 用单元测试 + 手动烟测
- Verification: go test ./...（cli）+ go vet ./...
```

---

## 7. File Map

| 文件 | 动作 |
|---|---|
| `cli/tui_cmd_registry.go` | 改：`SlashCommand` 加 `Source/Implemented`；新增 `Suggest`(模糊)、`RegisterOverlay`(不 panic 冲突)、`All()` |
| `cli/tui_compat.go` | 新：兼容目录 `compatCatalog` + `registerCompatCommands(r, cfg)` |
| `cli/tui_commands.go` | 改：`buildSlashRegistry` 调用 `registerCompatCommands`；`tuiDispatchCommand` 处理 `Implemented:false` 提示；注册 `/workspace` 和 `/cd` 命令 |
| `cli/tui_completion.go` | 新：`completionPopup` 状态机 + `update`/`render` + `Refresh(query)` |
| `cli/tui_task_panel.go` | 新：`TaskPanel` 状态机 + 渲染 + 与 `TaskTracker` 数据同步 |
| `cli/tui_message_actions.go` | 新：`MessageActionsMenu` 状态机 + 渲染 + 操作处理 |
| `cli/tui_workspace.go` | 新：`WorkspaceSwitch` 状态机 + 目录切换 + 历史记录 + 状态栏渲染 |
| `cli/tui_model.go` | 改：`chatTUI` 加 `completion`/`taskPanel`/`messageActions`/`workspace` 字段；Update 在 tab/enter 前路由弹窗键；View 在 statusBar 与 inputBox 之间插入弹窗行；`bottomRows/transcriptHeight` 计入；新增消息选择模式键路由；状态栏显示工作目录 |
| `cli/config.go` | 改：`FeatureFlags` 加 `SlashCompat`/`SlashPopup`/`TaskPanel`/`MessageActions`/`WorkspaceSwitch`，默认 true |
| `cli/tui_cmd_registry_test.go` | 新/改：`Suggest`/`Complete` 前缀、子序列、别名去重、limit 测试 |
| `cli/tui_task_panel_test.go` | 新：`TaskPanel` 渲染、状态切换、数据同步测试 |
| `cli/tui_message_actions_test.go` | 新：`MessageActionsMenu` 菜单渲染、操作选择测试 |
| `cli/tui_workspace_test.go` | 新：`WorkspaceSwitch` 目录切换、历史记录、状态栏渲染测试 |
| `docs/guides/tui-build-and-config.md` | 改：features 表补 `slash_compat`/`slash_popup`/`task_panel`/`message_actions`/`workspace_switch`；命令列表补兼容说明 |

---

## 8. Tasks

### Task 1 — 扩展 Registry：Source/Implemented + 模糊 `Suggest`
Files: `cli/tui_cmd_registry.go`，`cli/tui_cmd_registry_test.go`

Change Necessity: 现有 `Complete` 只能 `HasPrefix` 前缀匹配，且无法表达「已识别未实现」；
兼容 + 模糊联想需要新语义。最小边界 = `SlashRegistry`。

Steps:
1. `SlashCommand` 增加字段（见 §2.1）。
2. 新增 `func isSubsequence(prefix, s string) bool`（大小写不敏感，前缀优先交由调用方处理）。
3. 新增：
```go
func (r *SlashRegistry) Suggest(prefix string, limit int) []*SlashCommand {
    prefix = strings.ToLower(prefix)
    var out []*SlashCommand
    seen := map[*SlashCommand]bool{}
    var prefixMatches, subMatches []*SlashCommand
    order := func(c *SlashCommand) bool { return !seen[c] && c.Match(prefix) }
    // 遍历 r.commands：name 与 aliases 都以 prefix 开头 → prefixMatches；否则子序列 → subMatches
    // 排序后合并并截断 limit
}
```
4. 给 `SlashCommand` 加 `func (c *SlashCommand) Match(prefix string) bool`（name 或任一 alias 前缀/子序列匹配；小写）。
5. 测试：`Suggest("mo", 5)` 含 `model`/`memory`/`money`；`Suggest("bt", 5)` 含 `btw`（子序列）；`Suggest("", 5)` 返回前 5 个；别名去重。

Verify: `cd cli && go test -run 'Slash' ./... && go vet ./...`

### Task 2 — 兼容目录与注册
Files: `cli/tui_compat.go`，`cli/tui_commands.go`

Change Necessity: 兼容命令需要一份可维护数据表；`buildSlashRegistry` 注册原生后再叠加兼容项（跳过重名）。

Steps:
1. 新建 `cli/tui_compat.go`，声明 `type compatEntry struct{ name, aliases, desc, source string; implemented bool; handler func(*chatTUI,string)(tea.Cmd,bool) }` 与
   `var compatCatalog = []compatEntry{ ... §2.2 表 ... }`。
2. `func registerCompatCommands(r *SlashRegistry, cfg CLIConfig) { if !cfg.Features.SlashCompat { return } … }`：
   映射到原生命令的项用 `r.index[原生名]` 的同款 handler（或直接复用原生命令实例）；未实现的建 `SlashCommand{Implemented:false, Source:entry.source, Handler:nil}`。
   重名（`name` 已被原生或其它注册）→ 跳过该 `name`，仅加 `Aliases` 合并（`r.index[alias]=原生`）。
3. `tui_commands.go` 的 `buildSlashRegistry` 末尾调用 `registerCompatCommands(r, cfg)`。
4. `tuiDispatchCommand`：当 `m.cmdRegistry.Dispatch` 命中 `Implemented:false` 命令 → `m.commitLine(warnText("  /xxx 已识别（来自 <source>），但本版本未实现。"))`，不退出。可在 `SlashCommand` 上新增 `Implemented()` 判断。

Verify: `go build ./... && go vet ./...`

### Task 3 — IME 式弹窗状态机 + 渲染
Files: `cli/tui_completion.go`，`cli/tui_model.go`

Change Necessity: 现有 Tab 只在多匹配时打印一行，不是实时联想；需要弹窗状态 + 渲染。

Steps:
1. 新建 `cli/tui_completion.go`：`type completionPopup`，方法 `Active()`, `Refresh(query, registry)`, `Move(delta)`, `Selected() *SlashCommand`, `Render(width) string`。
   `Render` 每行 `"  ● <name>  <desc>"`，选中行用 `m.selection`/`selectionColor` 高亮；空返回 ""。
2. `chatTUI` 加 `completion completionPopup` 字段；`chatTUI` 的 `Update` 在 `case "tab"/"enter"` 之前：
```go
// 更新完成上下文
if m.completion.wantsOpen(m.input.Value(), m.state) {
    m.completion.Refresh(strings.TrimPrefix(m.input.Value(), "/"), m.cmdRegistry)
}
```
3. 键路由：`case "tab"` → 若 `completion.active`：`len(items)==1` 用原逻辑补全；否则 `Move(+1)`。  
   `case "up"/"down"` → 若 `completion.active` 拦截 `Move(-1/+1)`（否则放行，避免与历史记录冲突——需确认现有 up/down 是否已绑定历史）。  
   `case "esc"` → 若 `completion.active` 关闭并 return。  
   `case "enter"` → 若 `completion.Selected()!=nil` 且 `Implemented` → 直接 `Dispatch` 该命令（`cmd,quit,_ := r.Dispatch(m,name,"")`），关闭弹窗，return。  
   其它输入字符 → 若 `completion.active` 且非 `/xxx ` 编辑 → `Refresh`。
4. View：在 `parts` 中 `statusBar` 之后、`inputBox` 之前插入 `if rows := m.completion.Render(boxW); rows != "" { parts 追加; rowsAboveBox += 1 }`（每候选一行）。
5. `bottomRows()` 与 `transcriptHeight()` 在 `completion.active` 时 `+= len(items)`。

Verify: `go build ./... && go vet ./...`；手动烟测（见 §9）。

### Task 4 — 配置开关 + 文档
Files: `cli/config.go`，`docs/guides/tui-build-and-config.md`

Steps:
1. `FeatureFlags` 加 `SlashCompat`/`SlashPopup`/`TaskPanel`/`MessageActions`/`WorkspaceSwitch`，`DefaultCLIConfig` 设 true（见 §2.4）。
2. `tui_model.go` 弹窗激活条件再加 `&& m.cfg.Features.SlashPopup`；`buildSlashRegistry` 用 `SlashCompat`。
3. 文档 features 表补五行。

Verify: `go build ./... && go vet ./...`；`cli/examples/config.example.json` 与 `~/.inferglow/config.json` 校验 JSON 可解析。

### Task 5 — 任务面板（Task Panel）
Files: `cli/tui_task_panel.go`，`cli/tui_task_panel_test.go`，`cli/tui_model.go`

Change Necessity: OpenCode 等 TUI 均有右侧任务列表面板，展示任务状态（完成/进行中/待处理），
便于用户跟踪多步骤任务进度。inferglow 需要类似能力以支持复杂任务场景。

Steps:
1. 新建 `cli/tui_task_panel.go`：
   - `type TaskPanel struct`（见 §2.5）
   - `func (p *TaskPanel) Toggle()`：切换面板显隐
   - `func (p *TaskPanel) Render(width int) string`：渲染任务列表
   - `func (p *TaskPanel) Sync(tracker *TaskTracker)`：从 TaskTracker 同步数据
2. `chatTUI` 加 `taskPanel TaskPanel` 字段；
3. `tui_commands.go` 注册 `/tasks` 命令：`func cmdTasks(m *chatTUI, args string) (tea.Cmd, bool)` 调用 `m.taskPanel.Toggle()`。
4. View：若 `taskPanel.active`，在 transcript 视口右侧渲染面板（占 `taskPanelWidth` 列）；
   transcript 宽度相应缩减（`boxW - taskPanelWidth`）。
5. Update：AI 调用 `task_update` 工具时，更新 TaskTracker 并触发 `taskPanel.Sync()`。
6. 测试：`TaskPanel.Render` 渲染不同状态的任务列表；`Toggle` 切换显隐。

Verify: `cd cli && go test -run 'TaskPanel' ./... && go vet ./...`

### Task 6 — 历史消息操作菜单（Message Actions）
Files: `cli/tui_message_actions.go`，`cli/tui_message_actions_test.go`，`cli/tui_model.go`

Change Necessity: OpenCode 支持点击历史用户输入后弹出操作菜单（Revert/Copy/Fork），
便于用户管理对话历史。inferglow 需要类似能力以支持消息级操作。

Steps:
1. 新建 `cli/tui_message_actions.go`：
   - `type MessageActionsMenu struct`（见 §2.6）
   - `func (m *MessageActionsMenu) EnterSelectionMode(messages []*ChatMessage)`：进入选择模式
   - `func (m *MessageActionsMenu) Move(delta int)`：移动选择光标
   - `func (m *MessageActionsMenu) OpenMenu()`：展开操作菜单
   - `func (m *MessageActionsMenu) Select()`：执行选中操作
   - `func (m *MessageActionsMenu) Render(width int) string`：渲染菜单
2. `chatTUI` 加 `messageActions MessageActionsMenu` 字段；
3. `tui_model.go` Update 新增消息选择模式分支（当 `messageActions.active` 时拦截键输入）：
   - `m` 或 `a`：进入消息选择模式（从 transcript 中提取用户消息列表）
   - `↑/↓`：移动选择光标
   - `o` 或 `Enter`：展开操作菜单
   - `↑/↓`（菜单展开时）：菜单内移动光标
   - `Enter`（菜单展开时）：执行选中操作
   - `Esc`：退出选择模式或关闭菜单
4. View：若 `messageActions.active`，高亮当前选中消息行；若 `menuVisible`，在消息旁/下方渲染菜单。
5. 操作实现：
   - `Copy`：使用 `clipboard.Write` 复制消息文本
   - `Revert`：调用 checkpoint API 撤销消息（需 Task 7 支持，先 stub）
   - `Fork`：调用 `/fork` 命令创建新会话（需 Task 7 支持，先 stub）
6. 测试：`MessageActionsMenu` 渲染、菜单项选择、操作触发。

Verify: `cd cli && go test -run 'MessageActions' ./... && go vet ./...`

### Task 7 — 工作空间目录切换（Workspace Switch）
Files: `cli/tui_workspace.go`，`cli/tui_workspace_test.go`，`cli/tui_commands.go`，`cli/tui_model.go`

Change Necessity: OpenCode 等 TUI 均支持工作空间/目录切换，便于用户在不同项目间切换。
inferglow 需要类似能力以支持多项目工作场景。

Steps:
1. 新建 `cli/tui_workspace.go`：
   - `type WorkspaceInfo struct`（见 §2.5）
   - `type WorkspaceSwitch struct`
   - `func (w *WorkspaceSwitch) GetCurrentDir() string`：获取当前工作目录
   - `func (w *WorkspaceSwitch) SetCurrentDir(path string) error`：设置当前工作目录
   - `func (w *WorkspaceSwitch) GetHistory() []string`：获取历史记录
   - `func (w *WorkspaceSwitch) AddHistory(path string)`：添加历史记录
   - `func (w *WorkspaceSwitch) RenderStatus() string`：渲染状态栏工作目录
2. `chatTUI` 加 `workspace WorkspaceSwitch` 字段；
3. `tui_commands.go` 注册 `/workspace` 和 `/cd` 命令：
   - `func cmdWorkspace(m *chatTUI, args string) (tea.Cmd, bool)` — 处理目录切换
   - 支持参数：`<path>`、`list`、`~`、`-`
4. `tui_model.go` Update：当 workspace 模式激活时拦截键输入
5. View：在状态栏显示当前工作目录（`workspace.RenderStatus()`）
6. 历史记录持久化：`~/.inferglow/workspace_history.json`
7. Tab 补全：输入路径时提供目录名补全
8. 测试：`WorkspaceSwitch` 渲染、目录切换、历史记录、错误处理。

Verify: `cd cli && go test -run 'Workspace' ./... && go vet ./...`

---

## 9. Verification（验证）

```powershell
# 单元 + 回归
$env:GOCACHE="E:\test\rewrite-agently\inferglow-github\.gobuild\cache"
$env:GOMODCACHE="E:\test\rewrite-agently\inferglow-github\.gobuild\mod"
$env:GOPATH="E:\test\rewrite-agently\inferglow-github\.gobuild\gopath"
$env:GOTOOLCHAIN="auto"
cd E:\test\rewrite-agently\inferglow-github\cli
go vet ./...
go test ./...
go build -o bin\inferglow-cli.exe .\cmd\inferglow-cli
```

**手动 TUI 烟测**（Windows Terminal）：
1. 输入 `/` → 出现候选弹窗；输入 `/mo` → 候选收窄到 `model`/`memory`…
2. `/bt` → 候选含 `btw`（子序列联想）。
3. ↑/↓ 选择、回车触发；Esc 关闭。
4. 输入 `/vim` → 提示「已识别（来自 codex），未实现」。
5. 配置 `slash_popup=false` → 弹窗消失，Tab 老行为保留。
6. **任务面板**：输入 `/tasks` → 右侧出现 Todo 面板；AI 完成任务后状态自动更新（`[✓]`）。
7. **消息操作**：按 `m` 进入消息选择模式 → ↑/↓ 选择用户消息 → 按 `o` 弹出操作菜单 → 选择 `Copy` 验证剪贴板内容。
8. **工作空间切换**：输入 `/workspace` → 显示当前工作目录；输入 `/workspace /tmp` → 切换到 /tmp 目录；输入 `/workspace list` → 显示历史记录。

---

## 10. Risks / Rollback / Retirement

| 风险 | 处理 |
|---|---|
| up/down 已绑定输入历史，弹窗拦截会冲突 | Task 3 复核现有 up/down 绑定；仅在 `completion.active` 且 `state==tuiIdle` 时拦截 |
| 弹窗渲染抬高输入行导致光标定位偏移 | `bottomRows/transcriptHeight` 与 `View` 光标 `Y` 偏移同步计入 |
| 兼容目录过大造成 / 联想噪音 | `Suggest` 排序已保证前缀优先 + limit；未实现项降权重 |
| 覆盖原生命令 | 注册时跳过重名，绝不覆盖 |
| 行为变化 | `slash_compat/slash_popup/task_panel/message_actions/workspace_switch` 可整体关闭，一键回退 |
| 任务面板与 transcript 宽度竞争 | 默认面板宽度 30 列，可配置；transcript 最小宽度保证 40 列 |
| 消息选择模式与命令弹窗键冲突 | 消息选择模式仅在 `state==tuiIdle` 且无弹窗时激活；`m`/`a` 键在输入框内不触发 |
| Revert/Fork 操作需要 checkpoint 支持 | 先 stub 实现（提示「功能开发中」），后续 Task 7 补充 |
| 工作空间切换可能影响 AI 上下文 | 切换目录后自动通知 AI 当前工作目录变更；AI 可通过工具感知目录变化 |
| 工作空间历史记录持久化失败 | 历史记录写入失败时静默降级，不影响目录切换功能 |

**Retirement**：`compatCatalog` 后续若被 config 驱动、外部命令文件替代，删除目录函数即可；不改原生命令语义。
`TaskPanel`/`MessageActionsMenu`/`WorkspaceSwitch` 可通过 `features.task_panel`/`features.message_actions`/`features.workspace_switch` 完全禁用。

---

## 11. Execution Route

```text
Execution Route:
- Decision: inline
- Evidence: 改动集中在同一模块（cli TUI）且强依赖（注册顺序→弹窗渲染→布局），subagent 切分会破坏上下文
- Fallback: 若后续扩展大，可再拆为「Phase A 命令兼容」+「Phase B 弹窗」两个独立 PR
- User confirmation required: no
```

> 需用户确认才进入实现；当前为规划+证据稿。

---

## 12. Related Specs

- **任务管理 TUI 分块**（codex/opencode 面板布局调研 + inferglow 分块设计）：
  独立规格见 `docs/requirements/tui-task-panels-spec.md`。本计划的 Task 5（任务面板）和 Task 6（消息操作菜单）
  由该 spec 的 P1/P2 承接；本计划的 §2.5/§2.6 为具体实现设计。
- **OpenCode Todo 面板参考**：OpenCode 的 Todo 侧边栏（右侧常驻任务列表，42 列宽，[✓]/[•]/[ ] 状态图标）
  是本计划 Task 5 的主要视觉参考（见用户提供的截图）。
- **OpenCode Message Actions 参考**：OpenCode 的消息操作菜单（Revert/Copy/Fork）
  是本计划 Task 6 的主要交互参考（见用户提供的截图）。
- **代码参考仓库**：
  - `anomalyco/opencode` — OpenCode TUI 源码（`packages/tui/src/`）
  - `huiliyi37/dsh-tianshu-tui` — dsh-tianshu-tui 任务面板实现（`src/format/task-panel.ts`）
  - `huiliyi37/Tianshu-harness` — Tianshu harness 框架
  - `ccch1mneyyy/dsh-TUI` — dsh-TUI 组件实现
