# Tasks

## 阶段 1：自动化无脑修复（无依赖，可并行）

- [x] Task 1: 修复 gofmt 格式化违规（全模块自动化）
  - [x] SubTask 1.1: 对 14 个子模块（action/audit/builtins/components/examples/flow/model/observability/orchestrator/sandbox/schema/security/session/workspace）执行 `gofmt -w .`
  - [x] SubTask 1.2: 执行 `gofmt -l .`（含各子模块）验证输出为空

- [x] Task 2: 修复 govet reflect.Ptr 违规
  - [x] SubTask 2.1: 将 action/local_executor.go 中 `reflect.Ptr` 替换为 `reflect.Pointer`（2 处，约 L260、L308）
  - [x] SubTask 2.2: 将 builtins/tools/schema_from_func.go 中 `reflect.Ptr` 替换为 `reflect.Pointer`（约 L113）
  - [x] SubTask 2.3: 排查并修复 schema 模块中所有 `reflect.Ptr` 用法（约 3 处）
  - [x] SubTask 2.4: 验证 govet 无 "reflect.Ptr should be inlined" 报错

## 阶段 2：revive exported 缺失注释修复（按模块并行，无依赖）

- [x] Task 3: 为缺失注释的 exported 符号补全文档注释
  - [x] SubTask 3.1: action 模块（约 10 处，如 SideEffectNone、DecisionPlan 等常量及部分类型）
  - [x] SubTask 3.2: audit 模块（约 5 处，如 ExportJSON 常量及部分类型）
  - [x] SubTask 3.3: flow 模块（约 6 处）
  - [x] SubTask 3.4: model 模块（约 12 处）
  - [x] SubTask 3.5: sandbox 模块（约 21 处，最多）
  - [x] SubTask 3.6: session 模块（约 12 处，如 SessionData、ToJSON/ToYAML/SaveJSON/SaveYAML/LoadJSON/LoadYAML/LoadFromData、ResizeHandler、Session、NewSession 等）
  - [x] SubTask 3.7: security 模块（约 5 处，如 PIIType、Email 常量、SideEffectNone 等）
  - [x] SubTask 3.8: schema 模块（约 3 处）
  - [x] SubTask 3.9: observability / orchestrator / components 等剩余模块（各 1-2 处）

## 阶段 3：revive stuttering 抑制（与阶段 2 可并行）

- [x] Task 4: 对 stuttering 类型名添加 `//nolint:revive` 指令
  - [x] SubTask 4.1: 识别全模块所有 stuttering 类型名（运行 lint 收集 "stutters" 报告）
  - [x] SubTask 4.2: 对每个 stuttering 类型声明行添加 `//nolint:revive` 指令（不重命名，避免破坏 API）。已知典型：`action.ActionExecutor`、`action.ActionResult`、`action.ActionRegistry`、`action.ActionPolicy`、`action.ActionSpec`、`action.ActionCall`、`action.ActionDecision`、`mcp.MCPServerConfig`、`audit.AuditChain`、`audit.AuditEntry`、`audit.AuditConfig`、`audit.AuditHook`、`session.SessionOption`、`pii.PIIType`、`rbac.RBACApprovalAdapter`、`rbac.RBACMiddleware`、`tool.ToolInfo`
  - [x] SubTask 4.3: 验证无未抑制的 stuttering 报错

## 阶段 4：全量验证（依赖阶段 1-3 全部完成）

- [x] Task 5: 全量验证
  - [x] SubTask 5.1: 对 14 个子模块执行 `golangci-lint run --config=<root>/.golangci.yaml ./...`，全部 exit code 0
  - [x] SubTask 5.2: 对 14 个子模块执行 `go test ./...`，确认注释/格式化/nolint 改动未破坏测试（全部 ok）
  - [x] SubTask 5.3: 确认 `implement-p0-maturity-improvements` checklist 的 "`golangci-lint run` 无 error" 项真正达成

# Task Dependencies
- **Task 1（gofmt）** 与 **Task 2（reflect.Pointer）** 无依赖，可并行
- **Task 3（补注释）** 与 **Task 4（nolint）** 互相独立，可并行；建议基于阶段 1 完成后的 lint 输出精准定位
- **Task 5（验证）** 依赖 Task 1-4 全部完成

# Parallelizable Work
- 阶段 1：Task 1 + Task 2 完全独立，可并行
- 阶段 2：Task 3 各 SubTask 按模块互相独立，可按模块并行分配
- 阶段 2 + 阶段 3：Task 3（注释）与 Task 4（nolint）属不同违规类型，可并行
