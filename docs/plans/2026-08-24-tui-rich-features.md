# 计划：InferGlow TUI 富功能对齐（dsh-tui）— 多 Provider/多 Model 切换 + 思考等级 + 状态栏扩展

> 状态：**已实现（RF-1~RF-10 全部落地，cli + orchestrator/agent 构建/vet/单测通过）**
> 目标：让 inferglow TUI 达到 dsh-tui 的富功能水平：**运行时多 Provider / 多 Model 切换**、
> **思考等级（reasoning effort）控制**、`/theme` 主题切换、状态栏展示当前路由/思考等级/工作目录，
> 并持久化用户偏好（重启后保留）。
> 关联：命令兼容/联想计划见 `docs/plans/2026-08-24-slash-command-compat-autocomplete.md`（已实现 SC-1~SC-6）。

---

## 1. Goal（目标）

1. **多 Provider / 多 Model 运行时切换**：config 支持 `providers` 多路配置（每路 endpoint/model/api_key/provider）；
   `/model` 命令从「仅报告」升级为交互式选择器（provider → model 两级 + 最近使用列表）；
   切换**立即生效**（下一轮对话使用新路由），并持久化到 `~/.inferglow/model.json`。
2. **思考等级（reasoning effort）**：新增 `/effort` 命令（`/effort status` / `/effort <level>` / `/effort` 无参列级别）；
   通过 `ModelRequest.Options` 注入 `reasoning_effort`（OpenAI o 系列）与 `thinking`（MiMo/Stepfun 系），
   持久化到 `~/.inferglow/effort.json`；状态栏实时显示。
3. **主题切换**：`/theme` 从「识别但未实现」升级为真实切换（dark/light/auto + 列表），持久化。
4. **状态栏扩展**：底部状态栏显示 `provider/model · effort · workspace` 三元组（路由真实生效值，非配置残留）。
5. **输入历史持久化**：输入历史（当前仅内存 `submittedInputs`）落盘 `~/.inferglow/input_history.json`，重启可 ↑ 召回。
6. **轮次统计（RF-6）**：每轮记录**思考时间**、**工具调用数**、**每工具耗时**，随 `/receipt` 展示
   （`turnReceipt` 现有 duration/toolCalls 字段扩展）。
7. **TPS 输出效率（RF-7）**：流式期间实时计算 tokens/sec（字符数/4 估算），状态栏显示
   实时 gauge + 每轮 sparkline + avg60/p95 统计（镜像 dsh-tui `StatusMetrics.ts`）。
8. **缓存命中率（RF-8）**：从 `UsageInfo.PromptTokensDetails["cached_tokens"]` 计算
   `cacheRead/(input+cacheRead+cacheWrite)`，状态栏 + `/receipt` 显示。
9. **开屏欢迎页（RF-9）**：启动时首屏展示基本使用提示（tip 池：快捷键/命令/工作流/界面/避坑 分组，
   镜像 dsh-tui `tips.ts`），可 Esc 关闭；`/tips` 命令随时查看。
10. **API 健康检查（RF-10）**：周期性（默认 60s）对当前激活 endpoint 做轻量探活
    （本地 API：TCP 连接 + 可选 `GET /v1/models`），状态栏显示 `● online / ○ offline`；
    `/health` 命令手动触发 + 查看全部 provider 状态。
11. **可控**：新增配置开关 `features.model_switch`、`features.effort`、`features.theme_switch`、
    `features.input_history`、`features.turn_stats`、`features.tps`、`features.cache_hit`、
    `features.welcome`、`features.health_check`（默认 on）。

### 非目标（Non-goals）
- 不改 dsh-tui 源码（仅作功能/行为参考，见 §3 证据）。
- 不实现模型列表自动拉取（`GET /models`）；模型候选 = config `providers` 中声明的 model + `model.DEFAULT_SETTINGS` 静态目录。
- 不做 i18n 全量国际化、preset 预设包、`/tips`、自更新、`/scenes`、herdr（属 dsh-tui 外围，留待后续独立计划）。
- 不引入「Agent 工作模式」（default/plan/full）——inferglow 的 `/mode` 是上下文管理模式，语义不同，不混用。
- 不改变 `model` 包 Provider 实现（`reasoning_effort`/`thinking` 透传已具备，G1-03）。

---

## 2. Architecture（设计）

### 2.1 配置模型（`cli/config.go` 改）

`LLMConfig` 保持向后兼容（单路 `llm` 仍可用）；新增多路 `Providers`：

```go
// LLMConfig holds LLM endpoint configuration (unchanged, single-route form).
type LLMConfig struct {
    Endpoint string `json:"endpoint"`
    Model    string `json:"model"`
    APIKey   string `json:"api_key,omitempty"`
    Provider string `json:"provider,omitempty"`
}

// ProvidersConfig 多路 Provider 配置（RF-1）。
type ProvidersConfig struct {
    // Active 当前激活的 provider 键（省略 = 用 ~/.inferglow/model.json 或 llm 单路）。
    Active string                  `json:"active,omitempty"`
    // List key → 该 provider 的完整 LLM 配置；key 同时作为路由的 provider 名。
    List   map[string]LLMConfig    `json:"list,omitempty"`
}
```

`CLIConfig` 增加字段（`llm` 保留不动）：

```go
Providers  ProvidersConfig `json:"providers,omitempty"` // RF-1: 多路 provider
```

`TUIConfig` 增加：

```go
ReasoningEffort string `json:"reasoning_effort,omitempty"` // RF-2: "low"|"medium"|"high"|""
```

`FeatureFlags` 增加（§2.8）：

```go
ModelSwitch   bool `json:"model_switch"`   // RF-1: runtime multi-provider/model switching
EffortControl bool `json:"effort"`         // RF-2: /effort reasoning-level control
ThemeSwitch   bool `json:"theme_switch"`   // RF-3: /theme real switching
InputHistory  bool `json:"input_history"`  // RF-5: persisted input history
```

**兼容规则**：`providers.list` 为空且 `llm` 有值 → 单路模式（现状行为）；`providers.list` 非空 →
`llm` 被忽略（以 providers 为准）。迁移：`init_wizard` 写单路 `llm`，不改。

### 2.2 模型路由解析 + 持久化（`cli/tui_model_route.go` 新建）

镜像 dsh-tui `modelRoute.ts`（issue #67）的**原子路由**语义：任何来源要么提供完整的
`(provider, model)` 对，要么被跳过——绝不合并半对。解析顺序：

1. config 显式完整对（`providers.active` 指向的 List 项含 model，或单路 `llm` 两字段齐全）→ 整对生效；
2. 否则 `~/.inferglow/model.json` 持久化的 `{provider, model}` → 整对生效；
3. 否则默认路由（单路 `llm` 若存在 → 取之；多路 → `providers.active` 或首个 List 项；全空 → `deepseek/deepseek-chat`）。

```go
// ModelRoute 一个完整模型路由（provider + model 原子对）。
type ModelRoute struct {
    Provider string
    Model    string
    // Endpoint/APIKey 从 config providers.list[Provider] 或单路 llm 填充。
    Endpoint string
    APIKey   string
}

// resolveModelRoute 解析生效路由（原子：整对覆盖，半对忽略）。
// cfg 非空 provider 键 → 查 providers.list；否则查单路 llm。
func resolveModelRoute(cfg CLIConfig, pref *ModelPref) (ModelRoute, bool)

// ModelPref 持久化路由 ~/.inferglow/model.json。
type ModelPref struct {
    Provider string `json:"provider"`
    Model    string `json:"model"`
}

func readModelPref() *ModelPref            // 损坏/缺失 → nil（非致命）
func writeModelPref(p ModelPref)           // 失败静默
```

**Effort 持久化**（镜像 dsh-tui `effortPrefs.ts`，`~/.inferglow/effort.json`）：

```go
type EffortPref struct {
    Effort string `json:"effort"` // "" = provider default
}
func readEffortPref() string  // 损坏 → ""
func writeEffortPref(e string) // 失败静默
```

### 2.3 运行时切换机制（`orchestrator/agent/agent.go` + `engine.go` 改）

现状：`Agent` 构造时持有 `modelReq model.ModelRequester`，`Agent.Run` 每轮都用它；无法在运行时换模型。

方案：新增两个 RunOption（零行为变化，默认 nil）：

