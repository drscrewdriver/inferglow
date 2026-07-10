# TUI 升级修复记录 — 2026-07-31

> 日期：2026-07-31
> 范围：CLI 从 bufio.Scanner REPL 升级到 Bubble Tea v2 全屏 TUI 的全部修复
> 涉及模块：cli、orchestrator/agent、model

---

## 背景

将 InferGlow CLI 的交互模式从简单的 `bufio.Scanner` REPL 升级为 Bubble Tea v2 全屏 TUI（alt-screen 模式）。升级过程中暴露了多个交互、架构和安全层面的问题。

---

## 第一轮修复（前一会话，逐 bug 修复，效果不佳）

| # | 尝试 | 结果 |
|---|------|------|
| 1 | Textarea 焦点修复 | 无效 — 仅调整 focus 调用顺序 |
| 2 | 粘贴折叠处理 | 部分 — 大段粘贴仍然折叠为一行 |
| 3 | Viewport 高度计算 | 无效 — 未考虑 bottom region 动态高度 |
| 4 | Event channel nil capture | 无效 — 未识别 channel 关闭零值问题 |
| 5 | Key handling 顺序 | 无效 — 未分离特殊键和文本输入 |

**用户反馈**："你的修复几乎没有用"，需要从架构层面做整体对比分析。

---

## 第二轮修复（架构对比分析后，P0+P1 整体重写）

### 文件：`orchestrator/agent/event_sink.go`

- `AgentEvent` 新增 `Tokens int` 字段，用于传递 LLM completion token 计数
- `CallbacksFromSink` 中 `OnLLMCallEnd` 传递 token 参数

### 文件：`cli/tui_model.go`（完整重写）

**核心架构改动：**

1. **Transcript 模型**：所有显示内容（用户消息、助手回复、工具卡片、错误）统一写入 `[]string` transcript，viewport 渲染 transcript
2. **段落级流式渲染**：`flushableMarkdownPrefix()` 只渲染已完成的 Markdown 段落，半写的代码块留在 buffer
3. **Cursor 定位**：`View()` 中计算 `cur.Y = viewport.Height() + rowsAboveBox + 1`
4. **鼠标滚轮支持**：`tea.MouseWheelMsg` 处理，上下各 3 行
5. **Paste 折叠**：大段粘贴（≥200 字符或 ≥5 行）折叠为 `[Pasted text #N · M lines]`，提交时展开
6. **输入历史**：空输入时 Up/Down 键浏览历史

---

## 第三轮修复（交互 bug）

### Bug #1：Agent debug 日志泄漏到终端

**文件：** `cli/tui_model.go` — `RunTUI()`

**问题：** Agent 内部使用 `log.Printf("[agent-debug]...")` 输出调试日志。在 alt-screen 模式下，这些日志直接写入 stderr 腐蚀 TUI 显示。

**修复：**
```go
func RunTUI(...) error {
    log.SetOutput(io.Discard) // 抑制 agent debug 日志
    // ...
}
```

### Bug #2：Channel 关闭后零值事件无限循环

**文件：** `cli/tui_model.go`

**问题：** Agent goroutine 结束后调用 `closeSink()` 关闭 event channel。`<-m.eventCh` 在 channel 关闭后立即返回零值 `AgentEvent{Kind: 0}`，TUI 不断收到假事件导致死循环。

**修复：** 新增 `eventChClosed bool` 字段和 `channelClosedMsg` 类型：
```go
func (m *chatTUI) waitForAgentEvent() tea.Cmd {
    if m.eventChClosed { return nil }
    return func() tea.Msg {
        e, ok := <-m.eventCh
        if !ok { return channelClosedMsg{} }
        return agentEventMsg(e)
    }
}
```

### Bug #3：Ctrl+D 无效

**文件：** `cli/tui_model.go`

**问题：** 原逻辑要求 `state == tuiIdle && input == ""` 才退出，导致有输入时 Ctrl+D 无响应。

