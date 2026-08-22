# Spec：输入线交付 + TUI 能力补齐（整合两组未实现项）

> 状态：草稿，待用户批准后执行
> 目标仓库：`inferglow-github`（master）
> 上游实现源：dev 仓库 `inferglow`（同日已迁移 agent/session 相关内容）

## Why

inferglow-github 当前只有两类「未实现项目」，均未交付：

1. **组一「输入线交付」**：来自 `docs/requirements/rich-input-composer-prd.md` §8/§9（P0 状态机、P1 剪贴板/图片、P2 rich/raw 渲染）与其依赖的**模型层媒体门控**（验收 10 底座）。
2. **组二「能力补齐」**：来自 `docs/requirements/tui-capability-backlog.md` #1–#5（ask 建议工具 / 对话恢复 agent 重建 / TDD 工作流·证据门 / 视觉桥 / 读屏·看图 agent）。

现状盘点（已实测，见附录 A）：
- **组一在不该动的 dev 仓库已实现并被接入 TUI**，但迁移 spec 把 `cli/`、`model/` 列为「不迁移」，故 ghub **全部缺失**（无 `cli/composer/`、无 `cli/clipboard.go`、无模型层 `gateMultimodal`/`ErrUnsupportedContent`）。
- **组二两仓库均未实现**（仅需求文档），`grep` ghub Go 源码无 `ask_suggestion`/证据门/视觉桥/读屏 agent 任何符号。
- ghub 已具备的既有底座：`model/capability.go` `SupportsVision`/`ModelCapabilityRegistry`、`orchestrator/hitl` 桥 + `AgentCallbacks.OnApprovalRequired`、`cli/` bubbletea TUI（`tui_model.go`/`repl.go`/`tui_view.go`）、富 Composer 所需的 `cli/go.mod` 依赖已在内。

结论：**组一 = 迁移自 dev + 集成 + 验证；组二 = 从零实现**。组一的图片通道+媒体门控是组二 #4/#5 的底座，故按 A→B 分阶段。

---

## What Changes

### Phase A — 组一：输入线交付（迁移 + 集成 + 验证）

**A1 模型层媒体门控迁入**（验收 10 底座，dev 已有）
- 从 dev 按内容迁移：
  - `model/content_block.go`：`ErrUnsupportedContent`（L174）+ `gateMultimodal`（L187，策略：已知模型缺能力即拒绝、未知模型放行）
  - `model/openai.go`（L185 GenerateRequestData 拼接前调用）、`model/anthropic.go`（L102）
  - `model/content_block_gate_test.go`
- 判定：上述 3 个 `.go` 文件为 **HEAD-DIFFERS**（dev 相对 ghub 增量），按「代码对应」仅迁本 hunk；测试为新增文件整迁。保留 ghub 自身其余实现。

**A2 Composer 纯函数状态机迁入**（P0，dev 已有）
- 新增 `cli/composer/` 子包：`rich_composer.go`（M0–M4 + IN-1..6 + OUT-1..5 + tick/Enter 抑制）+ `rich_composer_test.go`。
- 放独立子包原因：进入严格 lint 域（`errcheck` 等），避免连带 cli 根包存量文件。
- 纯函数约定、时间参量默认值（burst 2ms / idle 10ms / enter window 10ms）已在源码内；本包只依赖 stdlib time，无需新增依赖。

**A3 剪贴板迁入**（P1）
- 新增 `cli/clipboard.go`（+ `clipboard_test.go`）：文本复制 OUT-6、剪贴板图片 IN-8→PNG 附件 OUT-7、Windows/macOS/WSL 回退、`ErrClipboardUnavailable` 明确报错不静默。
- 与 dev 一致。

**A4 CLI 接线代码对应迁移**（dev 已实现，ghub 缺失）
- `cli/tui_model.go`、`cli/repl.go`、`cli/tui_view.go`、`cli/tui_approval.go`：按 dev 的 composer 接线做**代码对应**迁移（事件入口先喂 Composer 再走 Bubble Tea update；applyComposerActions 应用 Typed/Paste/InsertNewline/Submit；图片附件挂 M5/OUT-7；复制键 OUT-6；vision 预检 OUT-7 提交前用 `model.SupportsVision` 提示；P2 `render_toggle` IN-9→OUT-8 rich/raw）。
- 四个文件均为 **HEAD-DIFFERS**，逐 hunk 迁移，保留 ghub 其它实现。

**A5 验证（Phase A 出口）**
- PRD §8 十条验收全部过（1–4、8、9 由状态机/单测覆盖；5、6、10 需 TUI 集成验证）。
- `cli/composer/rich_composer_test.go` 行覆盖率 ≥ 90%（P0 硬门槛）。
- 严格 lint：`cli/composer`/`cli/config` 进 errcheck 严格域（见 `docs/requirements/strict-lint-scope.md`）。
- `go test ./cli/... ./model/...` 全绿。

### Phase B — 组二：TUI 能力补齐（从零实现）

依赖 Phase A 交付的图片通道 + 媒体门控；各能力独立可验收。按 backlog #1–#5：

