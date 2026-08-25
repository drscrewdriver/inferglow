# InferGlow TUI — Windows 构建与配置指南

> 面向 Windows（无 `make`/`find`）环境下构建 InferGlow CLI 的 **TUI**（Bubble Tea 全屏文本界面）可执行文件，并给出完整的配置示例与说明。
>
> 涉及模块：`cli`（TUI / REPL / OneShot 应用层入口）。官方多模块构建总览见 [build-and-test.md](build-and-test.md)。

---

## 1. TUI 是什么

`cli` 模块提供三种交互模式（`cli/cmd/inferglow-cli/main.go` 中的分发逻辑，优先级 **OneShot > TUI > REPL**）：

| 模式 | 触发方式 | 说明 |
|---|---|---|
| **TUI（默认）** | 直接运行 `inferglow-cli` | 基于 `charm.land/bubbletea/v2` + `lipgloss` 的全屏 alt-screen 文本界面，多面板、斜杠命令、颜色主题 |
| REPL | `--cli` | 单行输入输出循环 |
| OneShot | `-z "<prompt>"` / `--oneshot` | 单次提问 → 直接打印最终回答到 stdout → 退出 |

> Windows 注意：TUI 依赖 Bubble Tea 的终端能力。请在 **Windows Terminal / Windows 11 终端 / VS Code 集成终端** 等支持 ANSI 的终端中运行；旧版 `cmd.exe`（conhost 的 legacy 模式）可能显示异常。

---

## 2. 环境准备

| 依赖 | 版本要求 | 说明 |
|---|---|---|
| Go toolchain | **go 1.25.0+** | `cli/go.mod` 声明 `go 1.25.0`；本地若只有旧版（如 go1.24），需 `GOTOOLCHAIN=auto` 自动下载 1.25 工具链 |
| 网络 | — | 首次构建需拉取 Bubble Tea 等第三方依赖与 go1.25 工具链 |

检查版本：

```powershell
go version          # 需要 >= 1.25.0（或开启 GOTOOLCHAIN=auto）
```

> 若本机 Go 为旧版，`go build` 会因 `go.mod: requires go >= 1.25.0` 报错。设置
> `GOTOOLCHAIN=auto` 让 Go 自动下载并切换工具链即可（默认即 `auto`）。

---

## 3. Windows 构建（已验证）

构建产物：`cli/bin/inferglow-cli.exe`（约 13 MB）。

### 3.1 基础构建

```powershell
cd E:\test\rewrite-agently\inferglow-github\cli
go build -o bin\inferglow-cli.exe .\cmd\inferglow-cli
```

`Makefile` 的 `make build` / `build-sandbox` 使用了 `find -execdir`（非 Windows 命令），
Windows 下直接对 `cli` 模块单独构建即可。若需沙箱隔离 tag：

```powershell
go build -tags with_sandbox -o bin\inferglow-cli.exe .\cmd\inferglow-cli
```

### 3.2 沙箱内构建（把 Go 缓存重定向到仓库内）

在 DSH 文件沙箱 / 受限写权限环境下，`go` 会去写用户 `AppData` 的构建缓存而失败
（报 `failed to trim cache: ... Access is denied`）。把 Go 各缓存目录重定向到仓库内的
`.gobuild/` 即可在只允许写工作区的模式下完成构建：

```powershell
cd E:\test\rewrite-agently\inferglow-github\cli
$env:GOCACHE   = "E:\test\rewrite-agently\inferglow-github\.gobuild\cache"
$env:GOMODCACHE= "E:\test\rewrite-agently\inferglow-github\.gobuild\mod"
$env:GOPATH    = "E:\test\rewrite-agently\inferglow-github\.gobuild\gopath"
$env:GOTOOLCHAIN = "auto"
go build -o bin\inferglow-cli.exe .\cmd\inferglow-cli
```

> `.gobuild/` 仅为构建缓存，可随时删除，不影响源码。