**修复：** 改为两步逻辑 — 有内容先清空，再按退出：
```go
case "ctrl+d":
    if strings.TrimSpace(m.input.Value()) != "" {
        m.input.SetValue("")
        m.input.Blur()
        m.input.Focus()
        return m, nil
    }
    return m, tea.Quit
```

### Bug #4：第二次输入后 agent 不执行（卡在 "thinking... 0s"）

**文件：** `cli/tui_model.go` — `submitTurn()`

**问题：** 第一轮 turn 结束后 `eventChClosed = true`。第二轮 `submitTurn` 创建了新 channel 但没有重置标志，`waitForAgentEvent()` 直接返回 nil。

**修复：**
```go
func (m *chatTUI) submitTurn(message string) {
    // ...
    m.eventCh = events
    m.closeSink = closeSink
    m.eventChClosed = false // Reset for new turn
    // ...
}
```

---

## 第四轮修复（ctx 计数偏少）

### Bug #5：Callbacks 覆盖导致助手回复未存储

**文件：** `cli/tui_model.go` — `submitTurn()`

**问题：** `submitTurn` 中 `WithCallbacks(sinkCB)` 完全覆盖了 agent 的 persisted callbacks，包括 `OnRunEnd → IngestAssistant`。助手回复从未被存储到 context store。

**第一次修复（有 bug）：** 使用 `pickCallback` 选择非 nil 的 callback。但 `CallbacksFromSink` 返回的结构体中**所有 8 个字段都是非 nil 的 lambda**，所以永远选 sink callbacks，原有 `OnRunEnd → IngestAssistant` 仍被丢弃。

**最终修复：** 重写 `mergeCallbacks`，当 original 和 override 都有非 nil hook 时，生成同时调用两者的 wrapper：
```go
func mergeCallbacks(original, override *agent.AgentCallbacks) *agent.AgentCallbacks {
    // ...
    return &agent.AgentCallbacks{
        OnRunEnd: func(ctx context.Context, response string, err error) {
            if original.OnRunEnd != nil {
                original.OnRunEnd(ctx, response, err)
            }
            if override.OnRunEnd != nil {
                override.OnRunEnd(ctx, response, err)
            }
        },
        // ... 其余 7 个字段同理
    }
}
```

**新增：** `agent.Agent.Callbacks()` 方法暴露 agent 的 persisted callbacks。

### Bug #6：Token 估算不准（字节计数 vs rune 计数）

**文件：** `cli/memory_bridge.go` — `estimateTokens()`

**问题：** 使用 `len(s) / 4`（字节计数），对短消息严重偏低。

**修复：** 改用 rune 计数（1 token ≈ 3 runes）：
```go
func estimateTokens(s string) int {
    runes := []rune(s)
    n := len(runes)
    tokens := n / 3
    if tokens < 1 { return 1 }
    return tokens
}
```

### Bug #7：API 未返回 usage 信息

**文件：** `model/openai.go`

**问题（三重）：**
1. 流式请求未设置 `stream_options: {"include_usage": true}`
2. `processOpenAILine` 中空 choices chunk（携带 usage）不 emit
3. `fireOnLLMCallEnd` 使用 `len(content.String())`（字节数）而非 provider 报告的 token 数

**修复：**
```go
// 1. 请求体添加 stream_options
reqBody := map[string]any{
    "model":    data.Model,
    "messages": msgs,
    "stream":   true,
    "stream_options": map[string]any{"include_usage": true},
}

// 2. 空 choices chunk emit usage
if len(chunk.Choices) == 0 {
    if usage != nil {
        emit(&StreamChunk{Usage: usage})
    }
    return usage
}

// 3. fireOnLLMCallEnd 优先使用 provider 报告的 CompletionTokens
endTokens := len(content.String())
if lastUsage != nil && lastUsage.CompletionTokens > 0 {
    endTokens = lastUsage.CompletionTokens
}
```

---

## 待处理：Approval 和 Sandbox Active Type 的 TUI 集成

### 当前状态

TUI 目前对 tool 事件的处理仅限于：
- `EventToolStart` → 显示工具卡片 "running"
- `EventToolEnd` → 显示工具卡片 "done" / "error"

