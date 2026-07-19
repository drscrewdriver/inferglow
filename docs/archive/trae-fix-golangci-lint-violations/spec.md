# 修复 golangci-lint 违规 Spec

## Why
`implement-p0-maturity-improvements` 的 checklist 标注 "`golangci-lint run` 无 error" 已完成，但实际验证发现全 14 个子模块共约 118 处 lint 违规（gofmt 格式化、govet `reflect.Ptr`、revive exported 缺注释与 stuttering），导致 CI 的 lint job 必然失败，checklist 项名不副实。本 Spec 彻底修复这些违规，使 CI 转绿、checklist 属实，同时保持现有 API 不变（无 BREAKING 变更）。

## What Changes
- 全量执行 `gofmt -w` 修复所有格式化违规（约 36 处，覆盖全部 14 模块）
- 将 `reflect.Ptr` 替换为 `reflect.Pointer`（Go 新版 inline 检查，二者等价）—— action / builtins / schema（约 6 处）
- 为所有缺失文档注释的 exported 符号（函数/类型/变量/常量/方法）补全注释（revive exported 缺注释类）
- 对 stuttering 类型名（如 `action.ActionExecutor`、`audit.AuditChain`、`session.SessionOption`、`pii.PIIType`、`rbac.RBACMiddleware` 等）添加 `//nolint:revive` 指令抑制，避免破坏性重命名
- **不修改** `.golangci.yaml`（保留现有强约束），**不重命名**任何 exported 符号

## Impact
- Affected specs: `implement-p0-maturity-improvements`（使其 checklist 的 lint 项真正达成）
- Affected code: 14 个子模块的存量 `.go` 文件（action / audit / builtins / components / examples / flow / model / observability / orchestrator / sandbox / schema / security / session / workspace）
- 无 BREAKING 变更：仅添加注释、格式化、`//nolint` 指令与 `reflect.Ptr`→`reflect.Pointer` 等价替换；所有 exported 符号名保持不变

## ADDED Requirements

### Requirement: golangci-lint 零违规
系统 SHALL 在所有含 `go.mod` 的子模块执行 `golangci-lint run`（使用根 `.golangci.yaml`）时返回 exit code 0，无任何 gofmt / govet / revive 报错。

#### Scenario: 全模块 lint 通过
- **WHEN** 对每个含 `go.mod` 的子模块执行 `golangci-lint run --config=<root>/.golangci.yaml ./...`
- **THEN** exit code 为 0
- **AND** 无 gofmt "File is not properly formatted" 报错
- **AND** 无 govet "reflect.Ptr should be inlined" 报错
- **AND** 无 revive "exported ... should have comment" 报错
- **AND** 无未抑制的 revive stuttering 报错

#### Scenario: 格式化修复
- **WHEN** 对所有子模块执行 `gofmt -w`
- **THEN** 所有 `.go` 文件符合 gofmt 标准
- **AND** `gofmt -l .` 输出为空

#### Scenario: reflect.Pointer 等价替换
- **WHEN** 检查 action / builtins / schema 中的 reflect 用法
- **THEN** 所有 `reflect.Ptr` 替换为 `reflect.Pointer`
- **AND** 代码行为不变（两者为同一常量的别名）

#### Scenario: stuttering 非破坏性抑制
- **WHEN** 检查 stuttering 类型名（如 `action.ActionExecutor`）
- **THEN** 该类型声明处添加 `//nolint:revive` 指令
- **AND** 类型名未被重命名（API 保持不变）

## MODIFIED Requirements

### Requirement: implement-p0-maturity-improvements 的 lint checklist 项
原 checklist 标注 "`golangci-lint run` 无 error" 已完成，但实际未达成。本 Spec 修复后该 checklist 项真正成立，CI lint job 转绿。

## REMOVED Requirements
无。
