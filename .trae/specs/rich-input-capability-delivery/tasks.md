# Tasks

> 关联 spec：`rich-input-capability-delivery/spec.md`
> 目标仓库：`inferglow-github`（master）。组一（Phase A）= 从 dev 迁移+集成+验证；组二（Phase B）= 从零实现。
> 标记约定：`[ ]` 未做 / `[x]` 已做（勾选即视为该 task 验收通过；迁移类需 `go build/test` + 相关断言通过才算）。

## Phase A — 组一：输入线交付（迁移 + 集成 + 验证）

- [x] Task 1: 迁移前底盘校验（前置，无依赖）
  - [x] SubTask 1.1: 核对 dev `inferglow` 与 ghub 的 HEAD 基线，确认附录 B 迁移判定表（内容块/门控/composer/clipboard/TUI 接线）仍成立
  - [x] SubTask 1.2: 校验 ghub `cli/go.mod` 依赖齐备（bubbletea/bubbles/lipgloss 已存在；composer 仅 stdlib `time`；clipboard 需 `atotto/clipboard` 已以 indirect 存在 module cache）
  - [x] SubTask 1.3: `git status` 确认 ghub 工作树干净（仅 spec 目录未跟踪）
- [x] Task 2: A1 — 模型层媒体门控迁入（验收 10 底座）
  - [x] SubTask 2.1: `model/content_block.go` 逐 hunk 迁 `ErrUnsupportedContent` + `gateMultimodal`（已知模型缺能力拒绝、未知放行），保留 ghub 其余实现
  - [x] SubTask 2.2: `model/openai.go`、`model/anthropic.go` 逐 hunk 在 `GenerateRequestData` 拼接 ContentBlocks 前调用 `gateMultimodal`
  - [x] SubTask 2.3: 迁 `model/content_block_gate_test.go`（gpt-4 拒绝 / gpt-4o 放行 / 未知模型放行 / audio 拒绝）
  - [x] SubTask 2.4: `go test ./model/...` 全绿
- [x] Task 3: A2 — Composer 纯函数状态机迁入（P0）
  - [x] SubTask 3.1: 整迁 `cli/composer/rich_composer.go`（M0–M4 + IN-1..6 + OUT-1..5 + tick/Enter 抑制 + `DefaultConfig` 时间参量）
  - [x] SubTask 3.2: 整迁 `cli/composer/rich_composer_test.go`（覆盖 PRD 验收 1/2/3/4/8/9）
  - [x] SubTask 3.3: 加入严格 lint 域（`cli/.golangci-strict.yaml` + Makefile `lint-strict` 已建；本地受 golangci go1.24<go1.25 工具链限制无法执行，见 Task 6.3 注记）
  - [x] SubTask 3.4: `go test ./cli/composer/...` 全绿，行覆盖率 **98.3%** ≥ 90%（P0 硬门槛）
- [x] Task 4: A3 — 剪贴板桥迁入（P1）
  - [x] SubTask 4.1: 整迁 `cli/clipboard.go`+`clipboard_test.go`（文本读/写、图片→PNG 临时文件、`ErrClipboardUnavailable`、Win/WSL/mac/Linux 回退）
  - [x] SubTask 4.2: `go test ./cli/...` 通过（clipboard 相关用例）
- [x] Task 5: A4 — CLI 接线逐 hunk 迁移（保留 ghub 实现；**剔除 dev 混入的 Steer 三档抢占接线**——非 PRD 输入线范围）
  - [x] SubTask 5.1: `cli/tui_model.go`：composerTick 类型/命令 + `applyComposerActions`；事件入口喂 Composer（PlainChar/NoHold/ModifiedInput/Enter/Tick）；提交后 Reset
  - [x] SubTask 5.2: `repl.go`/`tui_view.go`/`tui_approval.go` 经 diff 确认为 dev/ghub **全等**，无需改动
  - [x] SubTask 5.3: P2 `render_toggle` rich/raw 渲染：新增 `renderRaw` 状态 + `Ctrl+R` 切换键 + `renderTranscript` 富/纯文本分支（已完成并 build/test 通过）
  - [x] SubTask 5.4: P1 触发入口接线：粘贴键 `Ctrl+V`(兜底 `Ctrl+Y`) 读剪贴板（图→`pendingImage` 附件暂存 /文本→OUT-2）；复制键 `Ctrl+C` 选区→OUT-6；突发中剪贴板读取先冲刷（IN-5）；新增 `unicode/utf8`/`errors`/`path/filepath` 导入
  - [x] SubTask 5.5: ctrl+c 选区复制复用既有 `copySelection()`（ghub 无 dev 的 `selectText`）
  - [x] SubTask 5.6: 提交前 vision 预检（验收 10 TUI 体验侧）已实现：`model.LookupModelCapability` 已知非 vision → 可读告警 + 丢弃 `pendingImage`，不静默发送（配合 Task 2 模型层 `gateMultimodal` 兜底）
  - [x] SubTask 5.7: `go build ./...` 0 + `go vet ./cli/...` 0 通过