```go
// WithModelRequester 为该轮 Run 指定模型请求器（覆盖 Agent 构造时注入的）。
// 用于 /model 运行时切换：每次 submitTurn 传入当前路由构造的 requester。
func WithModelRequester(mr model.ModelRequester) RunOption

// WithModelOptions 为该轮所有 ModelRequest.Options 追加键值（与 engine 内建的
// max_tokens/force_json 合并，调用方键优先）。用于 /effort 注入
// reasoning_effort / thinking，也兼容未来参数。
func WithModelOptions(opts map[string]any) RunOption
```

`runConfig` 增加 `modelRequester model.ModelRequester` 与 `modelOptions map[string]any`。
`Agent.Run` 将两者随 engine 构造/调用传递（`executeFlow` 链路）；`engine.go` 的
`req.Options` 构造处（engine.go:502）合并：

```go
Options: func() map[string]any {
    base := map[string]any{"max_tokens": 16384} // 有工具时
    if hasTools == false {
        base = map[string]any{"force_json": true}
    }
    for k, v := range e.modelOptions { // nil map 直接跳过
        base[k] = v
    }
    return base
}(),
```

> 注：engine 的 `modelRequester` 覆盖点需在 `Agent.Run` 创建 Engine 处传入；若 Engine 被复用缓存，
> 需复核 `executeFlow`/`NewEngine` 调用点（Task 3 明确）。`model.StreamRequester` 即可满足 Engine 依赖。

### 2.4 `/model` 交互式选择器（`cli/tui_model_picker.go` 新建）

`/model` 命令升级（替换 `tui_compat.go` 的 `tuiHandleModel` 报告版）：

| 输入 | 行为 |
|---|---|
| `/model` | 进入选择器：第一级 = provider 列表（config providers + 静态目录），↑/↓ 选，Enter 确认 |
| `/model <provider>` | 第二级 = 该 provider 的模型列表（`DEFAULT_SETTINGS[provider].model` + config model），选中即切换 |
| `/model <provider> <model>` | 直接切换 + 持久化 |
| `/model status` | 报告当前生效路由（解析后值，非配置残留） |
| `/model recents` | 显示最近使用的模型路由（`~/.inferglow/model_recents.json`，最多 5 条） |
| Esc | 取消选择器 |

选择器状态机（复用 `cli/tui_completion.go` 的弹窗渲染模式）：

```go
type modelPicker struct {
    active   bool
    level    int        // 0=provider, 1=model
    providers []string  // level 0 候选
    models    []string  // level 1 候选
    selected  int
    recents  []ModelPref // 最近使用
}
```

**切换生效**：`chatTUI` 新增字段 `route ModelRoute` 与 `requester model.ModelRequester`
（构造时 `buildModelRequester` 初始化）。切换时：

```go
// tuiHandleModelSet：校验 provider 存在 → 构造新 requester（buildModelRequester(tmpCfg)）
// → 更新 m.route / m.requester / m.modelLabel → writeModelPref → 更新 recents。
// 下一轮 submitTurn 的 runOpts 追加 agent.WithModelRequester(m.requester)。
```

`submitTurn` 修改（tui_model.go:1009 处 runOpts）：

```go
runOpts := []agent.RunOption{
    agent.WithSystemPrompt(sysPrompt),
    agent.WithCallbacks(mergedCB),
    agent.WithModelRequester(m.requester),   // RF-1: 运行时路由
    agent.WithModelOptions(m.effortOptions()), // RF-2: 思考等级注入
}
```

`m.effortOptions()` 按当前 effort 生成：

```go
func (m *chatTUI) effortOptions() map[string]any {
    switch m.effort {
    case "", "auto":
        return nil // provider default，不注入
    case "low", "medium", "high":
        return map[string]any{"reasoning_effort": m.effort}
    }
    return nil
}
```

> 注：`thinking.type` 注入（MiMo/Stepfun）与 `reasoning_effort` 互斥由 provider 决定；plan 只注入
> `reasoning_effort`，`thinking` 透传路径已存在（G1-03），如需 per-provider 映射在 Task 5 决定。

### 2.5 `/effort` 命令（`cli/tui_commands.go` + `cli/tui_effort.go` 新建）

| 输入 | 行为 |
|---|---|
| `/effort` | 无参：列出可用级别 + 当前值 |
| `/effort status` | 报告当前生效级别（含「provider default」态） |
| `/effort low` / `medium` / `high` | 设置 + 持久化 + 状态栏更新 |
| `/effort auto` | 恢复 provider 默认（清除偏好） |

级别定义：`low` / `medium` / `high` / `auto`（=不注入）。注册进 `buildSlashRegistry`
（`cfg.Features.EffortControl` 门控），同时在 `tui_compat.go` 的 `thinking` 兼容项
（当前「识别未实现」）改为映射到 `/effort` handler（opencode 的 `/thinking` 语义对齐）。

### 2.6 `/theme` 真实实现（`cli/tui_theme.go` 改）

现状：`tui_theme.go` 有 `activeTheme` 与 `tui_theme` 系列（dark/light 配色），`/theme` 是 compat stub。
升级为真实命令：

| 输入 | 行为 |
|---|---|
| `/theme` | 无参：列出可用主题 + 当前 |
| `/theme dark` / `light` | 切换 + 持久化 `~/.inferglow/theme.json` |
| `/theme auto` | 按终端背景自动（探测 `COLORFGBG` / 平台 API），持久化 |

实现：`applyTheme(name string) error` 更新 `activeTheme`（当前 `tui_theme.go` 的全局），
并重绘输入框/状态栏样式（`applyTextareaTheme` 复用）。持久化键 `TUIConfig.Theme` 或独立文件（取独立文件
`~/.inferglow/theme.json`，避免与 config 文件写竞争）。

### 2.7 状态栏扩展（`cli/tui_footer.go` 或 `tui_model.go` View 改）

现状：状态栏有 `modelLabel`（仅 `cfg.LLM.Model`）。扩展为三元组：

```
 provider/model · effort · 📁 workspace
```

- `provider/model`：`m.route.Provider + "/" + m.route.Model`（解析后真实值）。
- `effort`：`m.effort` 为空 → `effort:auto`，否则 `effort:medium` 等。
- `📁 workspace`：`m.workspace.GetCurrentDir()` 的 basename（过长截断，复用 `tui_workspace.go`）。
- 窄终端（宽度 < 100）降级：只显示 `model · effort`。

### 2.8 输入历史持久化（`cli/tui_input_history.go` 新建）

现状：`m.submittedInputs []string`（内存）+ `submittedCursor`（↑ 召回）。
扩展：每次 `submitTurn` 追加后异步写 `~/.inferglow/input_history.json`（NDJSON，上限 200 条）；
`newChatTUI` 启动时加载。`cfg.Features.InputHistory` 门控。写失败静默（与 workspace history 同策略）。

### 2.9 配置开关汇总（`cli/config.go`）

```go
ModelSwitch   bool `json:"model_switch"`   // RF-1: /model 选择器 + 运行时切换（default true）
EffortControl bool `json:"effort"`         // RF-2: /effort（default true）
ThemeSwitch   bool `json:"theme_switch"`   // RF-3: /theme（default true）
InputHistory  bool `json:"input_history"`  // RF-5: 输入历史持久化（default true）
TurnStats     bool `json:"turn_stats"`     // RF-6: 轮次统计（思考/工具耗时）（default true）
TPS           bool `json:"tps"`            // RF-7: TPS 输出效率（default true）
CacheHit      bool `json:"cache_hit"`      // RF-8: 缓存命中率（default true）
Welcome       bool `json:"welcome"`        // RF-9: 开屏欢迎页（default true）
HealthCheck   bool `json:"health_check"`   // RF-10: API 健康检查（default true）
```

`DefaultCLIConfig` 全部 true；关闭后：`/model` 回退报告版、`/effort`/`/theme` 不注册、输入历史仅内存、
统计/欢迎页/健康检查不渲染不探活。

### 2.10 轮次统计（RF-6，`cli/tui_stats.go` 新建）

现状：`turnReceipt`（tui_receipt.go）已有 `turnNum/duration/llmRounds/toolCalls/promptTokens/completionTokens`。
扩展为：

