# Fix CI All Spec

## Why

GitHub Actions CI 有 4 类问题导致全部 job 失败：vet/test 根模块无守卫退出码 1，lint 在剩余子模块还有 10 个问题。需要修复使 CI 完全变绿。

## What Changes

### 类别 A：CI 工作流修复（vet/test 根模块守卫）
- `.github/workflows/ci.yml`：在 `Run go vet (root module)` 和 `Run go test (root module)` 步骤前加 `ls *.go 2>/dev/null` 守卫，根模块无 Go 文件时跳过

### 类别 B：gofmt 格式化（9 个文件，零风险）
- `observability/collector.go`：字段对齐修复
- `workspace/identity.go`：字段对齐修复
- `sandbox/policy_test.go`：注释对齐修复
- `sandbox/landlock.go`：字段对齐修复
- `rerank/fallback_test.go`：字段对齐修复
- `rerank/cohere.go`：字段对齐修复
- `flow/port_test.go`：末尾 missing newline
- `flow/port_resolver.go`：末尾 missing newline
- `flow/flowdef/ports.go`：末尾 missing newline

### 类别 C：revive/exported stutter（1 处，加 nolint 抑制）
- `workspace/context_source.go`：`WorkspaceContextSource` 重命名为 `ContextSource`，加 `type WorkspaceContextSource = ContextSource` 别名（`//nolint:revive`）

## Impact
- Affected code: 11 个文件（1 个 CI 配置 + 10 个源文件）
- No functional changes: 所有修复均为格式化/守卫/nolint 抑制
- 预期 CI 全部 5 个 job 变绿

## ADDED Requirements

### Requirement: CI 修复
The system SHALL fix all CI failures without changing runtime behavior.

#### Scenario: 修复后 CI 通过
- **WHEN** 推送修复后的代码到 GitHub
- **THEN** CI 所有 5 个 job（lint / vet-default / vet-with_sandbox / test-default / test-with_sandbox）应全部通过

#### Scenario: 功能不变
- **WHEN** 运行 `go vet ./...` 和 `go test ./...` 在各子模块
- **THEN** 所有检查通过，无行为变更