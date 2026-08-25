# 初始化命令（`inferglow init`）规划

> **状态**：规划稿（本次仅规划 + 文档，不改代码）。
> **目标**：让「启动必须配置」真正**从配置文件可靠读出**，首次运行不再因 `llm.endpoint is required`
> 而中断；提供非交互 / 幂等 / 从 env 预填的初始化命令。
> 现状指南见 [init-and-config.md](../guides/init-and-config.md)。

---

## 1. 背景与问题

启动时 `buildModelRequester`（`cli/model_factory.go`）硬校验 `llm.endpoint == ""` 即报错，
而 `LoadOrDefaultConfig`（`cli/config.go`）在无配置时只落盘**空 LLM 配置**。导致：

1. 首次运行默认进入 TUI，却在构建 agent 阶段才报 `llm.endpoint is required`，**位置太晚**、提示不友好。
2. `inferglow init`（`cli/init_wizard.go`）只有**交互向导**一种形态，无法在脚本 / CI / 非 TTY 下初始化。
3. 向导不读取已有配置 / 环境变量作**预填默认值**，每次都要手动重输。
4. 无 `--yes` / 幂等语义，重复 `init` 无感知；覆盖时无提示。
5. 空 endpoint 时，其它「启动必须配置」（model / provider）也一并成为隐性问题。

## 2. 现状（代码定位）

| 文件 | 现状 |
|---|---|
| `cli/config.go` | `LoadOrDefaultConfig` 落盘默认空配置；`ApplyEnvOverrides` 仅覆盖 `llm.*` 与 `COMPRESS_MODEL` |
| `cli/init_wizard.go` | `RunInitWizard` 纯交互（endpoint/model/api_key/provider + 审计），写 `data_dir/config.json`（用 `/` 拼接，Windows 可改 `filepath.Join`） |
| `cli/model_factory.go` | `buildModelRequester` 对空 endpoint 返回 `llm.endpoint is required` |
| `cli/runtime.go` | `BuildRuntime` 把该错误包装为 `init agent: …` |
| `cli/cmd/inferglow-cli/main.go` | 分发 `init/team/memory` 子命令；默认进 TUI/REPL/OneShot，未在启动早期做必须配置校验 |

## 3. 目标

- **配置即事实源**：`init` 把 `llm.*` 写入 `config.json`，启动直接从配置读取，不再要求每次手输。
- **三种初始化形态全支持**：交互向导 / 非交互 flag / 环境变量，覆盖首跑与 CI。
- **早期校验**：启动时（进入 TUI 之前）检测缺失的必须配置，缺失则给出清晰引导（提示跑 `init`），
  而不是在 agent 构建阶段抛晦涩错误。
- **幂等且安全**：重复 `init` 不破坏已有配置；覆盖前提示，可备份。

## 4. 设计

### 4.1 命令签名（建议）

```
inferglow init [flags]

Flags:
  --endpoint <url>    LLM API 端点（如 http://host:8200/v1）
  --model <name>      LLM 模型名
  --provider <name>   提供方（openai/deepseek/anthropic/qwen/glm/kimi/mimo，默认 openai）
  --api-key <key>     API Key（可用 LLM_API_KEY 替代）
  --config <path>     配置文件路径（默认 ~/.inferglow/config.json）
  --yes, -y           非交互；缺失必需项时直接报错而非进入向导
  --force             覆盖已存在的 llm 配置（默认已配置则跳过）
  --from-env          优先以 LLM_* 环境变量为默认值
  --show              只打印当前配置摘要，不写入
```

### 4.2 预填默认值（优先级从高到低）

1. 显式 flag（`--endpoint/--model/--provider/--api-key`）
2. 环境变量（`LLM_ENDPOINT` / `LLM_MODEL` / `LLM_PROVIDER` / `LLM_API_KEY`，`--from-env` 时前置）
3. 已有配置文件里的值（保留用户此前设置）
4. 内置默认（`model="gpt-4o"`、`provider="openai"`）

### 4.3 行为

- **TTY + 有缺项** → 交互向导，提示框**预填上述默认值**，回车即采用。
- **非 TTY（`--yes` 或无 stdin）** → 若 `endpoint`、`model` 已齐（flag/env/已有配置）则直接写入；
  否则列出缺失项并报错退出（不进入向导）。
- **写入**：保留配置文件里**未被覆盖**的其它字段（data_dir/features/tui/audit…），仅更新 `llm.*`；
  使用 `SaveConfig`（已 `MarshalIndent`）或 `filepath.Join` 修掉 Windows 路径 `/` 拼接问题。
- **幂等**：`llm.endpoint` 已配置且未 `--force` 时，打印摘要并提示「已初始化，如需重配加 `--force`」。
- **备份**：覆盖已有配置前，先把原文件复制为 `config.json.bak`（可逆）。

### 4.4 启动早期校验（`main.go`）

在 `LoadOrDefaultConfig + ApplyEnvOverrides + flag 覆盖` 之后、分发到 TUI/REPL/OneShot 之前：

```
if cfg.LLM.Endpoint == "" {
    if isTTY → 提示 "未配置 LLM 端点，正在进入初始化向导…" 并调用 init（保留默认值）
    else     → stderr 报错 "llm.endpoint is required — 请运行: inferglow init --endpoint <url> --model <name>"
              并以非零码退出
}
```

这样把错误从 `buildModelRequester`（agent 构建深处）**提前**到启动边界，提示可操作。

### 4.5 `CheckFirstRun` 复用

`cli/init_wizard.go` 已有 `CheckFirstRun`（探测 endpoint 或 `OPENAI_API_KEY`/`OPENAI_BASE_URL`/`LLM_ENDPOINT`）。
早期校验可直接复用它，减少重复逻辑。

## 5. 分阶段落地（后续，不在本次范围）

| 阶段 | 内容 |
|---|---|
| P0（最小可用） | 非交互 flag + 预填 env/已有配置 + `--yes` + 幂等；启动早期校验复用 `CheckFirstRun` |
| P1 | `--force` 备份 + `--show`；Windows 路径 `filepath.Join` 修正 |
| P2 | 缺失项清单式报错；多 provider 提示（deepseek/anthropic 等专属字段） |

## 6. 验收标准

- [ ] 空 `llm.endpoint` 启动时：TTY 引导 init / 非 TTY 报出可执行提示，**不再**是 agent 构建深处抛 `init agent: build model: llm.endpoint is required`
- [ ] `inferglow init --endpoint X --model Y --provider openai --yes` 一次写入 `config.json`，重复执行幂等
- [ ] 未指定值时能回退到 `LLM_*` 环境变量与已有配置
- [ ] 覆盖已有配置前生成 `config.json.bak`
- [ ] 写入后 `config.json` 其余字段（features/tui/audit/data_dir…）不被清空

## 7. 非目标 / 风险

- 不做多 provider 密钥托管 / 加密（建议密钥走环境变量）。
- `api_key` 明文落盘属已知权衡；文档明确建议敏感环境用 `LLM_API_KEY`。
- 行为变更会影响现有 `init` 交互——需在 P0 冒烟测试覆盖 TTY 与非 TTY 两分支。