```go
type turnReceipt struct {
    turnNum          int
    duration         int    // 秒（整轮）
    llmRounds        int
    toolCalls        int
    promptTokens     int
    completionTokens int
    // RF-6 新增：
    thinkingMs       int64  // 本轮思考总时长（EventReasoning 首尾时间差累计）
    toolDurationsMs  map[string]int64 // 工具名 → 累计耗时（EventToolCallStart/End 时间差）
    reasoningTokens  int    // 推理 token（UsageInfo.ReasoningTokens()，G1-06 已有）
    totalOutputChars int    // 流式输出总字符（供 TPS 复用）
}
```

**数据来源**：TUI 事件循环（`ingestEvent`，tui_model.go）已有 `EventReasoning`/`EventToken`/
`EventToolCallStart`/`EventToolCallEnd`；记录时间戳即可，无需改 agent 层。

**渲染**：`/receipt` 输出扩展（`tui_receipt.go`）：

```
  Turn #3 · 12s
  LLM rounds: 2 · Tools: 4 (bash 2.1s, file_read 0.4s, ...)
  Tokens: in 1,234 · out 567 · reasoning 345
  Thinking: 3.2s · Output: 12.4 chars/s (TPS)
```

### 2.11 TPS 输出效率（RF-7，`cli/tui_stats.go` 续）

镜像 dsh-tui `StatusMetrics.ts`：

```go
type tpsTracker struct {
    current    float64           // 当前轮实时 tps
    samples    []tpsSample       // 每轮样本（≤500）
    lastFirstToken time.Time     // 本轮首 token 时刻
    lastTokens     int           // 已累计输出 token 估算
}

type tpsSample struct {
    tps float64
    at  int64 // unix ms
}
```

- **采样**：`EventToken` 累计字符（`totalOutputChars`），tps = `(chars/4) / elapsed_s`
  （elapsed 从首 token 到当前；与 dsh-tui `outputChars / 4` 估算一致，channel.ts:6026）。
- **每轮结算**：`EventRunEnd` 时 push `{tps, now}`，截断 500 条。
- **渲染（状态栏）**：流式中显示 `▕▏gauge + N tps`（gauge 1/8 格，参照 `renderTpsGauge`）；
  空闲显示最近样本 sparkline（`▁▃▅▇`，参照 `renderTpsSparkline`）；
  `/tps` 命令显示 `tps N · avg60 X · mean Y · p95 Z`（参照 `tpsStats`）。
- **色阶**：≥50 绿、≥20 黄、<20 红（`speedColor` 同构）。

### 2.12 缓存命中率（RF-8，`cli/tui_stats.go` 续）

镜像 dsh-tui `formatCacheHitRate`（StatusLine.tsx:595）：

```go
// cacheHitRate 由 UsageInfo 计算缓存命中率；usage 缺失或 total<=0 → 无值。
func cacheHitRate(u *model.UsageInfo) (rate float64, cacheRead, cacheWrite int, ok bool) {
    if u == nil { return 0, 0, 0, false }
    cacheRead = u.PromptTokensDetails["cached_tokens"]   // 部分 provider 用 cached_tokens 键
    cacheWrite = u.PromptTokensDetails["cache_creation"] // 部分 provider 用 cache_creation 键
    total := u.PromptTokens + cacheRead + cacheWrite
    if total <= 0 { return 0, 0, 0, false }
    return float64(cacheRead) / float64(total) * 100, cacheRead, cacheWrite, true
}
```

- 状态栏 `cache` 字段：`cache 42.3%`（`features.cache_hit` 门控）。
- `/receipt` 追加行：`Cache hit 42.3% · read 12.3k · write 3.1k`（参照 dsh-tui i18n
  `cost-cache-hit-rate`：「缓存命中率 {{rate}}% · 缓存 {{read}} 读 / {{write}} 写」）。
- 当前 `UsageInfo.PromptTokensDetails` 已承载 `cached_tokens`（chat.go:140），无需改 model 包。

### 2.13 开屏欢迎页（RF-9，`cli/tui_welcome.go` 新建）

镜像 dsh-tui `tips.ts` + `LogoV2` 首屏：

```go
// welcomeTip 一条使用提示。
type welcomeTip struct {
    id    string
    group string // "keys" | "commands" | "workflow" | "display" | "pitfalls"
    text  string
}

// welcomeTips 内置提示池（§3.3 列举；随功能扩展追加）。
var welcomeTips = []welcomeTip{ ... }

// tuiWelcome 开屏状态。
type tuiWelcome struct {
    visible bool
    page    int   // 分页（每屏 ≤5 条）
    pageMax int
}
```

**行为**：
- `newChatTUI` 后首帧显示欢迎页（`features.welcome` 门控；`firstRun` 标志位写入
  `~/.inferglow/welcome_seen.json`，仅首启显示，之后 `/welcome` 手动唤起）。
- 欢迎页布局（置于 transcript 顶部，不挤占输入区）：
  ```
  ┌─ InferGlow ─────────────────────────────┐
  │  欢迎！5 条快速上手                       │
  │  · /help 查看全部命令                    │
  │  · /model 切换模型 · /effort 调整思考等级 │
  │  · ↑ 召回历史 · /workspace 切换目录      │
  │  · /tips 随时查看提示 · Esc 关闭         │
  └──────────────────────────────────────────┘
  ```
- 键：`Esc`/`q` 关闭；`Tab` 翻页（多页时）；`/tips` 命令随时重开（`/tips <group>` 过滤分组）。
- 提示池按 `welcomeTips` 分组轮换，每条 ≤60 字符（窄终端截断）。

### 2.14 API 健康检查（RF-10，`cli/tui_health.go` 新建）

现状：`model_factory.go` 的 `compressModelAdapter.Available()` 已有轻量探测模式
（`GenerateRequestData{Input:"ping"}` 3s 超时）；`examples/integration_demo.go` 有「Reply OK」健康检查样例。
升级为 TUI 周期探活：

```go
type providerHealth struct {
    key      string // provider 键（"deepseek"/"openai"/…）
    endpoint string
    ok       bool
    latency  time.Duration
    lastCheck time.Time
    err      string
}

type healthChecker struct {
    active   bool
    interval time.Duration // 默认 60s；config health_check_interval
    entries  map[string]*providerHealth
    checking bool // 防止重入
}
```

**探活方式**（按 endpoint 判定）：
1. **本地 API**（endpoint host 为 `localhost`/`127.0.0.1`/`::1`/内网）：`net.DialTimeout("tcp", host:port, 2s)` 连 TCP，
   可选 `GET /v1/models`（带 APIKey 时）验证 200——只做 TCP 层可减少对本地服务压力；
2. **远程 API**：仅做 `GenerateRequestData` 轻量构造探测（不真发请求），避免消耗配额；
   或 `HEAD /v1/models` 2s 超时（可配置 `health_probe_mode: "tcp"|"http"|"off"`）。
3. 结果缓存到 `entries`，状态栏显示 `● deepseek (42ms)` / `○ openai (timeout)`。

**命令**：
| 输入 | 行为 |
|---|---|
| `/health` | 手动触发全部 provider 探活 + 报告 |
| `/health <key>` | 探活单个 provider |
| `/health interval <sec>` | 动态调整周期（默认 60s，范围 10~600） |

**定时器**：`chatTUI.Update` 的 `tea.Tick(interval)`（bubbletea 原生）驱动；TUI 空闲时探活，
运行轮次中跳过（`state != tuiIdle` 不探，避免抢占带宽）。探活失败**不阻断**请求（仅提示 + 状态栏变红）。

---

## 3. 规格证据（收集）

### 3.1 dsh-tui（`ccch1mneyyy/dsh-TUI`，克隆自 ghfast.top 镜像）
- **`src/modelPrefs.ts`**：持久化路由 `~/.dsh-tui/model.json` = `{provider, model}`；
  损坏/缺失 → 回退 harness 默认。**inferglow 采用同构**（`~/.inferglow/model.json`）。
- **`src/modelRoute.ts`**（issue #67）：`resolveModelRoute(configured, pref, defaults)` 原子解析——
  cordis.yml 完整对 > 持久化偏好 > 默认；半对配置**忽略不合并**。**inferglow 的 §2.2 镜像此语义**。
- **`src/effortPrefs.ts`**：持久化 `~/.dsh-tui/effort.json` = `{effort}`；`/effort`（slider 或
  `/effort <id>`、`/effort status`）。**inferglow 采用同构**。
