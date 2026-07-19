# InferGlow CLI TUI 对标实施计划

## 目标

将 InferGlow CLI 从 `bufio.Scanner + fmt.Print` 线性 REPL 升级为基于 Charmbracelet Bubble Tea v2 的全屏 TUI，对标 Reasonix TUI 的核心交互能力：全屏 Alt-Screen、流式输出、状态栏、主题系统、工具卡片、推理显示。

## 设计原则

1. **Feature Flag 驱动**：`--tui` flag 切换，默认关闭，旧 REPL 完全不受影响
2. **零修改现有逻辑**：`repl.go`、`commands.go`、`agent_factory.go` 等文件不改动
3. **同包新文件**：所有 TUI 代码放在 `cli/` 包内，`tui_*.go` 前缀，直接访问 `MemoryBridge`、`CLIConfig` 等类型
4. **复用现有事件系统**：不修改 `event_sink.go`，现有 9 种 EventKind 足以支撑 P0-P2
5. **性能优先**：事件批处理排空、有界缓冲、增量渲染，避免长会话性能退化

## 参考架构

```
Reasonix: cli.go(chatREPL) → tea.NewProgram(chatTUI) → Update/View
InferGlow: main.go(RunREPL) → bufio.Scanner → fmt.Print  ← 改造目标
```

---

## Phase 0: 依赖引入与骨架搭建

### Step 0.1: 添加 Bubble Tea 依赖

**文件**: `inferglow/cli/go.mod`

添加：
```
charm.land/bubbletea/v2 v2.0.7
charm.land/bubbles/v2 v2.1.0
charm.land/lipgloss/v2 v2.0.4
github.com/charmbracelet/x/ansi v0.11.7
```

运行 `go mod tidy` 验证。

### Step 0.2: 添加 TUI Feature Flag

**文件**: `inferglow/cli/config.go` (第 50-56 行)

在 `FeatureFlags` 结构体中添加：
```go
TUIMode bool `json:"tui_mode"` // Enable full-screen TUI mode
```

### Step 0.3: 入口点添加 --tui flag

**文件**: `inferglow/cli/cmd/inferglow-cli/main.go` (第 41 行后)

```go
tuiMode := flag.Bool("tui", false, "Enable full-screen TUI mode")
```

第 77 行前添加分支：
```go
if *tuiMode || cfg.Features.TUIMode {
    if err := cli.RunTUI(ctx, cfg, *resumeID); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    return
}
```

### Step 0.4: 创建 tui_model.go — Elm Model 骨架

**新建文件**: `inferglow/cli/tui_model.go`

核心结构体（~25 字段，对比 Reasonix 的 ~100 字段）：
```go
type chatTUI struct {
    // 核心引用
    agent    *agent.Agent
    bridge   *MemoryBridge
    cfg      CLIConfig
    sessionID string
    modelLabel string

    // 终端尺寸
    width  int
    height int

    // 状态机
    state tuiState // tuiIdle | tuiRunning

    // UI 组件
    input    textarea.Model
    spinner  spinner.Model
    viewport viewport.Model

    // Transcript
    transcript      []string
    transcriptDirty bool

    // 流式缓冲
    reasoning  *strings.Builder
    pending    *strings.Builder
    showReasoning bool

    // 运行状态
    runStart time.Time
    elapsed  int
    turnTokens int

    // 事件通道
    eventCh   <-chan agent.AgentEvent
    closeSink func()

    // 历史输入
    submittedInputs []string
    submittedCursor int

    // 退出控制
    lastCtrlCAt time.Time
    quit        bool
}
```

关键方法：
- `RunTUI(ctx, cfg, resumeID) error` — 入口函数，构建 agent + 创建 channel sink + `tea.NewProgram`
- `Init() tea.Cmd` — `tea.Batch(textarea.Blink, waitForAgentEvent(m.eventCh))`
- `Update(msg tea.Msg) (tea.Model, tea.Cmd)` — 主更新循环
- `View() string` — 全屏渲染

事件桥接（与 Reasonix 一致的模式）：
```go
type agentEventMsg agent.AgentEvent

func waitForAgentEvent(ch <-chan agent.AgentEvent) tea.Cmd {
    return func() tea.Msg { return agentEventMsg(<-ch) }
}
```

### Step 0.5: 创建 tui_theme.go — 基础调色板

**新建文件**: `inferglow/cli/tui_theme.go`

简化版 Reasonix `theme.go`（~100 行）：
```go
type cliColor struct { hex string; xterm int }
type cliPalette struct {
    accent, muted, subtle, subtle2 cliColor
    success, warn, err, info       cliColor
    border, selection              cliColor
    userBG, userFG                 cliColor
    reasoning                      cliColor
}

var darkTheme = cliPalette{...}  // 参考 Reasonix cliDarkTheme
var activeTheme = &darkTheme     // 运行时可切换
```