- **B1 ask/user 问题与建议工具**：新增 `ask_suggestion` 工具，串 `orchestrator/hitl` 桥；模型运行中可回溯发起提问建议并汇入会话。验收：模型在运行中可触发提问建议并回显。
- **B2 对话恢复 agent 重建**：现有 `/resume` 仅列历史会话并显示目标，补「实际切换 + agent 重建」；恢复后 Composer 从 M0 重新开始。验收：`/resume <id>` 完成真实会话切换并进入可对话态。
- **B3 TDD 工作流 / 证据门**：无 TDD 编排、无证据门强制；补工作流编排与证据门强制门槛（新 spec 细化）。验收：TDD 流程可编排，证据不足时阻断。
- **B4 视觉桥（ask/vision）**：把图片附件喂给视觉模型并回读结果；复用 A1 门控 + A3 图片通道。验收：对 AI `vision` 模型传图可读回；非 vision 模型被 A1 门控拒绝。
- **B5 读屏 / 看图 agent**：把 `ContentImage` 挂用户消息，依赖既有 provider 编码，**模型层 0 改动**；TUI 侧完成附件回显。验收：会话产生可视化图片附件并进入模型请求。

> B1–B5 具体行为约束、用户触发入口、时间/批参默认值，立项时从 `docs/requirements/tui-capability-backlog.md` 迁为独立需求族/spec，并同步更新 backlog 表。

---

## Impact

- 新增：`cli/composer/`、`cli/clipboard.go`(+test)、`model/content_block_gate_test.go`，及 B 组各能力文件（`ask_suggestion` 工具、证据门、视觉桥、看图 agent）。
- 修改（代码对应）：`model/{content_block,openai,anthropic}.go`；`cli/{tui_model,repl,tui_view,tui_approval}.go`；B 组触及 `orchestrator/hitl`、`orchestrator/agent`（新工具注册）等。
- 行为：粘贴行为确定性（P0）、剪贴板双向（P1）、rich/raw（P2）；图片仅路由 vision 模型（A1 门控）；若干新产品能力（B 组）。均向后兼容。
- 验收命令：`go test ./cli/... ./model/...`（A）+ 各组新增包 `go test ./...` 全绿。

---

## Requirements

### REQ-A 组一「仅迁移、可验证」
Phase A 所有迁移 SHALL 限定在附录 A2 迁移判定表：HEAD-DIFFERS 文件逐 hunk 代码对应、新增文件整迁；不得整文件覆盖引入 dev 无关改动。出口 SHALL 满足 PRD §8 十条验收且 `cli/composer` 行覆盖率 ≥ 90%。

#### Scenario: HEAD-DIFFERS 文件
- **WHEN** 迁移 `model/*.go`、`cli/{tui_model,repl,tui_view,tui_approval}.go`
- **THEN** 仅应用组一相关 hunk，保留 ghub 自身实现，`go build ./...` 与 `go test ./cli/... ./model/...` 通过

#### Scenario: 新增文件
- **WHEN** 迁移 `cli/composer/`、`cli/clipboard.go`
- **THEN** 文件内容与 dev 一致，状态机单测稳定，覆盖率 ≥ 90%

### REQ-B 组二「从零实现、独立验收」
B1–B5 SHALL 各自实现并可通过独立验收；任何一项不得依赖未交付的 Phase A（B4/B5 依赖 A3/A1，须在 A 后立项）。

### REQ-C 审阅门槛
任何代码改动 SHALL 在本 spec 通过用户批准后才允许执行；批准前禁止改动 `inferglow-github` 文件。

---

## 附录 A：现状证据（实测）

| 项 | dev `inferglow` | ghub `inferglow-github` |
|---|---|---|
| `cli/composer/rich_composer.go` + test | ✅ 存在、已接入 `tui_model.go` | ❌ 不存在 |
| `cli/clipboard.go` + test | ✅ 存在 | ❌ 不存在 |
| `model` 媒体门控 `gateMultimodal`/`ErrUnsupportedContent` | ✅ `content_block.go` L174/L187，`openai.go` L185，`anthropic.go` L102 调用 | ❌ 不存在（仅 `capability.go` `SupportsVision` L139） |
| `cli/config/` + `configure.go`（provider 存储） | ✅ 存在 | ❌ 不存在（**不在本 spec 两组范围**，独立需求族） |
| backlog #1–#5 能力 | ❌ 无实现 | ❌ 无实现（仅 `docs/requirements/tui-capability-backlog.md`） |

## 附录 B：Phase A 迁移判定表

| 文件 | 基线条目 | 迁移方式 |
|---|---|---|
| `model/content_block.go` | HEAD-DIFFERS | 逐 hunk：`ErrUnsupportedContent` + `gateMultimodal` |
| `model/openai.go` | HEAD-DIFFERS | 逐 hunk：`GenerateRequestData` 拼接前调用门控 |
| `model/anthropic.go` | HEAD-DIFFERS | 逐 hunk：同上 |
| `model/content_block_gate_test.go` | 新增 | 整迁 |
| `cli/composer/rich_composer.go` | 新增 | 整迁（含严格 lint 注释） |
| `cli/composer/rich_composer_test.go` | 新增 | 整迁 |
| `cli/clipboard.go` | 新增 | 整迁 |
| `cli/clipboard_test.go` | 新增 | 整迁 |
| `cli/tui_model.go` | HEAD-DIFFERS | 逐 hunk：composer 接线 / 图片 / 复制 / vision 预检 / render 切换 |
| `cli/repl.go` | HEAD-DIFFERS | 逐 hunk：事件入口改喂 Composer |
| `cli/tui_view.go` | HEAD-DIFFERS | 逐 hunk：rich/raw 渲染维度 |
| `cli/tui_approval.go` | HEAD-DIFFERS | 逐 hunk：提交带审批与 OUT-4 串联 |

## 附录 C：明确的「不做」范围

- `docs/requirements/provider-config-store.md`（dev 独有、独立需求族，非本 spec 两组）
- 代码检索 / 记忆跨会话召回（`context/` + `memory/` 已具备，PRD §12 #6）
- LSP / 主体主题（已具备）/ `model/` 多模态共识层（已具备）