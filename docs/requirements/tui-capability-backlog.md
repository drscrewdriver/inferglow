# 需求族：TUI 能力补齐 backlog（非 input 线后续阶段）

> 状态：**规划中 📋**（未排期，不占 input PRD 里程碑 P0–P2）
> 来源：从 [rich-input-composer-prd.md](../rich-input-composer-prd.md) §12 #2/#3/#5 + §11.4 展望拆出（v0.4 拆分）
> 原则：input PRD 只交付这些能力的**输入底座**（图片附件通道等），能力本体在本族另行立项。

## 能力清单

| # | 能力 | 现状 | 依赖 | 备注 |
|---|------|------|------|------|
| 1 | ask/user 问题与建议工具 | `orchestrator/hitl`（人工介入桥）与 `approval/` 审批流已有，但**无对话内 ask 建议工具**（无 tool 让模型在运行中回溯提问建议） | hitl 桥 | 新 `ask_suggestion` 工具，串到 hitl 桥 |
| 2 | 对话恢复 | [tui_session.go](file:///e:/test/rewrite-agently/inferglow/cli/tui_session.go) `/resume [id]` 能列出历史会话并显示目标，但**实际切换需 agent 重建，已延期** | agent 重建 | 对 input 线无影响（Composer 状态与会话无关，恢复后从 M0 重新开始） |
| 3 | TDD 工作流 / 证据门 | 未接入 | — | 无 TDD 编排、无证据门强制 |
| 4 | 视觉桥（ask/vision） | 未接入，「看读」能力待补 | input PRD P1 图片附件底座 + [媒体能力门控](media-capability-gating.md)（已交付） | 依赖图片通道 |
| 5 | 读屏/看图 agent（展望） | 未接入 | 同 #4 | 只需把 `ContentImage` 挂到用户消息并依赖既有 provider 编码，模型层 0 改动 |

## 备注

- #1–#3 在上游 Codex 能力对齐盘点（原 PRD §12）中为 ⚠️ 需补 / ⚠️ 部分；#4/#5 为 ❌ 需补。
- 各能力立项时从本表迁出为独立需求族文件（或 spec），并同步更新本表与
  [README.md](README.md) 索引。