- **`src/modelGroups.ts` / `src/modelRecents.ts`**：模型分组与最近使用。inferglow 取「recents」子集。
- **`src/themePrefs.ts` / `src/workspaces.ts` / `src/history.ts`**：主题偏好 / 工作空间 / 输入历史持久化。
  inferglow 工作空间已实现（SC-5）；主题与输入历史按 §2.6/§2.8 对齐。
- **`src/screens/StatusMetrics.ts` + `src/screens/StatusLine.tsx`（RF-7/RF-8 证据）**：
  - `tps`/`tpsSamples`（channel.ts，`outputChars/4` 估算 token，首 token 到结算耗时）——
    **inferglow §2.11 镜像**；
  - 状态栏 tps 字段三态：流式中 gauge（`renderTpsGauge` 1/8 格）、空闲 sparkline（`renderTpsSparkline`）、
    纯数字（`N t/s`）；`tpsStats` 给 avg60/mean/p95；`speedColor` 色阶 ≥50 绿 / ≥20 黄 / <20 红——
    **inferglow §2.11 镜像**；
  - `formatCacheHitRate`（`cacheRead/(input+cacheRead+cacheWrite)`，usage 缺失返回空）——
    **inferglow §2.12 镜像**；
  - i18n `cost-cache-hit-rate`：「缓存命中率 {{rate}}% · 缓存 {{read}} 读 / {{write}} 写」——
    **inferglow `/receipt` 行文案参照**。
- **`src/components/messages/AssistantThinkingMessage.tsx` + `AssistantToolUseMessage.tsx`（RF-6 证据）**：
  思考行 ` · ${formatDuration(durationMs)}`（≥1s 才显示）、工具卡 ` · ${formatDuration(elapsedMs)}` 实时秒表；
  `SubagentCard` 显示 `· N tools` 工具数——**inferglow §2.10 镜像**。
- **`src/tips.ts` + `LogoV2`（RF-9 证据）**：启动首屏轮换 tip（`TIP_GROUP_LABELS`：keys/commands/workflow/
  display/pitfalls，单条 ≤60 字符），与 `/tips` 面板共用——**inferglow §2.13 镜像**。
- **非目标参照**：`sessionModes.ts`（default/plan/full，Shift+Tab）、`i18n.ts`、`presetPrefs.ts`、
  `minimalMode.ts`、`update.ts`、`scenes.ts`、`herdr.ts` — 不在本计划范围。

### 3.2 inferglow 现状（基线，读自工作区源码）
- `cli/config.go`：单路 `LLMConfig`（endpoint/model/api_key/provider）；`FeatureFlags` 已有 SC-1~SC-6。
- `cli/model_factory.go`：`buildModelRequester(cfg)` 支持 15+ provider 构造（openai/deepseek/anthropic/
  qwen/glm/kimi/mimo/stepfun/baidu/spark/sensenova/tencent/ollama…），**已具备多 provider 构造能力**，
  缺的是「多路配置 + 运行时切换」。
- `model/model.go`：`ModelRequest.Options map[string]any` 已存在；`model/config.go` G1-03 已保证
  `reasoning_effort` / `thinking` 透传（`openai_reasoning_content_test.go` 有测试）。
- `orchestrator/agent`：`Agent` 构造时注入 `modelReq`；`engine.go:492` 每轮构造 `ModelRequest` 时
  写 `Options`（`max_tokens`/`force_json`）——**无每轮 Options/Requester 覆盖点**（本计划 Task 3 补）。
- `cli/tui_compat.go:174` `tuiHandleModel`：仅报告，提示「edit llm.model and restart」——**待升级**。
- `cli/tui_theme.go`：`activeTheme` + dark/light 配色已存在；`/theme` 为 compat stub。
- `cli/tui_model.go`：`submittedInputs` 内存历史；`modelLabel = cfg.LLM.Model`；`submitTurn` runOpts 可扩展。

### 3.3 静态模型目录（模型候选来源）
`model/config.go` `DEFAULT_SETTINGS` 已含各 provider 默认 model（openai=gpt-4、deepseek=deepseek-chat、
qwen=qwen-max、glm=glm-4、kimi=moonshot-v1-8k、mimo=mimo-v2.5-pro…）。`/model` 第二级候选 =
`DEFAULT_SETTINGS[provider].model`（若有）+ config 该 provider 的 model 去重。

### 3.4 统计与健康检查基线（inferglow 已有能力）
- `cli/tui_receipt.go`：`turnReceipt` 已有 `turnNum/duration/llmRounds/toolCalls/promptTokens/completionTokens`——RF-6 直接扩展。
- `model/chat.go:135`：`UsageInfo` 已有 `PromptTokensDetails map[string]int`（`cached_tokens` 键）——
  RF-8 缓存命中率数据源已具备。
- `model/chat.go:153`：`UsageInfo.ReasoningTokens()`（G1-06）——RF-6 推理 token 直接复用。
- `cli/tui_model.go` `ingestEvent`：`EventReasoning`/`EventToken`/`EventToolCallStart`/`EventToolCallEnd`
  已分流——RF-6/RF-7 时间戳记录点已具备。
- `model_factory.go:125` `Available()`：`GenerateRequestData{Input:"ping"}` 3s 探测——RF-10 可复用/升级。
- `examples/integration_demo.go:282`：health-check 模式（「Reply with exactly 'OK'」）——RF-10 兜底探测样例。

---

## 4. Baseline / Authority 约束

- 不改 `codex/`、不改 dsh-tui 源码。`model` 包**不新增 provider**，仅 CLI 层接线。
- `orchestrator/agent` 改动**零行为变化**（新增可选 RunOption，默认 nil；回归跑 agent 全量测试）。
- `llm` 单路配置语义不变；`providers` 非空才走多路。
- `/mode`（上下文管理）与 `/model`（模型路由）职责分离，互不干扰。

---

## 5. Compatibility Boundary（兼容边界）

- config：`llm` 字段保留（单路模式）；`providers` 缺省 = 单路行为完全不变。
- 命令：`/model` 无参行为从「报告」变为「打开选择器」——文档标注为行为升级；
  `/model status` 保留报告能力（无感迁移）。
- 输入：`reasoning_effort` 注入仅当 effort 非空且非 auto；provider 不支持时由 provider 侧忽略（G1-03 透传，不报错）。
- 持久化文件均为新增（`model.json`/`effort.json`/`theme.json`/`input_history.json`），与现有
  `config.json`/`workspace_history.json` 互不干扰；写失败一律静默降级。

---

## 6. TDD Route

```text
TDD Route:
- Mode: auto
- Decision: light
- Strict authority: not applicable（无显式 user/project 严格 TDD 请求）
- Test posture: 路由解析/持久化的单元测试 + agent RunOption 回归 + 构建/vet/test
- Reason: 核心改动是纯函数（resolveModelRoute/effortOptions/持久化读写）与可选 RunOption；UI 手动烟测
- Verification: go test ./...（cli + orchestrator/agent）+ go vet ./...
```

---

## 7. File Map