### 3.3 验证构建产物

```powershell
cd E:\test\rewrite-agently\inferglow-github\cli\bin
.\inferglow-cli.exe -h      # 打印全部 flag（TUI / REPL / OneShot 等）
```

预期输出（部分）：

```
  -cli                Use single-output REPL mode instead of TUI
  -config string      Path to config file
  -model string       Model name to use
  -tui                Enable full-screen TUI mode (default) (default true)
  -unsafe             Allow bash execution without confirmation
  -workspace string   Working directory for the agent (default ".")
  -z string           One-shot mode: send prompt, print final response to stdout, exit
```

> TUI 本身是全屏交互程序，无法在无 TTY 的 headless 环境里做端到端烟测；
> `-h` 能正常打印 flag 即证明二进制可运行。

### 3.4 首次运行（交互式初始化向导）

```powershell
.\inferglow-cli.exe init
```

引导式输入 LLM endpoint / model / api_key / provider，以及可选的审计开关，最后把配置
写到 `~/.inferglow/config.json`。

---

## 4. 配置说明

配置默认路径：`%USERPROFILE%\.inferglow\config.json`（JSON 格式）。
可通过 `-config <path>` 指定其它文件。无配置文件时，程序会写入一份默认配置。

### 4.1 命令行 Flag（优先级最高，覆盖 config）

| Flag | 说明 |
|---|---|
| `-workspace <dir>` | 工作目录（默认 `.`） |
| `-model <name>` | 覆盖 `llm.model` |
| `-config <path>` | 指定配置文件路径 |
| `-resume <id>` | 恢复指定会话 |
| `-unsafe` | 允许 bash 执行无需确认 |
| `-tui` / `-cli` | 全屏 TUI（默认）/ 单行 REPL |
| `-z <prompt>` / `--oneshot` | OneShot 单次问答模式 |

### 4.2 完整配置示例

完整字段示例见仓库文件：`cli/examples/config.example.json`（含 15 个 provider 的多路配置）

```json
{
  "llm": {
    "endpoint": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "",
    "provider": "openai"
  },
  "providers": {
    "active": "deepseek",
    "list": {
      "openai": {
        "endpoint": "https://api.openai.com/v1",
        "model": "gpt-5.4",
        "provider": "openai"
      },
      "deepseek": {
        "endpoint": "https://api.deepseek.com/v1",
        "model": "deepseek-v4-pro",
        "provider": "deepseek"
      },
      "google": {
        "endpoint": "https://generativelanguage.googleapis.com/v1beta",
        "model": "gemini-3.1-pro-preview",
        "provider": "google"
      },
      "mistral": {
        "endpoint": "https://api.mistral.ai/v1",
        "model": "mistral-medium-latest",
        "provider": "mistral"
      },
      "openrouter": {
        "endpoint": "https://openrouter.ai/api/v1",
        "model": "anthropic/claude-opus-4-7",
        "provider": "openrouter"
      }
    }
  },
  "data_dir": "C:\\Users\\<you>\\.inferglow",
  "workspace_dir": ".",
  "constitutional": "C:\\Users\\<you>\\.inferglow\\constitutional\\rules.md",
  "window_tokens": 32000,
  "top_k": 5,
  "unsafe_mode": false,
  "sandbox_mode": "trusted_local",
  "context_mode": "hybrid",
  "features": {
    "memory_injection": true,
    "memory_storage": true,
    "constitutional": true,
    "meta_instructions": true,
    "compression": true,
    "proactive_recall": false,
    "runtime_mode_switch": true,
    "tui_mode": true,
    "output_mode": "tui",
    "slash_compat": true,
    "slash_popup": true,
    "task_panel": true,
    "message_actions": true,
    "workspace_switch": true,
    "skill_loader": true,
    "model_switch": true,
    "effort": true,
    "theme_switch": true,
    "input_history": true,
    "turn_stats": true,
    "tps": true,
    "cache_hit": true,
    "welcome": true,
    "health_check": true
  },
  "audit": {
    "enabled": false,
    "storage_path": "",
    "signature_key": ""
  },
  "tui": {
    "theme": "dark",
    "show_reasoning": false,
    "max_scrollback": 0,
    "reasoning_effort": "",
    "health_check_interval": 60,
    "health_probe_mode": "tcp",
    "effort_scales": {
      "deepseek": {
        "off":   { "label": "关闭思考" },
        "low":   { "params": { "reasoning_effort": "low" } },
        "high":  { "params": { "reasoning_effort": "high" } },
        "max":   { "params": { "reasoning_effort": "max" } }
      },
      "mybox/thinking-model": {
        "eco":   { "params": { "thinking": { "type": "enabled", "budget_tokens": 1024 } } },
        "pro":   { "params": { "thinking": { "type": "enabled", "budget_tokens": 4096 } } }
      }
    }
  },
  "compress_model": {
    "endpoint": "",
    "model": "",
    "provider": ""
  }
}
```