工具函数：`themeFg(color, text)`, `dim(text)`, `accent(text)`, `errorText(text)`

---

## Phase 1: 事件处理与流式渲染

### Step 1.1: 创建 tui_view.go — Transcript 渲染

**新建文件**: `inferglow/cli/tui_view.go`

核心函数：
- `commitLine(m *chatTUI, text string)` — 追加一行到 transcript，标记 dirty
- `commitUserBubble(m *chatTUI, text string)` — 用户消息气泡（带背景色）
- `commitAssistantBlock(m *chatTUI)` — 将 pending buffer 提交为完整回答
- `commitReasoningBlock(m *chatTUI)` — 推理块（dim 样式）
- `commitToolCard(m *chatTUI, name, status, output string)` — 工具卡片
- `renderTranscript(m *chatTUI) string` — 组装 viewport 内容

简化决策：不做 Reasonix 的 `transcriptSources` 双轨存储。直接用 `[]string` + `viewport.SetContent()`。

### Step 1.2: 实现事件批处理引擎

**文件**: `tui_model.go` 的 `Update()` 方法

```go
case agentEventMsg:
    e := agent.AgentEvent(msg)
    m.ingestEvent(e)
    turnDone := e.Kind == agent.EventRunEnd

    // 批处理排空：合并突发事件（参考 Reasonix maxEventDrain=512）
    for drained := 0; drained < 256; drained++ {
        select {
        case e2 := <-m.eventCh:
            m.ingestEvent(agent.AgentEvent(e2))
            if e2.Kind == agent.EventRunEnd { turnDone = true }
        default:
            goto doneDrain
        }
    }
    doneDrain:

    // 批量更新后单次 reflow
    if m.transcriptDirty {
        m.viewport.SetContent(strings.Join(m.transcript, "\n"))
        m.transcriptDirty = false
        if wasAtBottom { m.viewport.GotoBottom() }
    }

    cmds = append(cmds, waitForAgentEvent(m.eventCh))
    if turnDone {
        m.state = tuiIdle
        m.commitAssistantBlock()
    }
    return m, tea.Batch(cmds...)
```

### Step 1.3: 实现 ingestEvent() 事件分发

**文件**: `tui_model.go`

| EventKind | 处理逻辑 |
|-----------|---------|
| `EventRunStart` | state=running, 清空 buffers, 记录 runStart, commitUserBubble |
| `EventToken` | 追加到 pending builder, 标记 dirty |
| `EventReasoning` | 追加到 reasoning builder, 标记 dirty |
| `EventToolStart` | commit pending/reasoning, commitToolCard(name, "running", "") |
| `EventToolEnd` | 更新 tool card 状态 (完成/错误) |
| `EventRunEnd` | state=idle, commit 所有 pending, 记录 elapsed |
| `EventError` | commit error notice |
| `EventLLMStart/End` | 可选：显示 round 指示器 |

### Step 1.4: 实现 turn 启动流程

**文件**: `tui_model.go`

当用户按 Enter 提交消息时：
1. `commitUserBubble(input)`
2. 创建 `NewChannelSink(1024)` (加大缓冲)
3. 构建 `CallbacksFromSink(sink)`
4. 在 goroutine 中调用 `agent.Run(ctx, message, WithCallbacks(cb), WithSystemPrompt(sysPrompt))`
5. Run 返回后调用 `closeSink()`
6. 注册 `waitForAgentEvent(eventCh)` 开始事件消费

---

## Phase 2: 布局与状态栏

### Step 2.1: 创建 tui_footer.go — 状态栏

**新建文件**: `inferglow/cli/tui_footer.go`

简化版 Reasonix `status_footer.go`（~120 行）：

```
Chat · Idle · Shift+Tab Plan · Ctrl+Y YOLO
模型 deepseek-v3   上下文 12k/100k (12%)
```

函数：
- `renderStatusBar(m *chatTUI) string` — 完整状态区域
- `primaryStatusLine(m *chatTUI) string` — 模式标签 + 交互状态
- `telemetryLine(m *chatTUI) string` — 模型 + 上下文用量
- `layoutStatusSides(left, right, width) string` — 左右对齐/换行

### Step 2.2: 完善 View() 布局

**文件**: `tui_model.go` 的 `View()` 方法

```
+------------------------------------------+
|  Transcript (viewport, scrollable)       |
+------------------------------------------+
|  ⠋ thinking… (elapsed 3s)               |  ← runningWorkingLine (仅运行时)
+------------------------------------------+
|  ┌─ Input ─────────────────────────────┐ |
|  │ textarea (multi-line)               │ |
|  └─────────────────────────────────────┘ |
|  Chat · Idle · model:deepseek · ctx:12%  |  ← status bar
+------------------------------------------+
```

