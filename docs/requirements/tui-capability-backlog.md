# 需求族：TUI 能力补齐 backlog（非 input 线后续阶段）

> 状态：**已交付 ✅**（B1–B5 全部实现并随 `rich-input-capability-delivery` spec 落地）
> 来源：从 [rich-input-composer-prd.md](../rich-input-composer-prd.md) §12 #2/#3/#5 + §11.4 展望拆出（v0.4 拆分）
> 原则：input PRD 只交付这些能力的**输入底座**（图片附件通道等），能力本体在本族另行立项。

## 能力清单

| # | 能力 | 现状（已交付 ✅） | 依赖 | 备注 |
|---|------|------|------|------|
| 1 | ask/user 问题与建议工具 | ✅ `builtins/actions/ask_suggestion.go`：模型可调用的提问建议工具（SideEffectNone / ExposeToModel），host 经 HITL 回填答案；注册进三 policy | hitl 桥 | `AskSuggestionResult{Status,Question,NeedsOperator}` |
| 2 | 对话恢复 | ✅ `/resume <id>` 由「列信息」升级为「真实切换」：校验 `sessions/<id>.jsonl` → RunTUI resume 循环重建 runtime | agent 重建 | 恢复后新 composer 从 M0 重新开始 |
| 3 | TDD 工作流 / 证据门 | ✅ `orchestrator/evidence` 声明式证据门策略引擎（`Gate.Evaluate` deny-by-default；`PresetTDDGate`/`ConsequenceGate`） | — | 证据不足即阻断并返回可读原因 |
| 4 | 视觉桥（ask/vision） | ✅ `/vision`（别名 `/see`）命令：读图→`model.ImageBlock`→`agent.WithContentBlocks` 送模型并回读；非 vision 由 TUI 预检+模型层 `gateMultimodal` 兜底 | #2 引擎多模态通道 + 媒体门控（已交付） | 复用 #2 多模态输入 |
| 5 | 读屏/看图 agent（展望） | ✅ `cli/vision.go` `runVision` 挂 `ContentImage` 用户消息并回读（模型层 0 改动，依赖既有 provider 编码） | 同 #4 | TUI 回显提问气泡 + 助手文本块 |

## 备注

- B1–B5 均已实现并有测试（`builtins/actions/ask_suggestion_test.go`、`cli/resume_test.go`、`orchestrator/evidence/evidence_test.go`、`cli/vision_test.go`），落地见 `.trae/specs/rich-input-capability-delivery/`。
- #1–#3 在上游 Codex 能力对齐盘点（原 PRD §12）中为 ⚠️ 需补 / ⚠️ 部分；#4/#5 为 ❌ 需补——现已全部补齐。