| 文件 | 动作 |
|---|---|
| `cli/config.go` | 改：`ProvidersConfig`、`TUIConfig.ReasoningEffort`、FeatureFlags 9 项（含 RF-6~RF-10） |
| `cli/tui_model_route.go` | 新：`ModelRoute`、`resolveModelRoute`、`read/writeModelPref`、`read/writeEffortPref`、recents 读写 |
| `cli/tui_model_route_test.go` | 新：路由解析优先级/半对忽略/持久化损坏降级测试 |
| `cli/tui_model_picker.go` | 新：`modelPicker` 选择器状态机（两级 + recents） |
| `cli/tui_effort.go` | 新：`tuiHandleEffort` 命令 handler + `effortOptions()` |
| `cli/tui_commands.go` | 改：`buildSlashRegistry` 注册 `/model`（替换 compat 版）/`/effort`/`/theme`/`/tips`/`/health`/`/tps`；`tui_compat.go` 的 `thinking` 映射到 effort |
| `cli/tui_model.go` | 改：`chatTUI` 加 `route`/`requester`/`effort`/`picker`/`history`/`stats`/`welcome`/`health` 字段；`submitTurn` runOpts 追加两个 RunOption；启动加载持久化；状态栏三元组 + tps/cache/health 字段 |
| `cli/tui_theme.go` | 改：`applyTheme(name)` + 主题持久化读写 |
| `cli/tui_input_history.go` | 新：输入历史落盘/加载（NDJSON，上限 200） |
| `cli/tui_input_history_test.go` | 新：追加/去重/上限/损坏加载测试 |
| `cli/tui_stats.go` | 新：RF-6 轮次统计扩展 + RF-7 `tpsTracker` + RF-8 `cacheHitRate` + 渲染 |
| `cli/tui_stats_test.go` | 新：tps 采样/结算、cacheHitRate 计算、receipt 渲染测试 |
| `cli/tui_welcome.go` | 新：RF-9 `welcomeTips` 池 + `tuiWelcome` 状态 + 首屏渲染 + `/tips` |
| `cli/tui_welcome_test.go` | 新：tip 分组/分页/首启标志测试 |
| `cli/tui_health.go` | 新：RF-10 `healthChecker` + 探活（TCP/HTTP）+ `/health` 命令 |
| `cli/tui_health_test.go` | 新：本地/远程判定、超时降级、周期调整测试 |
| `orchestrator/agent/agent.go` | 改：`runConfig` 加 `modelRequester`/`modelOptions`；新增 `WithModelRequester`/`WithModelOptions` |
| `orchestrator/agent/engine.go` | 改：`req.Options` 合并 `e.modelOptions`；Engine 构造处接受 per-run requester |
| `orchestrator/agent/agent_options_test.go` | 新/改：两个 RunOption 的合并/覆盖/默认 nil 回归 |
| `docs/guides/tui-build-and-config.md` | 改：features 表补 9 项；命令表补 `/model` `/effort` `/theme` `/tips` `/health` `/tps` 新行为 |

---

## 8. Tasks

### Task 1 — 配置模型扩展
Files: `cli/config.go`

Change Necessity: 多路 provider 需要新配置载体；单路 `llm` 保持兼容（最小边界 = 新增字段）。

Steps:
1. 在 `LLMConfig` 后新增 `ProvidersConfig`（见 §2.1）。
2. `CLIConfig` 增加 `Providers ProvidersConfig \`json:"providers,omitempty"\``。
3. `TUIConfig` 增加 `ReasoningEffort string \`json:"reasoning_effort,omitempty"\``。
4. `FeatureFlags` 增加 `ModelSwitch`/`EffortControl`/`ThemeSwitch`/`InputHistory`（均 `json:"..."` 命名见 §2.8）。
5. `DefaultCLIConfig` 四项默认 true。
6. 自测（`go vet ./...` + `go build ./...`）。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go vet ./... && go build ./...`

### Task 2 — 路由解析 + 持久化
Files: `cli/tui_model_route.go`（新）、`cli/tui_model_route_test.go`（新）

Change Necessity: 运行时切换需要「原子路由解析 + 重启保留」，纯函数实现，独立可测。

Steps:
1. 新建 `cli/tui_model_route.go`：
```go
package cli

// ModelRoute 一个完整模型路由。
type ModelRoute struct {
    Provider string
    Model    string
    Endpoint string
    APIKey   string
}

// ModelPref 持久化路由。
type ModelPref struct {
    Provider string `json:"provider"`
    Model    string `json:"model"`
}

const (
    modelPrefFile     = "model.json"
    effortPrefFile    = "effort.json"
    modelRecentsFile  = "model_recents.json"
    defaultProvider   = "deepseek"
    defaultModel      = "deepseek-chat"
)

// resolveModelRoute 解析生效路由（原子：完整对覆盖；半对忽略；pref 优先于默认）。
func resolveModelRoute(cfg CLIConfig, pref *ModelPref) ModelRoute {
    if cfg.Providers.Active != "" {
        if p, ok := cfg.Providers.List[cfg.Providers.Active]; ok && p.Model != "" {
            return ModelRoute{Provider: cfg.Providers.Active, Model: p.Model, Endpoint: p.Endpoint, APIKey: p.APIKey}
        }
    }
    if len(cfg.Providers.List) > 0 {
        // 无 active 或 active 无 model → 首个完整项
        for key, p := range cfg.Providers.List {
            if p.Model != "" {
                return ModelRoute{Provider: key, Model: p.Model, Endpoint: p.Endpoint, APIKey: p.APIKey}
            }
        }
    }
    if cfg.LLM.Model != "" {
        prov := cfg.LLM.Provider
        if prov == "" { prov = "openai" }
        return ModelRoute{Provider: prov, Model: cfg.LLM.Model, Endpoint: cfg.LLM.Endpoint, APIKey: cfg.LLM.APIKey}
    }
    if pref != nil && pref.Provider != "" && pref.Model != "" {
        return ModelRoute{Provider: pref.Provider, Model: pref.Model}
    }
    return ModelRoute{Provider: defaultProvider, Model: defaultModel}
}

// routeConfig 由 ModelRoute 还原 CLIConfig（供 buildModelRequester 构造 requester）。
func (r ModelRoute) routeConfig() CLIConfig {
    return CLIConfig{LLM: LLMConfig{Endpoint: r.Endpoint, Model: r.Model, APIKey: r.APIKey, Provider: r.Provider}}
}
```
2. `readModelPref()`/`writeModelPref(p)`：JSON 读写 `DataDir/model.json`，损坏 → nil，写失败静默。
3. `readEffortPref() string`/`writeEffortPref(e string)`：`effort.json` 同策略。
4. `readModelRecents() []ModelPref`/`pushModelRecent(p)`：`model_recents.json`，去重、最多 5 条、最新在前。
5. 测试 `tui_model_route_test.go`：
   - `resolveModelRoute`：多路 active 命中 / active 无 model 落到首个 / 单路 llm / 全空回默认 / pref 兜底；
   - 半对 pref（缺 provider 或 model）被忽略；
   - `writeModelPref` → `readModelPref` 回环；损坏 JSON → nil 不 panic；
   - `pushModelRecent` 去重 + 上限 5。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'ModelRoute|ModelPref|EffortPref|ModelRecent' ./... && go vet ./...`

### Task 3 — Agent 运行时 requester / options 注入
Files: `orchestrator/agent/agent.go`、`orchestrator/agent/engine.go`、`orchestrator/agent/agent_options_test.go`（新）

Change Necessity: 运行时换模型/注入思考等级需要每轮覆盖点；新增可选 RunOption 零行为变化。

Steps:
1. `agent.go`：`runConfig` 增加 `modelRequester model.ModelRequester` 与 `modelOptions map[string]any`。
```go
// WithModelRequester 为该轮 Run 覆盖模型请求器（/model 运行时切换）。
func WithModelRequester(mr model.ModelRequester) RunOption {
    return func(c *runConfig) { c.modelRequester = mr }
}

// WithModelOptions 为该轮 ModelRequest.Options 追加键值（/effort 注入）。
func WithModelOptions(opts map[string]any) RunOption {
    return func(c *runConfig) { c.modelOptions = opts }
}
```
2. 复核 `Agent.Run`（agent.go:377）构造 Engine 的调用点：`modelRequester` 非 nil 时用其替换
   `a.modelReq`（Engine 构造参数）；`modelOptions` 传入 Engine（新字段 `modelOptions map[string]any`）。
3. `engine.go`：`Engine` 增加 `modelOptions map[string]any` 字段与 setter（或经构造参数）；
   在 `req.Options` 构造处（engine.go:502）合并（见 §2.3 代码）。
4. 测试 `agent_options_test.go`：
   - 默认（无 RunOption）→ 行为与现状一致（engine 用 Agent 构造时的 requester；Options 只含内建键）；
   - `WithModelOptions` → 请求 Options 含 `reasoning_effort:"high"`（用 mockModelRequester 捕获 req）；
   - `WithModelRequester` → 本轮使用新 requester 的响应（scriptedModelRequester 两条脚本可区分）。

Verify: `cd E:\test\rewrite-agently\inferglow-github\orchestrator\agent && go test ./... && go vet ./...`

### Task 4 — `/model` 选择器 + 运行时切换
Files: `cli/tui_model_picker.go`（新）、`cli/tui_commands.go`、`cli/tui_compat.go`、`cli/tui_model.go`

Change Necessity: `/model` 从报告升级为切换；`chatTUI` 需持当前路由与 requester，每轮注入。