### 4.3 配置字段速查

#### 顶层字段

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `llm` | object | — | LLM 端点配置（见下） |
| `data_dir` | string | `~/.inferglow` | 数据目录（sessions/memory/skills/audit） |
| `workspace_dir` | string | `.` | Agent 工作目录 |
| `constitutional` | string | `<data_dir>/constitutional/rules.md` | 宪法规则文件（Zone 0.5） |
| `window_tokens` | int | `32000` | 上下文窗口 tokens |
| `top_k` | int | `5` | 召回 top-k |
| `unsafe_mode` | bool | `false` | 允许 bash 免确认执行 |
| `sandbox_mode` | string | `trusted_local` | 见下方取值 |
| `context_mode` | string | `hybrid` | `passthrough` / `three_zone` / `summary` / `hybrid` |
| `compress_model` | object | 空 | 专用压缩模型，缺省回退主 LLM |
| `audit` | object | 关闭 | 审计链配置 |
| `features` | object | — | 功能开关 |
| `tui` | object | — | TUI 显示配置 |

#### `llm`

| 字段 | 说明 |
|---|---|
| `endpoint` | API 地址（如 `https://api.openai.com/v1`） |
| `model` | 模型名 |
| `api_key` | API Key（**建议用环境变量**，勿写入配置文件） |
| `provider` | `openai` / `deepseek` / `anthropic` … |

#### `providers`（RF-1：多路 Provider 配置，可选）

| 字段 | 说明 |
|---|---|
| `active` | 当前激活的 provider 键（`list` 中的某个键）。省略 = 回退 `~/.inferglow/model.json`（运行时 `/model` 选择）或单路 `llm` |
| `list` | `map<provider键, LLMConfig>`：每个键是一路完整配置（endpoint/model/api_key/provider） |

> **多路配置语义**
> - `providers.list` 为空且 `llm` 有值 → 单路模式（原行为）。
> - `providers.list` 非空时，`/model` 选择器以 `list` 键为候选；运行时 `/model` 切换写入
>   `~/.inferglow/model.json`，重启后生效（优先级：`model.json` > `providers.active` > 单路 `llm`）。
> - `list` 中每个键的 `provider` 字段决定协议路由：`google` 走原生 streamGenerateContent；
>   `anthropic` 走 Anthropic Messages；其余（openai/deepseek/mistral/groq/xai/zai/openrouter/...）
>   走 OpenAI 兼容协议（各 provider 的 effort wire 格式自动按 profile 翻译，见下方 effort 说明）。
> - **api_key 建议留空并用环境变量**（如 `DEEPSEEK_API_KEY`），勿写死在配置文件。

**支持的 `provider` 值**（`list` 键与 `llm.provider` 同源）：