**未处理以下场景：**

### 1. Approval 审批流程

**现有架构：**
- `approval.PolicyApprovalManager` 支持 `Submit()` → `Record` 流程
- `DecisionStatus` 有 `approved` / `denied` / `pending` / `allowed` 四种状态
- `SandboxExecutor.Execute()` 在 `Baseline.ApprovalRequired` 时调用 `ApprovalManager.Submit()`
- 返回 `ActionResult{Status: "blocked", Error: "pending approval: <record_id>"}` 表示等待审批

**TUI 缺失：**
- 无 `EventApproval` 事件类型 — agent 的 `EventToolEnd` 只传递 `toolName + err`，不传递 `blocked` 状态
- 无审批 UI — 用户无法在 TUI 中看到审批请求、approve/deny
- 无 `ResolveRecord` 交互 — 无法从 TUI 发送审批决策

**需要新增：**
```
EventKind: EventApproval
AgentEvent 新增: ApprovalRecordID string, ApprovalPayload map[string]any
TUI 新增: 审批提示 UI（显示工具名 + 参数 + [Y]es/[N]o 快捷键）
回调新增: OnApprovalRequired / OnApprovalResolved
```

### 2. Sandbox Active Type

**现有架构：**
- `sandbox.SandboxMode`：`ModeDocker` / `ModeTrustedLocal` / `ModeAuto` 等
- `sandbox.HandleStatus`：`created` / `running` / `stopped` / `error`
- `action.SideEffectLevel`：`none` / `read` / `write` / `network` / `exec`
- `ActionSpec` 包含 `SandboxRequired bool` 和 `SideEffectLevel`

**TUI 缺失：**
- 工具卡片不显示 sandbox 模式（Docker / TrustedLocal / 无沙箱）
- 工具卡片不显示 SideEffectLevel（read/write/exec 风险等级）
- 不显示 Handle 生命周期（create → start → execute → stop）
- 无 sandbox 可用性预检显示

**需要新增：**
```
EventKind: EventSandboxStatus
AgentEvent 新增: SandboxMode string, HandleStatus string, SideEffectLevel string
TUI 工具卡片增强: 显示 [Docker·write] 或 [Trusted·read] 等标签
```

### 3. ActiveType 综合显示

工具调用时 TUI 应展示的综合信息：

| 维度 | 数据来源 | 显示示例 |
|------|----------|----------|
| 工具名 | `EventToolStart.ToolName` | `bash_execute` |
| 副作用级别 | `ActionSpec.SideEffectLevel` | `[exec]` / `[read]` / `[write]` |
| 沙箱模式 | `SandboxExecutor.cfg.DefaultMode` | `[Docker]` / `[Trusted]` / `[none]` |
| 审批状态 | `ActionResult.Status` | `[pending approval]` / `[approved]` |
| 执行状态 | `EventToolEnd.Err` | `running` / `done` / `error` / `blocked` |

---

## 修复总结

| # | 文件 | 问题 | 状态 |
|---|------|------|------|
| 1 | `tui_model.go` | debug 日志泄漏 | ✅ 已修复 |
| 2 | `tui_model.go` | channel 关闭零值循环 | ✅ 已修复 |
| 3 | `tui_model.go` | Ctrl+D 无效 | ✅ 已修复 |
| 4 | `tui_model.go` | 第二次输入不执行 | ✅ 已修复 |
| 5 | `tui_model.go` + `agent.go` | callbacks 覆盖丢 IngestAssistant | ✅ 已修复 |
| 6 | `memory_bridge.go` | token 估算字节→rune | ✅ 已修复 |
| 7 | `openai.go` + `engine.go` | API usage 未返回 | ✅ 已修复 |
| 8 | `tui_model.go` + `event_sink.go` | Approval 审批 UI | ⏳ 待实施 |
| 9 | `tui_model.go` + `event_sink.go` | Sandbox ActiveType 显示 | ⏳ 待实施 |