Steps:
1. 新建 `cli/tui_model_picker.go`：
```go
type modelPicker struct {
    active    bool
    level     int        // 0=provider, 1=model
    providers []string
    models    []string
    selected  int
    recents   []ModelPref
}
// Enter(providers) 进入第一级；NextLevel(models) 进入第二级；Move(delta)；Cancel()。
```
2. `cli/tui_model.go`：`chatTUI` 增加字段（构造时初始化）：
```go
route     ModelRoute
requester model.ModelRequester // 构造时 buildModelRequester(resolveModelRoute(cfg, readModelPref()).routeConfig())
effort    string               // readEffortPref() 或 cfg.TUI.ReasoningEffort
picker    modelPicker
```
   `modelLabel` 改为 `fmt.Sprintf("%s/%s", m.route.Provider, m.route.Model)`。
3. `tui_commands.go` `buildSlashRegistry`：`cfg.Features.ModelSwitch` 时注册 `/model` handler
   （替代 compat 版）；`tui_model.go` Update 在 `picker.active` 时拦截 ↑/↓/enter/esc。
4. `cli/tui_model.go` `submitTurn` runOpts 追加（见 §2.4 代码）：
```go
agent.WithModelRequester(m.requester),
agent.WithModelOptions(m.effortOptions()),
```
5. 切换 handler `tuiHandleModelSet(provider, model)`：
   校验 provider 在 `providers.list`（或 `DEFAULT_SETTINGS`）→ `buildModelRequester(route.routeConfig())`
   → 更新 `m.route`/`m.requester` → `writeModelPref` → `pushModelRecent` → `commitLine(successText("✓ 已切换到 provider/model"))`。
6. `tui_compat.go`：删除旧 `tuiHandleModel` 报告版，`/model` 兼容别名（models/scoped-models）仍指向新 handler。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go build ./... && go vet ./...`；手动烟测 §9.6/§9.7。

### Task 5 — `/effort` 命令 + Options 注入
Files: `cli/tui_effort.go`（新）、`cli/tui_commands.go`、`cli/tui_compat.go`、`cli/tui_model.go`

Change Necessity: 思考等级控制需要命令 + 注入点（Task 3 已备）；纯 handler + effortOptions。

Steps:
1. 新建 `cli/tui_effort.go`：
```go
// tuiHandleEffort 处理 /effort：
//   无参        → 列出可用级别 + 当前
//   status      → 报告当前（含 provider default）
//   low/medium/high/auto → 设置 + 持久化
func tuiHandleEffort(m *chatTUI, args string) (tea.Cmd, bool) { ... }
```
   `effortOptions()`（见 §2.4）挂到 `chatTUI`（Task 4 已引用）。
2. `buildSlashRegistry`：`cfg.Features.EffortControl` 时注册 `/effort`（Usage: `[low|medium|high|auto|status]`）。
3. `tui_compat.go`：`thinking`（opencode）兼容项从「未实现」改为 `Implemented:true` + 指向
   `tuiHandleEffort`（行为对齐：opencode `/thinking` = 思考开关 → 映射 `/effort`）。
4. `tui_model.go`：`m.effort` 初始化 = `readEffortPref()`；`""` 时用 `cfg.TUI.ReasoningEffort`。
5. 测试（并入 `tui_model_route_test.go`）：`effortOptions` 的 low/medium/high/auto/空 → 期望 map/nil。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Effort' ./... && go vet ./...`

### Task 6 — `/theme` 真实实现
Files: `cli/tui_theme.go`、`cli/tui_commands.go`、`cli/tui_compat.go`、`cli/tui_theme_test.go`（新）

Change Necessity: `/theme` 现为 stub；`activeTheme` 与 `tui_theme.go` 已具备切换基础设施。

Steps:
1. `cli/tui_theme.go` 增加：
```go
const themePrefFile = "theme.json"

// applyTheme 切换 activeTheme 并持久化（dark/light/auto）。
func applyTheme(name string) error {
    // name=auto 时探测（COLORFGBG 或平台默认 dark）；设 activeTheme = darkTheme/lightTheme
    // 持久化 ~/.inferglow/theme.json = {"theme": name}
}
func readThemePref() string // 损坏 → ""
```
2. `buildSlashRegistry`：`cfg.Features.ThemeSwitch` 时注册 `/theme`（`[dark|light|auto]`），
   handler `tuiHandleTheme`：无参列出 + 当前；有参切换并重绘（`applyTextareaTheme` + 状态栏）。
3. `tui_compat.go`：`theme` 兼容项（pi/opencode/codex）改为 `Implemented:true` 指向 `tuiHandleTheme`。
4. `newChatTUI` 启动：`theme := readThemePref()` 非空 → `applyTheme(theme)`。
5. 测试：`applyTheme("light")` → activeTheme 为 light 且 theme.json 写入；`readThemePref` 回环；损坏文件 → ""。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Theme' ./... && go vet ./...`

### Task 7 — 状态栏扩展
Files: `cli/tui_model.go`（View 部分）

Change Necessity: 用户需要看到「实际生效」的路由/思考等级/工作目录，而非配置残留。

Steps:
1. 定位状态栏渲染处（`m.modelLabel` 当前用途），构造三元组：
```go
statusRoute := fmt.Sprintf("%s/%s", m.route.Provider, m.route.Model) // provider 空 → 仅 model
statusEffort := m.effort
if statusEffort == "" { statusEffort = "auto" }
statusWs := filepath.Base(m.workspace.GetCurrentDir())
// 宽度 < 100 → 只显示 model · effort
```
2. `m.modelLabel` 引用处统一替换为三元组渲染（窄终端降级见 §2.7）。
3. 切换模型/effort/workspace 后置 `m.transcriptDirty = true` 触发重绘。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go build ./... && go vet ./...`；手动烟测 §9.8。

### Task 8 — 输入历史持久化 + 文档
Files: `cli/tui_input_history.go`（新）、`cli/tui_input_history_test.go`（新）、`cli/tui_model.go`、`docs/guides/tui-build-and-config.md`

Change Necessity: 输入历史现为内存；落盘让重启后 ↑ 召回（dsh-tui history.ts 对齐）。

Steps:
1. 新建 `cli/tui_input_history.go`：
```go
const (
    inputHistoryFile = "input_history.json"
    inputHistoryMax  = 200
)
// loadInputHistory() []string / appendInputHistory(line)（NDJSON；去重相邻重复；超限截断最旧）
```
2. `newChatTUI`：`m.submittedInputs = loadInputHistory()`（`cfg.Features.InputHistory` 门控）。
3. `submitTurn`：`appendInputHistory(message)`（异步 goroutine，写失败静默）。
4. 测试：追加/相邻去重/200 上限/损坏文件 → 空列表不 panic。
5. 文档：features 表补 `model_switch`/`effort`/`theme_switch`/`input_history`；
   命令表补 `/model` `/effort` `/theme` 新行为（含 `/model status`、`/effort status` 迁移说明）。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'InputHistory' ./... && go vet ./... && go build ./...`

### Task 9 — 轮次统计扩展（RF-6）
Files: `cli/tui_stats.go`（新）、`cli/tui_receipt.go`、`cli/tui_model.go`、`cli/tui_stats_test.go`（新）

Change Necessity: `turnReceipt` 已有 duration/toolCalls，缺思考时间与每工具耗时；数据点在 `ingestEvent`
已有事件，纯 TUI 层扩展，不改 agent。

Steps:
1. 新建 `cli/tui_stats.go`，扩展 `turnReceipt`（见 §2.10 结构）：
```go
type turnReceipt struct {
    turnNum          int
    duration         int
    llmRounds        int
    toolCalls        int
    promptTokens     int
    completionTokens int
    // RF-6 新增
    thinkingMs       int64
    reasoningTokens  int
    toolDurationsMs  map[string]int64
    totalOutputChars int
}
```
2. `tui_model.go` `ingestEvent` 增加计时点（`m.receipt` 已存在则复用，否则新增字段）：
```go
case agent.EventReasoning:
    // 首条 reasoning → m.thinkingStart=now；EventRunEnd → thinkingMs += now-thinkingStart
case agent.EventToolCallStart:
    m.toolStart[toolName] = now
case agent.EventToolCallEnd:
    m.receipt.toolDurationsMs[name] += now - m.toolStart[name]
    m.receipt.toolCalls++ // 若原计数在别处，保持单点
case agent.EventToken:
    m.receipt.totalOutputChars += len(e.Text) // 供 TPS 复用
```
3. `tui_receipt.go` 渲染扩展（§2.10 布局）；`reasoningTokens` 取 `usage.ReasoningTokens()`。
4. 测试：思考时间累计、工具耗时累计、多工具同名累计。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Stats|Receipt' ./... && go vet ./...`