| 值 | 协议 | 默认模型 | 说明 |
|---|---|---|---|
| `openai` | OpenAI 兼容 | gpt-4 | |
| `openai_responses` | OpenAI Responses | gpt-4o | o 系列推荐 |
| `anthropic` | Anthropic Messages | claude-3-5-sonnet | |
| `deepseek` | OpenAI 兼容 + thinking | deepseek-chat | effort 档位 off/low/high/max |
| `qwen` | OpenAI 兼容 | qwen-max | 阿里 DashScope |
| `glm` | OpenAI 兼容 | glm-4 | 智谱 bigmodel |
| `kimi` | OpenAI 兼容 | moonshot-v1-8k | 月之暗面国内版 |
| `google` | **原生 streamGenerateContent** | gemini-3.1-pro-preview | Gemini 3 / Gemma 4；effort wire 大写 LOW/HIGH |
| `mistral` | OpenAI 兼容 | mistral-medium-latest | |
| `groq` | OpenAI 兼容 | openai/gpt-oss-120b | |
| `xai` | OpenAI 兼容 | grok-4.5 | |
| `zai` | OpenAI 兼容 + thinking | glm-5.2 | Z.AI 新版 GLM；档位折叠 low/medium/high→high |
| `moonshotai` | OpenAI 兼容 | kimi-k3 | 国际版 |
| `together` | OpenAI 兼容 | openai/gpt-oss-120b | |
| `nvidia` | OpenAI 兼容 | nemotron-3-super-120b | |
| `cerebras` | OpenAI 兼容 | gpt-oss-120b | |
| `huggingface` | OpenAI 兼容 | openai/gpt-oss-120b | Router 聚合 |
| `fireworks` | OpenAI 兼容 | gpt-oss-120b | |
| `qwen-token-plan-cn` | OpenAI 兼容 | qwen3.7-plus | 通义 Token 套餐 |
| `openrouter` | OpenAI 兼容 + reasoning | openai/gpt-4o | 聚合；effort wire `reasoning:{effort}` |
| `ollama` | OpenAI 兼容 | llama3 | 本地 |
| `stepfun`/`baidu`/`spark`/`sensenova`/`mimo`/`tencent`/`volcengine`/`zeroone`/`minimax`/`siliconflow` | OpenAI 兼容 | 各默认 | 国内厂商 |

> 完整 provider 目录与 effort thinkingLevelMap 参考：`docs/guides/effort-and-providers-pi-ai-reference.md`。

**effort 的 wire 自动翻译**：`/effort` 选定的语义档位（如 `high`）由 model 层按 provider 的
wire 格式翻译成实际请求参数，无需在配置里手动拼：

| provider | wire 参数 |
|---|---|
| openai / 大多数 | `reasoning_effort: "high"` |
| deepseek | `thinking:{type:"enabled"}` + `reasoning_effort:"high"` |
| openrouter | `reasoning:{effort:"high"}` |
| zai / glm | `thinking:{type:"enabled"}` + `reasoning_effort` |
| qwen | `enable_thinking: true` |
| google | `generationConfig.thinkingConfig.thinkingLevel: "HIGH"` |
| anthropic | `thinking:{type:"enabled", effort:"..."}` |