关键计算：
- `bottomRows()` = input height + status bar height + working line (if running)
- `viewport.SetHeight(m.height - bottomRows())`
- 使用 `lipgloss` 渲染 input box 边框和 status bar 样式

### Step 2.3: 窗口尺寸处理

**文件**: `tui_model.go` 的 `Update()` 方法

```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    m.input.SetWidth(max(msg.Width-4, 1))
    contentW := max(m.width-1, 1) // 留 1 列给滚动条
    m.viewport.SetWidth(contentW)
    m.viewport.SetHeight(m.transcriptHeight())
    if widthChanged {
        m.reflowTranscript(m.width)
    }
```

---

## Phase 3: 交互增强

### Step 3.1: 键绑定

**文件**: `tui_model.go` 的 `Update()` 方法

| 键 | 上下文 | 行为 |
|---|---|---|
| `Enter` | idle | 提交输入，启动 turn |
| `Shift+Enter` | 任意 | 输入框换行 |
| `Ctrl+C` | running | 取消当前 turn |
| `Ctrl+C` | idle, 空输入, 1.5s 内双击 | 退出 |
| `Ctrl+D` | idle, 空输入 | 退出 |
| `Ctrl+O` | 任意 | 切换 reasoning 显示 |
| `Up/Down` | idle, 空输入 | 浏览历史输入 |
| `PgUp/PgDn` | 任意 | 滚动 viewport |
| `/` 开头 | idle | 斜杠命令 |

### Step 3.2: 斜杠命令 TUI 适配

**新建文件**: `inferglow/cli/tui_commands.go`

不改 `commands.go`，在 TUI 层拦截 `/` 前缀输入：
- `/help` → 在 transcript 中渲染命令列表
- `/memory stats` → 渲染记忆统计
- `/compact` → 触发压缩，显示进度
- `/quit` → `tea.Quit`
- `/clear` → 清空 transcript
- `/model <name>` → 切换模型（后续实现）

### Step 3.3: 信号处理改造

**文件**: `inferglow/cli/cmd/inferglow-cli/main.go`

TUI 模式下，SIGINT/SIGTERM 通过 `program.Send(shutdownMsg{})` 通知 TUI 优雅退出（参考 Reasonix `cli.go:1214-1222`），而非直接 cancel context。

---

## Phase 4: 性能优化

### Step 4.1: Token 合并渲染

**文件**: `tui_model.go`

维护 `answerFlushed int` 记录已渲染字节数。收到 `EventToken` 时追加到 pending builder，仅当新文本构成完整段落/行时才更新 transcript。将 O(tokens) 次渲染降低为 O(paragraphs) 次。

### Step 4.2: 推理流有界窗口

**文件**: `tui_model.go`

`reasoningView []byte` 维护推理文本的有界尾部窗口（最后 2048 字节）。完整文本保留在 `reasoning *strings.Builder` 中（用于 Ctrl+O verbose 模式）。避免长推理链导致 O(n) 渲染成本。

### Step 4.3: Transcript 有界管理

**文件**: `tui_view.go`

设置 `maxTranscriptBlocks = 5000` 上限。超过时从头部 evict 旧块。`wrappedLines []string` 缓存换行后的行，避免每帧重新换行。

### Step 4.4: Viewport 增量更新

使用 `transcriptDirty` 标志位避免不必要的 `viewport.SetContent`。仅在以下情况触发：
1. 新块被 commit
2. 流式块原地更新
3. 终端宽度变化

---

## Phase 5: 测试

### Step 5.1: 单元测试

**新建文件**: `inferglow/cli/tui_test.go`

- 事件桥接：AgentEvent → agentEventMsg 转换
- 批处理排空：验证多事件合并
- Transcript 有界管理：evict 逻辑
- 窗口 resize 后的 reflow
- 状态栏不同宽度下的布局

### Step 5.2: 性能基准

**新建文件**: `inferglow/cli/tui_bench_test.go`

- `BenchmarkIngestEvent_TokenBurst` — 10000 个连续 token
- `BenchmarkViewRender` — 完整 View() 渲染帧率
- 目标：单帧 < 5ms，10000 token < 100ms

---

## 文件变更清单