### Task 10 — TPS 输出效率（RF-7）
Files: `cli/tui_stats.go`（续）、`cli/tui_model.go`、`cli/tui_stats_test.go`（续）

Change Necessity: 状态栏需实时 tps + 每轮样本统计；纯 TUI 层采样。

Steps:
1. `tui_stats.go` 增加 `tpsTracker`（见 §2.11）：
```go
type tpsTracker struct {
    current     float64
    samples     []tpsSample
    firstToken  time.Time
    firstSet    bool
}
// OnToken(chars int)：首 token 记时；current = (chars/4) / elapsed_s
// OnRunEnd()：push sample{current, now}，截断 500
// RenderLive(width)：▕gauge▏ N tps（1/8 格，≥50 绿 / ≥20 黄 / <20 红）
// RenderHistory(width)：sparkline（▁▃▅▇）+ N tps
// Stats()：avg60/mean/p95
```
2. `tui_model.go`：`EventToken` 调 `OnToken`（复用 Task 9 的 `totalOutputChars` 增量）；
   `EventRunEnd` 调 `OnRunEnd`；`EventRunStart` 重置。
3. 状态栏：`features.tps` 门控；流式显示 `RenderLive`，空闲 `RenderHistory`（§2.7 三元组后追加）。
4. `/tps` 命令注册（`buildSlashRegistry`）：`tps N · avg60 X · mean Y · p95 Z`。
5. 测试：`OnToken` 累积 tps 单调、`OnRunEnd` push、样本截断、`Stats` p95 计算、色阶阈值。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Tps|Stats' ./... && go vet ./...`

### Task 11 — 缓存命中率（RF-8）
Files: `cli/tui_stats.go`（续）、`cli/tui_receipt.go`、`cli/tui_model.go`、`cli/tui_stats_test.go`（续）

Change Necessity: `UsageInfo.PromptTokensDetails` 已承载 `cached_tokens`，缺展示层。

Steps:
1. `tui_stats.go` 增加（见 §2.12）：
```go
func cacheHitRate(u *model.UsageInfo) (rate float64, cacheRead, cacheWrite int, ok bool)
```
2. `tui_model.go`：`EventRunEnd` 的 usage（`StreamChunk.Usage`）传入 receipt，存 `usage *model.UsageInfo`。
3. 状态栏 `cache` 字段（`features.cache_hit` 门控）：`cache 42.3%`（无值不渲染）。
4. `/receipt` 追加行：`Cache hit 42.3% · read 12.3k · write 3.1k`（单位换算 `formatTokens` 式 k/M）。
5. 测试：`cacheRead/(input+cacheRead+cacheWrite)`；usage nil / total≤0 → ok=false；cached_tokens 缺失 → cacheRead=0。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'CacheHit|Stats' ./... && go vet ./...`

### Task 12 — 开屏欢迎页 + `/tips`（RF-9）
Files: `cli/tui_welcome.go`（新）、`cli/tui_welcome_test.go`（新）、`cli/tui_commands.go`、`cli/tui_model.go`

Change Necessity: 新用户需要首屏引导；提示池与 `/tips` 共用（dsh-tui tips.ts 对齐）。

Steps:
1. 新建 `cli/tui_welcome.go`（见 §2.13）：
```go
type welcomeTip struct { id, group, text string }
var welcomeTips = []welcomeTip{
    {"keys-history", "keys", "↑ 召回输入历史（Ctrl+R 搜索）"},
    {"cmd-model", "commands", "/model 切换模型 · /model status 查看路由"},
    {"cmd-effort", "commands", "/effort high 提高思考等级 · /effort auto 恢复默认"},
    {"cmd-workspace", "commands", "/workspace 切换目录 · /cd - 回到上一个"},
    {"cmd-tips", "commands", "/tips 随时查看提示 · /tips display 过滤分组"},
    {"wf-receipt", "workflow", "/receipt 查看本轮耗时/工具/tps/缓存命中"},
    {"disp-theme", "display", "/theme light|dark|auto 切换主题"},
    {"pitfall-restart", "pitfalls", "模型切换立即生效；编辑 config 后需重启"},
}
```
2. `tuiWelcome` 状态 + `Render(width) []string`（分页 ≤5 条/屏，窄终端截断）。
3. `newChatTUI`：`welcome_seen.json` 不存在 → `welcome.visible=true` + 写标志；
   `features.welcome` 门控；之后 `/welcome` 手动唤起。
4. Update：`welcome.visible` 时 `esc`/`q` 关闭、`tab` 翻页；不拦截其他键。
5. View：欢迎页渲染在 transcript 顶部（首帧），不占 `bottomRows`。
6. `/tips` 命令注册：无参显示全部分组当前页；`/tips <group>` 过滤；复用 `Render`。
7. 测试：tip 池分组、分页边界、首启标志读写、`/tips <group>` 过滤。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Welcome|Tips' ./... && go vet ./...`

### Task 13 — API 健康检查（RF-10）
Files: `cli/tui_health.go`（新）、`cli/tui_health_test.go`（新）、`cli/tui_commands.go`、`cli/tui_model.go`、`cli/config.go`

Change Necessity: 本地 API（ollama/vllm/lmstudio 等）是否上线用户无法感知；周期探活 + 状态栏指示。

Steps:
1. `config.go` `TUIConfig` 增加：
```go
HealthCheckInterval int    `json:"health_check_interval,omitempty"` // 秒，默认 60
HealthProbeMode     string `json:"health_probe_mode,omitempty"`     // "tcp"|"http"|"off"，默认 tcp
```
2. 新建 `cli/tui_health.go`（见 §2.14）：
```go
type healthChecker struct {
    active   bool
    interval time.Duration
    entries  map[string]*providerHealth
    checking bool
}

// probeOne(r ModelRoute) *providerHealth：
//   本地 host（localhost/127.0.0.1/::1/10.x/192.168.x/172.16-31.x）→ mode tcp: net.DialTimeout(2s)
//   mode http: GET {endpoint}/models（带 APIKey 头）2s 超时，期望 2xx
//   远程 → 仅 GenerateRequestData 构造探测（Available() 模式），不真发请求
// checkAll(providers []ModelRoute)：并发探活全部，写入 entries
```
3. `tui_model.go`：`chatTUI` 加 `health healthChecker`；`Init` 返回
   `tea.Tick(interval)` 驱动；Update 中 `state==tuiIdle && !checking` 才触发 `checkAll`；
   探活结果置 `transcriptDirty` 触发状态栏刷新。
4. 状态栏 `health` 字段（`features.health_check` 门控）：`● deepseek 42ms` 绿 / `○ openai timeout` 红；
   失败不阻断请求。
5. `/health` 命令：无参 → `checkAll` + 报告全部；`/health <key>` → 单探；
   `/health interval <sec>` → 动态调周期（10~600 钳制）+ 重发 `tea.Tick`。
6. 测试：本地/远程判定、`net.DialTimeout` 失败 → ok=false、interval 钳制、checking 防重入。
7. 探活失败时 `commitLine(warnText("○ provider 不可达: ..."))` 一次性提示（每 provider 冷却 5 分钟防刷屏）。