#### `tui`（TUI 专属）

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `theme` | string | `dark` | `dark` / `light` / `auto`；`/theme` 命令实时切换并持久化到 `~/.inferglow/theme.json` |
| `show_reasoning` | bool | `false` | 是否显示推理过程（也可在 TUI 内用 `/verbose` 切换） |
| `max_scrollback` | int | `0` | 转录最大行数，`0`=不限（当前为预留字段，尚未接入运行时） |
| `reasoning_effort` | string | `""` | 思考等级：`off` / `low` / `medium` / `high` / `max` / `""`（空=provider 默认）。**档位随模型变化**（LLM-provider-port：按 pi-ai thinkingLevelMap 收紧）：DeepSeek 为 `off/low/high/max`，OpenAI o 系为 `low/medium/high`，Anthropic opus-4-7+ 为 `off/xhigh/max`，Gemini 为 `low/high`（wire 值大写 `LOW/HIGH`）等；`/effort` 命令切换后持久化到 `~/.inferglow/effort.json`，优先级高于本字段。切换模型时若档位不在新模型尺度内自动恢复默认 |
| `effort_scales` | map | 内置 | 自定义某 provider（`"deepseek"`）或某模型（`"deepseek/deepseek-chat"`）的思考档位表，优先级高于内置。键为档位名，值为 `{"label": "说明", "params": {注入参数}}`；`params` 为空=不注入。匹配优先级：`provider/model` > `provider` > 内置默认。**wire 翻译**：`/effort` 注入的 `reasoning_effort` 会被 model 层按 provider 的 wire 格式翻译（deepseek→`thinking:{type}`+`reasoning_effort`、openrouter→`reasoning:{effort}`、qwen→`enable_thinking`、gemini→`thinkingConfig` 等），见 `docs/guides/effort-and-providers-pi-ai-reference.md` |
| `health_check_interval` | int | `60` | API 健康检查周期（秒，10~600 自动钳制） |
| `health_probe_mode` | string | `tcp` | 探活方式：`tcp`（本地 TCP 拨号）/ `http`（`GET {endpoint}/models`）/ `off`（关闭探活） |

#### `sandbox_mode` 合法值

`trusted_local` · `local` · `docker` · `gvisor` · `auto`
（与 TUI `/sandbox <mode>` 命令取值一致）

#### `features` 开关