- [x] Task 6: A5 — Phase A 出口验收
  - [x] SubTask 6.1: PRD §8 验收：1/2/3/4/8/9 composer 单测覆盖（98.3%）；5/6 剪贴板已接线；7(P2 rich/raw) 已实现；10 TUI 预检已实现 + 模型门控已测
  - [x] SubTask 6.2: `cli/composer` 行覆盖率 98.3% ≥ 90%
  - [ ] SubTask 6.3: `make lint-strict` —— **本地无法运行**：golangci 1.64.8(go1.24) < cli 模块 target go1.25（仓库级限制，全局 `make lint` 同受影响）；配置已就绪，CI(go1.25 工具链) 或升级本地 golangci 后可跑
  - [x] SubTask 6.4: `go test ./cli/... ./model/...` 全绿
  - [x] SubTask 6.5: **#2（图片→模型实际发送）已实现**：`Agent.Run` 新增 `WithContentBlocks` RunOption；`session` 复用 `ChatMessage.Content any`（string|[]ContentBlock）；`SessionExtension.AddUserContentBlocks` 存文本+媒体块；`PreparePrompt` 把 `[]model.ContentBlock` 映射进 `model.ChatMessage.ContentBlocks`；cli 提交把 `pendingImage` 读为 `ImageBlock`→`WithContentBlocks`。agent `internal/extension` + `agent` 全量测试通过（新 `contentblocks_test.go` 覆盖透传与文本无回归）。`go test ./orchestrator/agent/...` 全绿

## Phase B — 组二：TUI 能力补齐（从零实现）

> B4/B5 依赖 Phase A（A3 图片通道 + A1 媒体门控）；B1 依赖既有 `orchestrator/hitl` 桥。

- [x] Task 7: B1 — ask/user 问题与建议工具
  - [x] SubTask 7.1: 新增 `ask_suggestion` builtin action（[ask_suggestion.go](file:///e:/test/rewrite-agently/inferglow-github/builtins/actions/ask_suggestion.go)）+ 注册进 Restrictive/Balanced/Permissive 三 policy
  - [x] SubTask 7.2: 契约：SideEffectNone、ExposeToModel、`AskSuggestionResult{Status,Question,NeedsOperator}`（host 经 HITL 回填答案，动作可独立测试）
  - [x] SubTask 7.3: `ask_suggestion_test.go`（缺 question 报错 / 正常 posed）+ policies_test 注册断言已含 ask_suggestion
  - [x] SubTask 7.4: `go test ./builtins/...` 通过（ask_suggestion + policies）
- [x] Task 8: B2 — 对话恢复 agent 重建
  - [x] SubTask 8.1: `/resume <id>` 由「列信息」升级为「真实切换」：校验 `sessions/<id>.jsonl` 存在 → 置 `restartResumeID` → `(tea.Quit,true)` → RunTUI resume 循环用新 id 重建 runtime
  - [x] SubTask 8.2: 恢复后重新进入（新建 composer 实例 M0 起）；输入态与会话解耦
  - [x] SubTask 8.3: `resume_test.go`（`sessionFileExists`：存在/缺失/路径穿越/空 id）+ cli `go test ./...` 全绿
- [x] Task 9: B3 — TDD 工作流 / 证据门
  - [x] SubTask 9.1: 新增 `orchestrator/evidence` 包：声明式证据门策略引擎（[evidence.go](file:///e:/test/rewrite-agently/inferglow-github/orchestrator/evidence/evidence.go)）
  - [x] SubTask 9.2: `Gate.Evaluate` deny-by-default；`PresetTDDGate`（tests+coverage[+lint]）、`ConsequenceGate`（tests+verification）
  - [x] SubTask 9.3: 证据不足即阻断并返回可读 unmet 原因；`RequiresLint` 帮助
  - [x] SubTask 9.4: `evidence_test.go`（allow/deny/verification/lint/nil 防御）`go test ./orchestrator/evidence/...` 全绿
- [x] Task 10: B4 — 视觉桥（ask/vision）
  - [x] SubTask 10.1: `/vision`（别名 `/see`）命令：读图→`model.ImageBlock`→`agent.WithContentBlocks`（复用 #2 引擎多模态通道）
  - [x] SubTask 10.2: 非 vision 模型路径由 TUI 预检丢弃 + 模型层 `gateMultimodal` 兜底（`ErrUnsupportedContent`）
  - [x] SubTask 10.3: `vision.go` `buildImageBlocks`/`mimeForPath` + `vision_test.go` 全绿
- [x] Task 11: B5 — 读屏 / 看图 agent
  - [x] SubTask 11.1: runVision 挂用户消息给模型并回读（模型层 0 改动，依赖既有 provider 编码）
  - [x] SubTask 11.2: TUI 回显用户提问气泡 + 助手文本块
  - [x] SubTask 11.3: 测试 + 验收（cli `go test ./...` 全绿）
- [ ] Task 12: B 组总验收 + 文档
  - [x] SubTask 12.1: Phase B 各包 go test 全绿（builtins/orchestrator(evidence,agent)/model/cli）
  - [ ] SubTask 12.2: 同步 `docs/requirements/tui-capability-backlog.md` 状态（待做）

# Task Dependencies

- [Task 1] 前置，无依赖；[Task 2]..[Task 6] 均在 [Task 1] 后
- [Task 2] 独立，可与 [Task 3]/[Task 4] 并行（模型层与 cli 解耦）
- [Task 3] 独立（composer 纯函数子包，无 TUI 依赖）
- [Task 4] 独立；其触发入口接线在 [Task 5]
- [Task 5] depends on [Task 3]（接线需状态机）与 [Task 4]（触发入口）；贵 timing 与 [Task 2] 无关
- [Task 6] depends on [Task 2]..[Task 5]
- [Task 7] 依赖既有 `orchestrator/hitl`，无 Phase A 依赖
- [Task 8] 依赖既有 `/resume`，无 Phase A 依赖
- [Task 9] 无外部依赖（可独立立项）
- [Task 10] depends on [Task 4]+[Task 2]（图片通道 + 门控）
- [Task 11] depends on [Task 4]（附件通道）
- [Task 12] depends on [Task 7]..[Task 11]

> 注：Task 5（A4）为 HEAD-DIFFERS 逐 hunk 迁移，是 Phase A 中唯一需要「代码对应」谨慎处理的任务，严禁整文件覆盖引入 dev 无关改动。