| 文件 | 操作 | 行数变化 | 说明 |
|------|------|---------|------|
| `cli/go.mod` | 修改 | +4 | 添加 bubbletea 依赖 |
| `cli/config.go` | 修改 | +1 | FeatureFlags 添加 TUIMode |
| `cli/cmd/inferglow-cli/main.go` | 修改 | +8 | 添加 --tui flag + 分支 |
| `cli/tui_model.go` | **新建** | ~500 | Elm Model + Update + 事件处理 |
| `cli/tui_view.go` | **新建** | ~200 | Transcript 渲染 + 布局 |
| `cli/tui_theme.go` | **新建** | ~100 | 调色板 + 样式函数 |
| `cli/tui_footer.go` | **新建** | ~120 | 状态栏渲染 |
| `cli/tui_commands.go` | **新建** | ~80 | TUI 斜杠命令 |
| `cli/tui_test.go` | **新建** | ~150 | 单元测试 |
| `cli/tui_bench_test.go` | **新建** | ~60 | 性能基准 |

**总计**: 修改 3 个现有文件（+13 行），新增 6 个文件（~1210 行）

---

## 依赖关系

```
Phase 0: 0.1(go.mod) → 0.2(config) → 0.3(main.go) → 0.4(tui_model) → 0.5(tui_theme)
Phase 1: 1.1(tui_view) → 1.2(批处理) → 1.3(ingestEvent) → 1.4(turn启动)
Phase 2: 2.1(tui_footer) → 2.2(View布局) → 2.3(窗口处理)
Phase 3: 3.1(键绑定) → 3.2(命令) → 3.3(信号)
Phase 4: 4.1-4.4 (性能优化，依赖 Phase 1-2)
Phase 5: 5.1-5.2 (测试，依赖所有 Phase)
```

**推荐实施顺序**: 0.1 → 0.2 → 0.3 → 0.5 → 0.4 → 1.1 → 1.2 → 1.3 → 1.4 → 2.1 → 2.2 → 2.3 → 3.1 → 3.2 → 3.3 → 4.1-4.4 → 5.1-5.2

---

## 风险与缓解

| 风险 | 严重度 | 缓解措施 |
|------|--------|---------|
| Bubble Tea v2 依赖解析失败 | 中 | 提前验证 `go mod download`；备选使用 github.com/charmbracelet 旧路径 |
| 终端 raw mode 崩溃后残留 | 高 | Bubble Tea 自动处理；添加 SIGHUP/SIGTERM handler 通过 `program.Send()` 优雅退出 |
| 事件通道死锁 | 中 | 1024 缓冲 + 非阻塞 Token 策略（现有） + 批处理排空 |
| 旧 REPL 回归 | 低 | Feature Flag 默认关闭；`repl.go` 零修改；保留 `--no-tui` 作为显式 fallback |
| 长会话内存膨胀 | 低 | Transcript 有界管理 (Phase 4.3)：5000 块上限 |
| 多模块 replace 冲突 | 低 | 新依赖仅添加到 `cli/go.mod`，不影响其他模块 |

---

## 被拒绝的替代方案

### 方案 A: 创建 `cli/tui/` 子包
- **拒绝原因**: 子包无法直接访问 `cli/` 包的 `MemoryBridge`、`CLIConfig` 等类型，需要定义接口或导出更多字段，增加复杂度。当前 `cli/` 包仅 11 个文件，同包 `tui_` 前缀已足够清晰。

### 方案 B: 修改 `event_sink.go` 扩展事件类型
- **拒绝原因**: 修改核心事件系统风险高，可能影响其他消费者（server、desktop）。现有 9 种事件足以支撑 P0-P2 的全部功能。后续需要新事件类型时再扩展。

### 方案 C: 直接替换 REPL 为 TUI（无 Feature Flag）
- **拒绝原因**: 无法回滚，CI/调试环境可能需要线性输出。Feature Flag 提供安全网。

### 方案 D: 使用 tcell/termbox 替代 Bubble Tea
- **拒绝原因**: Bubble Tea 是 Reasonix 的选型，生态成熟，Elm 架构易于测试。tcell 更底层但需要自行实现更多组件。

---

## 关键参考文件

| 用途 | 路径 |
|------|------|
| Reasonix TUI 主 Model | `DeepSeek-Reasonix/internal/cli/chat_tui.go` |
| Reasonix 事件定义 | `DeepSeek-Reasonix/internal/event/event.go` |
| Reasonix 入口 | `DeepSeek-Reasonix/internal/cli/cli.go` |
| Reasonix Status Footer | `DeepSeek-Reasonix/internal/cli/status_footer.go` |
| Reasonix Theme | `DeepSeek-Reasonix/internal/cli/theme.go` |
| InferGlow 当前 REPL | `inferglow/cli/repl.go` |
| InferGlow 事件系统 | `inferglow/orchestrator/agent/event_sink.go` |
| InferGlow 入口 | `inferglow/cli/cmd/inferglow-cli/main.go` |
