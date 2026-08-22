# 需求族：provider 配置长期存储域（YAML 单一事实源 + 配置流程）

> 状态：**已交付 ✅**（2026-08-22 核查）
> 领域：`cli/config/` 配置域 + `cli/configure.go`
> 来源：从 [rich-input-composer-prd.md](../rich-input-composer-prd.md) §13.2 拆出（v0.4 拆分）
> 定位：保留设计记录与验收（防回归）。

## 背景

provider 配置原本只能走 env / config.json / flag 注入，无法长期持久化多 provider。

**交付状态**：[cli/config/config.go](file:///e:/test/rewrite-agently/inferglow/cli/config/config.go)
实现本文件全部建议 API（`Manager/Load/Save/UpsertProvider/RemoveProvider/ActiveProvider/BlockValues/
KnownProviders/Scaffold`，Save 为 tmp+rename 原子写）；
[cli/configure.go](file:///e:/test/rewrite-agently/inferglow/cli/configure.go) 实现
`configure`（多问题向导）/ `configure list` / `configure show <name>`（api_key 脱敏）。
[model_factory.go](file:///e:/test/rewrite-agently/inferglow/cli/model_factory.go) 的
`resolveProvider` 已按「config 域优先、CLIConfig.LLM 回退」接入。

## 决策：YAML 作为唯一权威事实源（source of truth），不写 sqlite

| 存储 | 角色 |
|------|------|
| **`providers.yaml`（新建）** | provider 配置的唯一权威事实源；人手编辑 + CLI 向导写**同一份** |
| `sqlite`（既有） | 只承载**运行期记录**（audit / session / memory），不承载配置 |

**兼容性论证（回答「配置进 sqlite 会怎样」）**：若把 provider 配置放进 sqlite，会形成
「人手编辑文件 ↔ 运行期 sqlite」两处事实源。向导/文件编辑与 sqlite 写入互相不知情，必然脱节；
文件天然可 diff、可进版本库、可 grep。**结论：配置走文件，记录走 sqlite，职责分离。**

## 文件布局

默认 `<DataDir>/providers.yaml`：

```yaml
providers:
  openai:
    base_url: https://api.openai.com/v1
    api_key: sk-xxx
    model: gpt-4o
    full_url: ""            # 可选：整体覆盖 base_url+路径
    content_mapping: {}     # 可选：非标准 SSE 字段路径
    settings: {}            # 可选：provider 特定子树（temperature 等）
  deepseek:
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat
current: openai             # 当前激活 provider（空 = 未激活，走回退）
```

`ProviderBlock` 字段与 `model.StaticConfigProvider` 读取的 dot-path（`<name>.base_url` 等）**完全对齐**，
可无损耗喂给现有模型工厂。

## 运行优先级

- **provider 选择**：`providers.yaml` 激活 provider（`current`，权威）**优先**；未配置/未激活时回退
  `CLIConfig.LLM`（env / config.json / flags）——见 `resolveProvider` 实现。
- **字段级**：YAML 显式字段优先生效；YAML 里 `api_key` 等缺省字段可由 env 注入兜底（对标既有
  `model.EnvConfigProvider` 的补充语义）。env 的定位是**应急覆盖 / CI / 密钥交付**：不想被 YAML
  覆盖时清空 `current` 或移除该 provider 块即可回退 env 路径。
- 注：早期 PRD 版本曾写「env 仍最高（providers.yaml < env）」与实现相反，以本节为准。

## 配置流程：独立子命令 + 非 TUI 多问题问答 `inferglow configure`

- `inferglow configure list` / `show <name>` —— 只读展示（`api_key` 脱敏），不改文件；
- `inferglow configure`（无参）—— 进入 pre-TUI 的多问题向导：一问一答填 provider/base_url/api_key/model，
  支持**增量修改**（空回车 = 保留现值）、任意次运行、询问是否设为激活 provider；
- 手动 `$EDITOR providers.yaml` 亦可——与向导写的是同一份文件，天然一致。

## API（已实现，`cli/config` 包）

`Manager{Path}` + `NewManager`/`DefaultPath(dataDir)`/`Load`/`Save`(+tmp+rename 原子写)/
`UpsertProvider(name, blk, activate)`/`RemoveProvider`/`ActiveProvider() (name, blk, err)`/
`BlockValues(name, blk) map`/`KnownProviders()`/`Scaffold()`
（后者依据 `model.DEFAULT_SETTINGS` 生成带默认 base_url/model 的骨架，供向导直接改值而非从零敲）。

## 验收（已达成 ✅）

1. 手动编辑 YAML → 运行时直接生效（无需重启进程内同步，Manager 不缓存、每次读盘）。
2. `configure list/show` 脱敏展示；`configure` 向导完成一次多 provider 配置并落盘（`configure_test.go`）。
3. 已激活 provider 的 base_url/model 经 `buildModelRequester`/`resolveProvider` 生效，优先于
   env/config.json/flags 回退路径；YAML 未配置或未激活时回退行为不变。