Verify: `cd E:\test\rewrite-agently\inferglow-github\cli && go test -run 'Health' ./... && go vet ./... && go build ./...`

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
cd E:\test\rewrite-agently\inferglow-github\orchestrator\agent
go test ./...
go vet ./...
```

**手动 TUI 烟测**（Windows Terminal）：
1. `config.json` 配 `providers: {"deepseek": {...}, "openai": {...}, "active": "deepseek"}` → 启动状态栏显示 `deepseek/<model> · effort:auto · 📁 <dir>`。
2. `/model` → 弹出 provider 列表；选 `openai` → 模型列表；选模型 → `✓ 已切换到 openai/gpt-4`；
   状态栏更新；下一轮对话用新模型。
3. 重启 inferglow → 路由保持 `openai/gpt-4`（`model.json` 生效）。
4. `/model status` → 报告当前生效路由；`/model recents` → 最近 5 条。
5. `/effort high` → 状态栏 `effort:high`；对话请求体含 `reasoning_effort:high`（provider 侧验证）；
   `/effort auto` → 恢复默认。
6. `/effort` 无参 → 列出 low/medium/high/auto + 当前值。
7. `/theme light` → 界面配色切换；重启保持。
8. 输入几条消息 → 重启 → ↑ 能召回历史（`input_history.json` 生效）。
9. 配置 `model_switch=false` → `/model` 回退报告版；`effort=false` → `/effort` 未注册。
10. **轮次统计**：跑一轮多工具对话 → `/receipt` 显示 thinking 3.2s、tools 4 (bash 2.1s…)、
    reasoning tokens、cache hit 42.3%。
11. **TPS**：流式期间状态栏出现 `▕▏ 34 tps` gauge；结束后变 sparkline；`/tps` 显示 avg60/mean/p95。
12. **缓存命中**：状态栏 `cache 42.3%`；`/receipt` 行 `Cache hit 42.3% · read 12.3k · write 3.1k`。
13. **欢迎页**：删除 `welcome_seen.json` 后启动 → 首屏提示；Esc 关闭；`/tips` 重开；`/tips keys` 过滤分组。
14. **健康检查**：本地配 ollama（localhost:11434）→ 状态栏 `● ollama 42ms`；停掉 ollama →
    60s 内变 `○ ollama timeout`；`/health` 手动触发报告；`/health interval 30` 改周期。
15. 配置 `turn_stats=false`/`tps=false`/`cache_hit=false`/`welcome=false`/`health_check=false` →
    对应字段/页面消失，行为回退。

---

## 10. Risks / Rollback / Retirement

| 风险 | 处理 |
|---|---|
| Agent/Engine 运行时换 requester 破坏现有循环 | Task 3 默认 nil 零行为变化；`orchestrator/agent` 全量测试回归 |
| `providers` 配置写错导致启动失败 | `resolveModelRoute` 纯函数兜底（默认 deepseek）；`llm` 单路兼容路径不变 |
| `/model` 无参行为从报告变选择器（行为变化） | 文档标注升级；`/model status` 保留原报告能力 |
| `reasoning_effort` 注入后 provider 报错 | G1-03 已保证透传不拦截；`auto`/空 = 不注入，一键回退 |
| 持久化文件写竞争/损坏 | 全部 best-effort 静默降级；独立文件不与 config.json 竞争 |
| 状态栏三元组在窄终端溢出 | 宽度 <100 降级为 `model · effort`；tps/cache/health 字段窄终端自动隐藏 |
| 输入历史文件增长 | NDJSON 上限 200 条，超限截断最旧 |
| TPS 估算不准（字符/4 非真实 token） | 与 dsh-tui 同估算口径（outputChars/4）；provider 返回 usage 时优先用真实 completion_tokens |
| 健康检查误报（本地服务慢启动） | TCP 2s 超时 + 冷却 5 分钟；`health_probe_mode=off` 可关；失败仅提示不阻断 |
| 周期探活抢占带宽/打扰运行轮次 | `state!=tuiIdle` 跳过；interval 可调 10~600s |
| 欢迎页干扰老用户 | `welcome_seen.json` 仅首启；`/welcome` 手动唤起；`features.welcome=false` 关 |
| 缓存命中率 provider 不返回 cached_tokens | `cacheHitRate` ok=false → 字段不渲染，无副作用 |

**Retirement**：`providers` 若被后续 config 驱动替代，`resolveModelRoute` 单一函数可替换；
`/effort`/`/theme`/输入历史/统计/TPS/缓存/欢迎页/健康检查均可经 FeatureFlags 关闭；
不改变 `model` 包与 `orchestrator` 既有语义。

---

## 11. Execution Route

```text
Execution Route:
- Decision: inline（Task 1-8 与 Task 9-13 可分批：Phase A 模型/effort/theme/历史 + Phase B 统计/TPS/缓存/欢迎/健康）
- Evidence: 改动横跨 cli + orchestrator/agent 两个模块且强依赖（config→route→RunOption→TUI 接线），
  subagent 切分会破坏上下文；Task 3 需复核 Engine 构造点
- Fallback: 若 Task 3 复核发现 Engine 复用缓存，拆出「Task 3a requester」+「Task 3b options」两个独立 commit；
  Phase B（Task 9-13）全部为 TUI 层纯扩展，可独立于 Phase A 合入
- User confirmation required: no
```

> 已按 §8 Task 1–13 全部实现；与计划的实现偏差见实现说明（resolveModelRoute 采用
> **持久化偏好优先**（plan §9 烟测 #3「重启保持 model.json 生效」），以及
> `OnLLMCallEnd` 回调扩展 `usage *model.UsageInfo` 以支撑 RF-6/8 真实数据）。
>
> **RF-2 增强（§2.5/Task 5 之上）**：`/effort` 从固定 `low/medium/high` 升级为
> **按模型感知的 effort 尺度**（`cli/tui_effort_scale.go`）。每个 provider/模型有
> 自己的档位集合与参数映射——DeepSeek 为 `off/low/high/max`（无 `medium`），
> OpenAI 及默认走 `off/low/medium/high`；`tui.effort_scales` 允许按
> `provider/model` 或 `provider` 覆盖（匹配优先级 `provider/model` > `provider` >
> 内置默认）。档位名持久化到 `effort.json`；切换模型时档位不在新尺度内自动恢复
> 默认并提示，切回原模型时自动恢复（运行时对无效档位降级为不注入）。
>
> **LLM-provider-port（2026-08-25，计划外追加）**：移植 pi-ai/DSH 的公共 LLM
> provider + effort 模式（规划见 `task_plan_llm_port.md`）：
> - `model/effort.go`：effort wire 翻译层（11 种 format：openai/deepseek/openrouter/
>   together/zai/qwen/string/ant-ling/google/anthropic/mistral/bedrock），
>   `TranslateEffort` + `EffortOffWire`；
> - `model/provider_profile.go` + `provider_profiles_gen.go`：ProviderProfile 注册表
>   （8 个手写核心 + 23 个生成，数据来自 pi-ai 0.82.1 `data/*.json`）；
> - `model/openai.go`/`openai_responses.go`/`anthropic.go`：`EffortFormat`/
>   `EffortLevels` 字段 + `effortWireParams`，`RequestModel` 处把
>   `Options["reasoning_effort"]` 翻译成 wire 参数（无 level map 时保持旧裸 key 透传）；
> - `cli/tui_effort.go`：`effortScaleForRoute()` 按 model profile 收紧 `/effort`
>   展示档位（模型不提供的档位不显示）；
> - 新增 12 个 provider（google/mistral/groq/xai/together/zai/moonshotai/nvidia/
>   cerebras/huggingface/fireworks/qwen-token-plan-cn）进 `DEFAULT_SETTINGS` + 工厂 +
>   CLI 路由；**Google 原生协议**（`model/google.go`）实现 streamGenerateContent +
>   thinkingConfig + thought part 分离（`EffortGoogle` wire 生效）。

---

## 12. Related Specs

- **斜杠命令兼容计划**：`docs/plans/2026-08-24-slash-command-compat-autocomplete.md`（SC-1~SC-6 已实现；
  本计划的 `/model`/`/theme` 升级其 compat stub，`/effort` 接管 `thinking` 兼容项）。
- **任务面板规格**：`docs/requirements/tui-task-panels-spec.md`（任务面板/消息操作/工作空间已实现，
  本计划的状态栏三元组与其互不冲突）。
- **dsh-tui 参照**：`ccch1mneyyy/dsh-TUI`（`src/modelPrefs.ts`、`src/modelRoute.ts`、
  `src/effortPrefs.ts`、`src/themePrefs.ts`、`src/history.ts`、`src/workspaces.ts`、
  `src/screens/StatusMetrics.ts`、`src/screens/StatusLine.tsx`、`src/components/messages/*`、
  `src/tips.ts`）。
- **OpenCode 参照**：`anomalyco/opencode`（`packages/tui/src/component/dialog-model.tsx`、
  `dialog-provider.tsx`、`dialog-theme-list.tsx` — 选择器 UI 形态参考）。
