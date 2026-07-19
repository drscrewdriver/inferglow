# TUI / REPL / OneShot 三模式同构架构

## 核心思路

三种模式共享完全相同的 Agent 初始化管线（MemoryBridge → buildAgent → Run），差异仅在 **输入源** 和 **输出渲染**：

| 模式 | 输入源 | 输出渲染 | 审批 | 退出行为 |
|------|--------|----------|------|----------|
| TUI | Bubble Tea 键盘事件 | Alt-screen viewport | 交互审批卡片 | 持续运行 |
| REPL | bufio.Scanner stdin | fmt.Print 流式 token | 交互审批(当前无) | 持续运行 |
| OneShot | CLI 参数 prompt | 仅 final_response → stdout | 自动批准 | 单次后 exit |

## 架构分层

```
main.go (dispatch)
  │
  ├── buildRuntime(cfg, sessionID) → (agent, bridge, cleanup)   ← 共享
  │
  ├── --tui   → RunTUI(ctx, runtime)       [Bubble Tea]
  ├── --cli   → RunREPL(ctx, runtime)      [stdin/stdout loop]
  └── -z PROMPT → RunOneShot(ctx, runtime, prompt) [stdout only]
```

关键重构：提取 `buildRuntime()` 将 agent+bridge 初始化从三种模式中抽出，消除 [RunTUI](file:///home/joshua/Downloads/inferglow-workdir/inferglow/cli/tui_model.go#L162-L199) 和 [RunREPL](file:///home/joshua/Downloads/inferglow-workdir/inferglow/cli/repl.go#L40-L115) 中的重复初始化代码。

## 具体变更

### 1. 新建 `cli/runtime.go` — 共享运行时

```go
// AgentRuntime holds the shared agent + bridge + session for all modes.
type AgentRuntime struct {
    Agent     *agent.Agent
    Bridge    *MemoryBridge
    SessionID string
    Config    CLIConfig
    cancel    context.CancelFunc  // for bridge cleanup
}

// BuildRuntime creates the shared agent infrastructure.
func BuildRuntime(cfg CLIConfig, resumeID string) (*AgentRuntime, error) {
    sessionID := resumeID
    if sessionID == "" {
        sessionID = uuid.New().String()
    }
    bridge, err := NewMemoryBridge(cfg, sessionID)
    if err != nil {
        return nil, fmt.Errorf("init memory bridge: %w", err)
    }
    if cfg.Features.Constitutional && cfg.Constitutional != "" {
        entries, err := loadConstitutional(cfg.Constitutional)
        if err == nil {
            bridge.AppendConstitutional(entries)
        }
    }
    ag, err := buildAgent(cfg, bridge, sessionID)
    if err != nil {
        bridge.OnSessionEnd(context.Background())
        return nil, fmt.Errorf("init agent: %w", err)
    }
    return &AgentRuntime{
        Agent: ag, Bridge: bridge,
        SessionID: sessionID, Config: cfg,
    }, nil
}

func (r *AgentRuntime) Close(ctx context.Context) {
    r.Bridge.OnSessionEnd(ctx)
}
```

### 2. 新建 `cli/oneshot.go` — OneShot 模式

```go
// RunOneShot executes a single prompt and prints only the final response to stdout.
// Designed for scripts/pipes: no banner, no spinner, no tool previews.
func RunOneShot(ctx context.Context, cfg CLIConfig, prompt string) error {
    // Force auto-approve for non-interactive use.
    cfg.UnsafeMode = true

    runtime, err := BuildRuntime(cfg, "")
    if err != nil {
        return err
    }
    defer runtime.Close(ctx)

    // Suppress all logging to avoid corrupting stdout.
    log.SetOutput(io.Discard)

    // Build system prompt (same as REPL/TUI).
    sysPrompt := runtime.Bridge.BuildSystemPrompt(baseSystemPrompt, prompt)
    runtime.Bridge.IngestUser(prompt)

    // Run agent — collect final response, no streaming to stdout.
    resp, err := runtime.Agent.Run(ctx, prompt,
        agent.WithSystemPrompt(sysPrompt),
    )
    if err != nil {
        return fmt.Errorf("agent run: %w", err)
    }

    // Print ONLY the final response to stdout.
    if resp != "" {
        fmt.Print(resp)
        if !strings.HasSuffix(resp, "\n") {
            fmt.Println()
        }
    }
    return nil
}
```

### 3. 重构 `cli/repl.go` — 使用共享 Runtime

将 [RunREPL](file:///home/joshua/Downloads/inferglow-workdir/inferglow/cli/repl.go#L40-L115) 中的初始化代码替换为 `BuildRuntime()` 调用：

```go
func RunREPL(ctx context.Context, cfg CLIConfig, resumeID string) error {
    runtime, err := BuildRuntime(cfg, resumeID)
    if err != nil { return err }
    defer runtime.Close(ctx)

    // ... 其余 REPL 循环不变，使用 runtime.Agent / runtime.Bridge ...
}
```

### 4. 重构 `cli/tui_model.go` — 使用共享 Runtime

将 [RunTUI](file:///home/joshua/Downloads/inferglow-workdir/inferglow/cli/tui_model.go#L162-L199) 中的初始化代码替换为 `BuildRuntime()` 调用：

```go
func RunTUI(ctx context.Context, cfg CLIConfig, resumeID string) error {
    log.SetOutput(io.Discard)
    runtime, err := BuildRuntime(cfg, resumeID)
    if err != nil { return err }
    defer runtime.Close(ctx)

    m := newChatTUI(runtime.Agent, runtime.Bridge, cfg, runtime.SessionID)
    // ... 其余 Bubble Tea 启动不变 ...
}
```

### 5. 更新 `cli/cmd/inferglow-cli/main.go` — 三模式分发

```go
func main() {
    // ... 现有 flag 解析 ...
    oneshotPrompt := flag.String("z", "", "One-shot mode: send prompt, print final response, exit")
    oneshotLong := flag.String("oneshot", "", "Same as -z")
    // ...

    // 优先级: oneshot > tui > cli(REPL)
    prompt := *oneshotPrompt
    if prompt == "" { prompt = *oneshotLong }

    if prompt != "" {
        // OneShot: 单次执行 → stdout → exit
        if err := cli.RunOneShot(ctx, cfg, prompt); err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            os.Exit(1)
        }
        return
    }

    if !*cliMode {
        if err := cli.RunTUI(ctx, cfg, *resumeID); err != nil { ... }
        return
    }
    if err := cli.RunREPL(ctx, cfg, *resumeID); err != nil { ... }
}
```

### 6. 更新 `cli/config.go` — 新增 OutputMode

```go
type FeatureFlags struct {
    // ... 现有字段 ...
    OutputMode string `json:"output_mode"` // "tui", "cli", "oneshot"
}
```

## 使用方式

```bash
# TUI 模式（默认）
inferglow-cli

# REPL 模式
inferglow-cli --cli

# OneShot 模式（脚本/管道用）
inferglow-cli -z "解释这段代码的作用"
echo "fix this bug" | inferglow-cli -z -

# 管道组合
inferglow-cli -z "summarize README.md" | grep -i important
```

## 关键设计决策

1. **OneShot 不做流式输出** — 只输出 final_response，中间过程（tool calls、reasoning）全部静默。这与 hermes 的 `-z` 行为一致。
2. **OneShot 自动开启 UnsafeMode** — 非交互模式下无法等待用户审批，等同于 hermes 的 `HERMES_YOLO_MODE=1`。
3. **共享 Runtime 不强制共享生命周期** — 每种模式各自管理 context 和 cleanup，通过 `defer runtime.Close()` 统一。
4. **后续可扩展** — 共享 Runtime 使得未来增加 `--json` 输出格式、`--stdin` 管道模式等只需新增一个输出后端。