| 字段 | 默认 | 说明 |
|---|---|---|
| `memory_injection` | true | 每轮自动召回 |
| `memory_storage` | true | 工具结果自动入库 |
| `constitutional` | true | 加载宪法区 |
| `meta_instructions` | true | CM-3 注入工具/后台/压缩指令 |
| `compression` | true | 自动压缩 |
| `proactive_recall` | false | 会话启动时自动召回 |
| `runtime_mode_switch` | true | 启用 TUI `/mode` 命令 |
| `tui_mode` | true | 启用全屏 TUI |
| `auto_background` | true | CM-2：Zone 1（head buffer）为空时自动 `/rebackground` 分析项目（可能反复跑 `list_dir`/`bash_executor`）；设 `false` 关闭该自动行为，仅保留手动 `/rebackground` |
| `output_mode` | string | 默认模式：`tui` / `cli` / `oneshot` |
| `slash_compat` | true | SC-1：接受 claude/pi/opencode/codex 的斜杠命令（别名映射 + 已识别未实现提示）；关闭后仅保留原生命令 |
| `slash_popup` | true | SC-2：输入框输入 `/` 时的输入法式前缀联想弹窗（↑/↓ 选择、Tab 补全、Enter 触发、Esc 关闭）；关闭后回退为 Tab 补全 |
| `task_panel` | true | SC-3：右侧任务列表面板（`/tasks` 切换显隐，数据来自 task tracker）；**仅有真实 todo 内容时才渲染**，无任务时不显示、不占宽度 |
| `message_actions` | true | SC-4：历史消息操作菜单（`m`/`a` 进入选择模式，`o` 打开菜单：Copy 已实现，Revert/Fork 为占位提示） |
| `workspace_switch` | true | SC-5：工作空间目录切换（`/workspace`、`/cd`，状态栏显示当前目录） |
| `skill_loader` | true | SC-6：启动时扫描 `~/.agents/skills/`（Windows 为 `C:\Users\<user>\.agents\skills\`），把每个含 `SKILL.md` 的 skill 注册为斜杠命令（`/<skill>` 召唤展示内容，并出现在 `/` 弹窗联想中）；关闭后不加载 |
| `model_switch` | true | RF-1：运行时多 Provider/多 Model 切换（`/model` 交互式选择器 + 持久化 `~/.inferglow/model.json`）；关闭后 `/model` 回退为「仅报告」版本 |
| `effort` | true | RF-2：`/effort` 思考等级控制（按当前模型尺度：DeepSeek `off`/`low`/`high`/`max`，OpenAI `off`/`low`/`medium`/`high`，可经 `tui.effort_scales` 自定义；`auto`=不注入、走 provider 默认；持久化 `~/.inferglow/effort.json`） |
| `theme_switch` | true | RF-3：`/theme` 真实主题切换（dark/light/auto，持久化 `~/.inferglow/theme.json`） |
| `input_history` | true | RF-5：输入历史落盘 `~/.inferglow/input_history.json`（NDJSON，上限 200 条，重启后 ↑ 召回） |
| `turn_stats` | true | RF-6：轮次统计（思考时间 / 每工具耗时 / reasoning tokens，`/receipt` 展示） |
| `tps` | true | RF-7：TPS 输出效率（状态栏实时 gauge + 历史 sparkline + `/tps` 的 avg60/mean/p95） |
| `cache_hit` | true | RF-8：缓存命中率（状态栏 + `/receipt`，来自 `cached_tokens`） |
| `welcome` | true | RF-9：首启欢迎页 + `/tips`（提示池分组 keys/commands/workflow/display/pitfalls） |
| `health_check` | true | RF-10：API 健康检查（`/health` + 状态栏 `●`/`○`，周期探活） |

### 4.4 环境变量覆盖

程序启动时用环境变量覆盖 `llm` 与压缩模型配置（`ApplyEnvOverrides`，优先级高于
config 文件，低于命令行 flag）：

| 环境变量 | 覆盖字段 |
|---|---|
| `LLM_ENDPOINT` | `llm.endpoint` |
| `LLM_MODEL` | `llm.model` |
| `LLM_API_KEY` | `llm.api_key` |
| `LLM_PROVIDER` | `llm.provider` |
| `COMPRESS_MODEL` | `compress_model.model` |

PowerShell 设置示例：

```powershell
$env:LLM_ENDPOINT = "https://api.deepseek.com/v1"
$env:LLM_MODEL    = "deepseek-chat"
$env:LLM_PROVIDER = "deepseek"
# 优先把 key 放环境变量，避免写进配置文件
$env:LLM_API_KEY  = "sk-..."
.\inferglow-cli.exe
```

---

## 5. TUI 使用要点

启动即进入全屏 TUI（默认 `-tui`）。TUI 内支持的斜杠命令（`/help` 可查看完整列表）：

| 命令 | 作用 |
|---|---|
| `/help` | 显示帮助 |
| `/mode <mode>` | 查看 / 切换上下文模式（`hybrid` / `passthrough` / `three_zone` / `summary`） |
| `/sandbox [mode]` | 查看 / 切换沙箱模式 |
| `/memory search <q>` · `/memory stats` | 检索 / 查看记忆统计 |
| `/compact` · `/async-compress` | 手动 / 强制异步压缩 |
| `/clear` | 清空转录 |
| `/verbose` | 切换推理显示 |
| `/receipt` · `/session` | 本回合回执 / 当前会话 ID |
| `/resume [id]` | 列出 / 恢复历史会话 |
| `/vision <img> [q]` | 图片视觉问答 |
| `/config` | 显示配置路径与设置 |
| `/tasks` | 切换右侧任务面板显隐 |
| `/workspace` · `/cd` | 查看 / 切换工作目录（`list` / `<path>` / `~` / `-`） |
| `/model` | 交互式模型选择器（两级：provider → model）；`/model <provider> <model>` 直接切换；`/model status` 查看生效路由；`/model recents` 最近使用。切换立即生效并持久化 |
| `/effort` | 思考等级：`/effort` 列出当前模型可用档位 / `/effort <档位>` 设置（按模型尺度，如 DeepSeek `off\|low\|high\|max`）/ `/effort status` 查看当前档位与参数注入 |
| `/theme` | 主题：`/theme` 列主题 / `/theme dark\|light\|auto` 切换并持久化 |
| `/tips` | 显示使用提示（`/tips <keys\|commands\|workflow\|display\|pitfalls>` 过滤分组） |
| `/welcome` | 重新显示开屏欢迎页 |
| `/tps` | 显示 TPS 输出效率统计（avg60 / mean / p95） |
| `/health` | API 健康检查：无参探活全部并报告；`/health <key>` 探单个；`/health interval <sec>` 调整周期（10~600） |
| `/receipt` | 本回合回执（含思考时间、每工具耗时、reasoning tokens、缓存命中率） |
| `/graphify`、`/lark-doc` … | 本机 `~/.agents/skills/` 下的 skill（见下方 SC-6 说明） |
| `/quit` / `/exit` | 退出 |

> **命令兼容（SC-1）**：`features.slash_compat=true`（默认）时，还接受 claude/pi/opencode/codex
> 的常用命令，例如 `/reset` `/new`（→ `/clear`）、`/summarize`（→ `/compact`）、
> `/continue` `/sessions`（→ `/resume`）、`/settings`（→ `/config`）、`/status` `/usage` `/cost`
> （→ `/receipt`）、`/q` `/logout`（→ `/quit`）、`/models` `/scoped-models`（→ `/model`）等。
> 无法映射的命令（如 `/vim`、`/pets`、`/init`、`/mcp`）注册为「已识别但未实现」，
> 输入后会得到友好提示而不是 Unknown command。

> **Skill 兼容（SC-6）**：`features.skill_loader=true`（默认）时，启动会扫描
> `~/.agents/skills/`（Windows：`C:\Users\<user>\.agents\skills\`）。每个顶层目录中的
> `SKILL.md`（YAML frontmatter 的 `name`/`description` + markdown 正文）注册为一个斜杠命令，
> 例如 `~/.agents/skills/graphify/SKILL.md` → `/graphify`，弹窗中以 ◇（占位符）标记。
> **占位符语义**：从弹窗选中 skill 只把 `/graphify ` 上屏进输入框，**不会加载任何内容**；
> 再次按 Enter 确认提交时才真正激活——从磁盘读取并展示该 skill 的内容（正文上限 100 行，
> 超出提示完整文件路径；每次激活重新读盘，内容保持新鲜）。直接手输 `/graphify` 回车同理。
> **Tab = 对齐 + 循环补全**：多候选时先对齐到最大公共前缀（`/mod` + Tab → `/mode `），
> 继续按 Tab 在候选中循环上屏完整命令名（`/mode ` → `/model ` → … 回绕），弹窗保持打开，
> Enter 上屏当前高亮 / Esc 关闭。
> 命名规则：小写 + 连字符化（`AI-Research-SKILLs` → `/ai-research-skills`）；frontmatter 无
> `name` 时回退目录名。与内建命令重名时内建命令优先；与「已识别未实现」的 stub 重名时
> skill 生效（如本机有 `vim` skill，`/vim` 展示 skill 而非未实现提示）。`references/` 等
> 嵌套目录不会被视为独立 skill。关闭 `skill_loader` 即完全禁用。

---

## 6. 常见问题

| 现象 | 原因 / 处理 |
|---|---|
| `requires go >= 1.25.0` | 本机 Go 过旧；设置 `GOTOOLCHAIN=auto` 自动拉取 go1.25 工具链 |
| `failed to trim cache: Access is denied` | 沙箱限制了 `AppData` 写权限；把 `GOCACHE`/`GOMODCACHE`/`GOPATH` 重定向到仓库内 `.gobuild/`（见 §3.2） |
| TUI 界面花屏 / 无颜色 | 旧版 `cmd.exe` legacy 模式；改用 Windows Terminal / 现代终端 |
| `make` / `find` 命令不存在 | Windows 无 make；按 §3 对 `cli` 模块直接 `go build` |
| 提交到 git 前 | 建议在 `.gitignore` 忽略 `cli/bin/` 与 `.gobuild/`（纯构建产物/缓存） |
