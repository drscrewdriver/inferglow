# InferGlow 初始化与配置指南

> 面向启动阶段的 **初始化文档**：`llm.endpoint` 等「启动必须配置」从配置文件读取的机制、
> 如何初始化配置（交互向导 / 直接写文件 / 环境变量）、配置优先级与故障排查。
> 参考：CLI 使用指南 [cli-usage.md](cli-usage.md) · TUI 构建与配置 [tui-build-and-config.md](tui-build-and-config.md)。

---

## 1. 为什么启动会报 `llm.endpoint is required`

启动时（TUI / REPL / OneShot）`BuildRuntime → buildAgent → buildModelRequester`（`cli/model_factory.go`）
会校验：

```go
if cfg.LLM.Endpoint == "" {
    return nil, fmt.Errorf("llm.endpoint is required")
}
```

也就是 **只要配置文件里 `llm.endpoint` 为空，任何输出模式都无法启动**。这不是"没读配置"，
而是配置虽然被读了，但里面根本没有填过 LLM 信息——`LoadOrDefaultConfig` 在找不到配置时只
落盘一份**默认空配置**（`endpoint:""`、`model:""`），除非你跑过 `init` 向导，否则这个字段永远为空。

所以**「启动必须配置」确实是从配置文件读取的**，前提是配置文件先被初始化填上值。

### 必须配置项

| 项 | 字段 | 不填后果 |
|---|---|---|
| API 地址 | `llm.endpoint` | 启动即报 `llm.endpoint is required`（硬性） |
| 模型名 | `llm.model` | 请求缺少 model，大概率 4xx（建议必填） |
| 提供方 | `llm.provider` | 默认回退 `openai`（OpenAI 兼容，覆盖本地服务） |
| API Key | `llm.api_key` | 可空；本地 OpenAI 兼容服务常无需鉴权 |

---

## 2. 配置作为唯一事实源（Source of Truth）

配置加载链路（`cli/config.go`）：

1. `LoadOrDefaultConfig(path)`：显式 `-config <path>` → 默认 `~/.inferglow/config.json` → 都没有则落盘默认值。
2. `ApplyEnvOverrides(&cfg)`：用环境变量覆盖 `llm.*`。
3. 命令行 flag 覆盖（`-model` / `-workspace` / `-unsafe`）。

**优先级（从低到高）**：配置文件 `<` 环境变量 `<` 命令行 flag。

> 结论：日常只需初始化**配置文件一次**，`llm.*` 就从配置读出，启动不再报错；
> 想临时换端点/模型就用环境变量或 flag 覆盖，无需改文件。

---

## 3. 三种初始化方式

### 方式 A：交互式向导（推荐首次）

```powershell
.\inferglow-cli.exe init
```

按提示依次输入 endpoint / model / api_key / provider 与可选审计项，最后写入
`~/.inferglow/config.json`。（实现见 `cli/init_wizard.go`。）

### 方式 B：直接写配置文件（已初始化示例）

编辑 `%USERPROFILE%\.inferglow\config.json`。本机已按以下内容完成初始化
（本地 OpenAI 兼容服务，备份见 `config.json.bak`）：

```json
{
  "llm": {
    "endpoint": "http://192.168.100.242:8200/v1",
    "model": "Qwen3.6-35B-A3B",
    "api_key": "sp-dummy",
    "provider": "openai"
  },
  "data_dir": "C:\\Users\\joshua\\.inferglow",
  "workspace_dir": ".",
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
    "auto_background": false,
    "output_mode": "tui"
  },
  "audit": { "enabled": false },
  "tui": { "theme": "dark", "show_reasoning": false, "max_scrollback": 0 }
}
```

> 说明：`provider:"openai"` 是通用 OpenAI 兼容入口，覆盖本地 vLLM/Ollama 等；
> `api_key` 对无鉴权的本地服务可留空，若服务要求鉴权再填入或改走环境变量。

### 方式 C：环境变量（不落盘，适合 CI / 临时切换）

| 环境变量 | 作用 |
|---|---|
| `LLM_ENDPOINT` | 覆盖 `llm.endpoint` |
| `LLM_MODEL` | 覆盖 `llm.model` |
| `LLM_API_KEY` | 覆盖 `llm.api_key` |
| `LLM_PROVIDER` | 覆盖 `llm.provider` |
| `COMPRESS_MODEL` | 覆盖专用压缩模型 `compress_model.model` |

```powershell
$env:LLM_ENDPOINT = "http://192.168.100.242:8200/v1"
$env:LLM_MODEL    = "Qwen3.6-35B-A3B"
.\inferglow-cli.exe
```

---

## 4. 数据目录与首次运行

`~/.inferglow/` 布局（`cli/config.go` 的 `EnsureDataDirs` 自动创建）：

```
~/.inferglow/
├── config.json               # 初始化后的配置
├── constitutional/rules.md   # 宪法规则（Zone 0.5）
├── sessions/                 # 会话 JSONL + index.jsonl
├── audit/                    # 审计链路
├── memory/                   # 长期记忆
└── skills/global/ · projects/default/skills/
```

启动前自动 `EnsureDataDirs` 保证目录存在；缺 LLM 配置时当前行为是**进入 TUI 后在构建
agent 时报错**（见第 1 节），更友好的「缺失即引导 init」已在 `docs/plans/init-command-plan.md`
中规划。

---

## 5. 故障排查

| 报错 | 原因 / 处理 |
|---|---|
| `init agent: build model: llm.endpoint is required` | `llm.endpoint` 为空；跑 `inferglow init` 或按第 3 节补 endpoint |
| 请求 `model` 缺失 / 404 | `llm.model` 未填或与本地服务注册名不一致 |
| 401 / 鉴权失败 | 本地服务需要 key：填 `llm.api_key` 或设 `LLM_API_KEY` |
| 连不上端点 | 确认 `endpoint` 可达（`Invoke-WebRequest <endpoint>/models` 探测） |

---

## 6. 相关文档

- [CLI 使用指南](cli-usage.md) — 启动模式 / Slash 命令 / 审计
- [TUI 构建与配置](tui-build-and-config.md) — Windows 构建、TUI 配置字段
- [初始化命令规划](../plans/init-command-plan.md) — init 命令改进设计（规划稿）
