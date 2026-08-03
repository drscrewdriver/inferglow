# Fix Lint Issues Spec

## Why

GitHub Actions CI 中 golangci-lint 在 `flow/` 子模块发现 10 个 lint 问题（revive + govet + gofmt），导致 lint 步骤有 error 注解。需要修复使 CI 完全变绿，且不改变运行时行为。

## What Changes

### 类别 A：gofmt 格式化（零风险）
- `flow/stage/builtin/stages.go`：运行 `gofmt -w` 修复格式
- `flow/port.go`：运行 `gofmt -w` 修复格式
- `flow/flow_context.go`：运行 `gofmt -w` 修复格式

### 类别 B：govet/inline 常量替换（零风险）
- `flow/flowdef/expr.go`：`reflect.Ptr` → `reflect.Pointer`（Go 1.17+ 已弃用 Ptr，推荐 Pointer）

### 类别 C：revive/exported 缺少注释（零风险）
- `flow/stage/meta.go`：在 `const (` 块前添加 `// Port types` 注释
- `flow/port.go`：在 `const (` 块前添加 `// Port types` 注释

### 类别 D：revive/exported 名称 stutter（加 nolint 抑制，避免破坏性改名）
- `flow/stage/registry.go#L31`：`type StageFunc` → 添加 `//nolint:revive` 抑制
- `flow/stage/meta.go#L53`：`type StageMeta` → 添加 `//nolint:revive` 抑制
- `flow/flow_context.go#L71`：`type FlowContext` → 添加 `//nolint:revive` 抑制
- `flow/flow_context.go#L163`：`func FlowContextFrom` → 添加 `//nolint:revive` 抑制

**不改名的原因**：`FlowContext` 被 19 个文件引用，`FlowContextFrom` 被 11 个文件引用，`StageFunc`/`StageMeta` 被跨包使用。改名会引入大量侵入性变更，影响代码可读性且零功能收益。`//nolint:revive` 是 Go 社区处理已知 stutter 的标准做法。

## Impact
- Affected code: 6 个源文件
- No functional changes: 所有修复均为格式化/注释/nolint 抑制
- CI lint 步骤预期完全变绿

## ADDED Requirements

### Requirement: Lint 修复
The system SHALL fix all 10 lint issues without changing runtime behavior.

#### Scenario: 修复后 CI 通过
- **WHEN** 推送修复后的代码到 GitHub
- **THEN** CI lint 步骤应无 errors 通过

#### Scenario: 功能不变
- **WHEN** 运行 `go test ./...`
- **THEN** 所有测试通过，无行为变更