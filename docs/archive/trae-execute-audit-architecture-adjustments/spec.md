# 执行架构审计调整 Spec

## Why

架构一致性审计报告（`inferglow/docs/architecture-consistency-audit.md`，680 行）已完成，识别出 45 项问题（13 项 P0 / 19 项 P1 / 13 项 P2），并产出了一张"删除/合并/拆分/保留"架构调整建议总表。当前需要将该审计报告从"分析产出"推进到"代码落地"：先归档 master 作为回滚基线，再在主分支上执行架构调整（删除死代码、修复 P0 安全与数据链路问题、合并重复逻辑、拆分 God Package），最后检查并适配下游 `inferflow` 的依赖。

## What Changes

### 阶段 0：master 备份归档
- 在当前 master HEAD（`80a216f`）打 tag `pre-arch-adjust-20260724` 作为回滚基线
- 推送 tag 到 origin

### 阶段 1：删除死代码（P0-8、P0-13、P2-1）
- **BREAKING** 删除 `components/tool/` 整包（`adapt_action.go` / `adapt_action_test.go` / `interface.go` / `interface_test.go` / `option.go` / `go.mod` / `go.sum`），替代路径为 `engine.buildToolDefinitions`
- **BREAKING** 删除 `model.ActionResult` 死类型（`model.go:121-126`），替代路径为 `action.ActionResult`
- **BREAKING** 删除 `ModelRequest.Actions` 死字段（`model.go:35`）

### 阶段 2：修复 P0 安全声明与实现脱节（P0-3 ~ P0-7）
- 修复 `buildPolicyFromInput` 可被 LLM 操控：引入 deny-by-default 服务端策略基线，LLM 输入只能在基线范围内收紧
- 修复 `approval_required` 可绕过：`approval_required` 改由服务端策略决定，不从 input map 读取
- 修复 Docker/gVisor 后端不强制 `network_access`：根据 `network_access` 设置 `NetworkDisabled`
- 修复 `ModeAuto` 回落到 `trusted_local`：移除无隔离回落，仅显式受信环境允许

### 阶段 3：修复 P0 间接注入（P0-7）
- 在 `AddToolResult` / `AddMessageWithMeta` 接入 prompt injection detector 和 PII masker，覆盖工具结果（含 MCP 输出）

### 阶段 4：修复 P0 数据链路断裂（P0-1、P0-2）
- 修复 Anthropic Provider 多轮 native tool call：在 `PreparePrompt` 中按 Provider 分支转换 `tool_calls` / `tool_call_id` 为 Anthropic content block 格式
- 修复 `SaveJSON` 声称 crash recovery 但无 `LoadJSON`：实现 `LoadJSON` 或修改注释并补充恢复方案

### 阶段 5：合并重复逻辑（D7-4、P1-20）
- 合并两套审批系统：`sandbox.ApprovalService` 合并到 `approval.PolicyApprovalManager`，定义统一接口
- 提取 Provider SSE 公共逻辑到 `model/providers/internal/ssestream` 公共包（`effectiveHTTPClient` / `RequestModel` goroutine 骨架 / `processLine` / `emit` 闭包 / EOF+error 处理 / `BroadcastResponse` 骨架 / `mapRole`，约 250-300 行重复）
- 提取 resize handlers 总字节计算循环为 `TotalContentBytes` 辅助函数

### 阶段 6：拆分 God Package（P0-9、P0-10、P0-11、P0-12）
- 拆分 `model/` God Package（25 文件 / ~117 导出符号）：`model/schema.go` 保留核心类型 + `model/internal/`（attempt / failover / pool）+ `model/providers/openai/` + `model/providers/anthropic/`
- 拆分 `orchestrator/agent/` God Package（15 文件 / 8 类职责）：`agent.go` + `engine.go` + `internal/turnloop` + `internal/step` + `internal/strategy` + `hooks/` + `streaming/` + `extension/` + `features/`
- 引入 `internal/` 目录收敛 API 表面，将不应对外暴露的辅助包移入 `internal/`
- 迁移核心数据类型 `ModelRequest` / `ModelResponse` 至 `schema/` 包，与 Provider 实现解耦

### 阶段 7：拆分过宽接口（P1-21、P1-22）
- 拆分 `ModelRequester` 接口为 `StreamRequester`（核心流式请求）+ `ResponseBroadcaster`（多消费者广播）
- 拆分 `SessionBackend` 接口为 `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore`

### 阶段 8：inferflow 依赖适配
- 检查 `inferflow` 对 inferglow 的所有直接依赖（`model` / `orchestrator/agent` / `flow` / `session` / `action` / `builtins/actions`）在架构调整后的兼容性
- 适配 `model.ModelRequester` 接口拆分：inferflow 的 stub 实现需拆分为 `StreamRequester` + `ResponseBroadcaster`
- 适配核心类型迁移：`model.ModelRequest` / `model.ChatMessage` 等若迁移到 `schema/` 包，更新 inferflow 的 import 路径
- 适配 `orchestrator/agent` 包拆分：更新 inferflow 对 `agent.New` / `agent.NewEngine` / `agent.NewSessionExtension` / `agent.NewActionExtension` 的 import
- 确保 `inferflow` 的 `go build ./...` 和 `go test ./...` 全部通过

## Impact

- **Affected specs**:
  - `architecture-consistency-audit`（本 Spec 落地其审计结论）
  - `implement-p0-maturity-improvements`（部分 P0 修复有交叉，需协调）
  - `post-g1-roadmap`（包结构治理影响后续路线图）
- **Affected code**:
  - 删除：`components/tool/*` 整包
  - 修改：`model/model.go`（删除 ActionResult / Actions 字段）、`model/anthropic.go`、`model/openai.go`、`model/openai_responses.go`、`model/chat.go`
  - 修改：`sandbox/executor_sandbox.go`、`sandbox/docker.go`、`sandbox/gvisor.go`、`sandbox/manager.go`、`sandbox/bubblewrap.go`
  - 修改：`session/session.go`、`session/session_ext.go`、`session/three_zone.go`、`session/resize.go`
  - 修改：`orchestrator/agent/engine.go`、`orchestrator/agent/agent.go`
  - 修改：`approval/*`、`sandbox/approval*.go`
  - 新增：`model/providers/internal/ssestream/`、`model/internal/`、`model/providers/openai/`、`model/providers/anthropic/`
  - 新增：`orchestrator/agent/internal/`、`orchestrator/agent/hooks/`、`orchestrator/agent/streaming/`
  - 修改：`schema/`（接收迁移的核心类型）
  - 修改：`inferflow/runtime/integration/model_provider.go`、`inferflow/runtime/engine/engine.go`、`inferflow/runtime/nl2flow/converter.go`、`inferflow/cmd/inferflowd/wire.go` 及所有 stub 测试
- **BREAKING** 变更：删除 `components/tool` 包、删除 `model.ActionResult` 类型、拆分 `ModelRequester` 接口、拆分 `SessionBackend` 接口、包路径重组

## ADDED Requirements

### Requirement: master 备份归档
系统 SHALL 在执行任何架构调整前，在当前 master HEAD 打 tag `pre-arch-adjust-20260724` 并推送到 origin，作为回滚基线。

#### Scenario: 打 tag 备份
- **WHEN** 执行阶段 0
- **THEN** 在当前 master HEAD（`80a216f`）创建 annotated tag `pre-arch-adjust-20260724`
- **AND** tag 推送到 origin
- **AND** tag 包含审计报告路径和回滚说明

### Requirement: 删除 components/tool 死代码包
系统 SHALL 删除 `components/tool/` 整包，包括 `adapt_action.go`、`adapt_action_test.go`、`interface.go`、`interface_test.go`、`option.go`、`go.mod`、`go.sum`，因 `ActionToTool` 零生产调用，替代路径为 `engine.buildToolDefinitions`。

#### Scenario: 删除后构建通过
- **WHEN** 删除 `components/tool/` 整包
- **THEN** `go build ./...` 在 inferglow 根模块通过
- **AND** 无任何包引用 `github.com/inferglow/components/tool`

### Requirement: 删除 model.ActionResult 死类型
系统 SHALL 删除 `model.ActionResult`（`model.go:121-126`）死类型和 `ModelRequest.Actions` 死字段（`model.go:35`），替代路径为 `action.ActionResult`。

#### Scenario: 死类型清除
- **WHEN** 删除 `model.ActionResult` 和 `ModelRequest.Actions`
- **THEN** `go build ./...` 通过
- **AND** 无代码引用 `model.ActionResult` 或 `ModelRequest.Actions`

### Requirement: 沙箱策略 deny-by-default 基线
系统 SHALL 在 `buildPolicyFromInput` 中引入 deny-by-default 服务端策略基线，LLM 生成的 input map 参数只能在基线范围内收紧，不能放宽。

#### Scenario: LLM 无法放宽策略
- **WHEN** LLM 生成的 tool call 参数包含 `network_access: "full"`
- **AND** 服务端策略基线为 `network_access: "none"`
- **THEN** 实际执行策略为 `network_access: "none"`（基线优先）
- **AND** 记录策略收紧日志

#### Scenario: LLM 可以收紧策略
- **WHEN** 服务端策略基线为 `network_access: "egress_only"`
- **AND** LLM 参数为 `network_access: "none"`
- **THEN** 实际执行策略为 `network_access: "none"`（取更严格值）

### Requirement: approval_required 服务端决定
系统 SHALL 将 `approval_required` 改由服务端策略决定，不从 LLM 生成的 input map 读取，防止 LLM 绕过审批。

#### Scenario: LLM 无法绕过审批
- **WHEN** LLM 生成的 tool call 参数包含 `approval_required: false`
- **AND** 服务端策略要求该 action 必须审批
- **THEN** 仍触发审批流程

### Requirement: Docker/gVisor 网络策略强制执行
系统 SHALL 在 Docker 和 gVisor 后端根据 `network_access` 策略实际设置网络隔离，`NetworkDisabled` 对应 `network_access: "none"`。

#### Scenario: network_access none 时禁用网络
- **WHEN** 策略 `network_access` 为 `"none"`
- **THEN** Docker 后端创建容器时 `NetworkDisabled: true`
- **AND** gVisor 后端配置禁用网络

### Requirement: ModeAuto 不回落到无隔离
系统 SHALL 移除 `ModeAuto` 回落到 `trusted_local` 的逻辑，仅显式受信环境（通过配置开关）允许回落。

#### Scenario: ModeAuto 不自动回落
- **WHEN** `ModeAuto` 无法找到合适后端
- **AND** 未显式配置允许受信回落
- **THEN** 返回错误而非回落到 `trusted_local`

### Requirement: 工具结果接入防注入检测
系统 SHALL 在 `AddToolResult` 和 `AddMessageWithMeta` 接入 prompt injection detector 和 PII masker，覆盖工具返回内容（含 MCP 输出），防止间接注入。

#### Scenario: 工具结果被检测
- **WHEN** 工具返回内容包含注入模式（如 "Ignore previous instructions"）
- **THEN** prompt injection detector 触发告警
- **AND** 按配置等级处理（拒绝/标记/放行）

#### Scenario: MCP 输出被脱敏
- **WHEN** MCP 服务器返回内容包含 PII（如邮箱）
- **THEN** PII masker 执行脱敏后再写入 session

### Requirement: Anthropic 多轮 native tool call
系统 SHALL 在 `PreparePrompt` 中按 Provider 分支转换 `tool_calls` / `tool_call_id` 为 Anthropic content block 格式，确保多轮 native tool call 正确工作。

#### Scenario: Anthropic 第二轮请求格式正确
- **WHEN** Anthropic Provider 发送第二轮请求
- **AND** session 中存在 `tool_calls` / `tool_call_id` 格式的历史消息
- **THEN** 转换为 Anthropic 的 `tool_use` / `tool_result` content block 格式
- **AND** Anthropic API 接受请求不报错

### Requirement: ThreeZoneSession LoadJSON 恢复
系统 SHALL 实现 `LoadJSON` 方法支持从持久化文件恢复 session 状态，或修改 `SaveJSON` 注释移除 crash recovery 声明并补充替代方案。

#### Scenario: 从 SaveJSON 文件恢复
- **WHEN** 调用 `LoadJSON(path)` 传入 `SaveJSON` 写入的文件
- **THEN** ThreeZoneSession 三区状态从文件恢复
- **AND** Zone 1 / Zone 2 / Zone 3 数据完整

### Requirement: 审批系统统一
系统 SHALL 合并 `sandbox.ApprovalService` 和 `approval.PolicyApprovalManager` 为统一审批接口，消除两套割裂系统。

#### Scenario: 统一审批入口
- **WHEN** SandboxExecutor 需要审批
- **THEN** 调用统一的 `approval.PolicyApprovalManager`
- **AND** 不再存在 `sandbox.ApprovalService` 独立实现

### Requirement: Provider SSE 公共逻辑提取
系统 SHALL 将三个 Provider 的 SSE 解析公共逻辑提取到 `model/providers/internal/ssestream` 包，消除约 250-300 行重复代码。

#### Scenario: Provider 复用 SSE 公共包
- **WHEN** OpenAI / Anthropic / OpenAI Responses Provider 实现 SSE 解析
- **THEN** 复用 `ssestream` 包的 `effectiveHTTPClient` / `RequestModel` goroutine 骨架 / `processLine` / `emit` 闭包 / EOF+error 处理 / `BroadcastResponse` 骨架 / `mapRole`
- **AND** 各 Provider 仅保留自身特有逻辑

### Requirement: resize TotalContentBytes 辅助函数
系统 SHALL 提取 resize handlers 中重复的总字节计算循环为 `TotalContentBytes` 辅助函数。

#### Scenario: resize handlers 复用辅助函数
- **WHEN** `SimpleCutResizeHandler` / `SummaryFirstResizeHandler` / `TokenAwareResizeHandler` / `SmartCompressResizeHandler` 计算总字节
- **THEN** 调用 `TotalContentBytes` 辅助函数
- **AND** 不再重复循环逻辑

### Requirement: model/ God Package 拆分
系统 SHALL 拆分 `model/` God Package 为 `model/schema.go`（核心类型）+ `model/internal/`（attempt / failover / pool）+ `model/providers/openai/` + `model/providers/anthropic/`，降低单包职责混杂度。

#### Scenario: model 包拆分后结构清晰
- **WHEN** 检查 `model/` 目录结构
- **THEN** 核心类型集中在 `schema.go`
- **AND** 内部辅助逻辑在 `internal/` 不对外暴露
- **AND** Provider 实现按厂商分子包

### Requirement: orchestrator/agent/ God Package 拆分
系统 SHALL 拆分 `orchestrator/agent/` God Package 为 `agent.go` + `engine.go` + `internal/turnloop` + `internal/step` + `internal/strategy` + `hooks/` + `streaming/` + `extension/` + `features/`，分离 8 类正交职责。

#### Scenario: agent 包拆分后职责分离
- **WHEN** 检查 `orchestrator/agent/` 目录结构
- **THEN** turnloop / step / strategy 在 `internal/` 下
- **AND** hooks / streaming / extension / features 为独立子包
- **AND** 公共 API 表面收敛

### Requirement: internal/ 目录隔离
系统 SHALL 引入 `internal/` 目录收敛 API 表面，将不应对外暴露的辅助包移入 `internal/`，防止外部模块依赖内部实现细节。

#### Scenario: 内部包不对外暴露
- **WHEN** 外部模块（如 inferflow）尝试 import inferglow 的 `internal/` 包
- **THEN** Go 编译器拒绝编译
- **AND** 仅公共 API 可被外部引用

### Requirement: 核心数据类型迁移至 schema/
系统 SHALL 迁移 `ModelRequest` / `ModelResponse` 等核心数据类型至 `schema/` 包，与 Provider 实现解耦。

#### Scenario: 核心类型在 schema 包
- **WHEN** 查找 `ModelRequest` / `ModelResponse` 类型定义
- **THEN** 位于 `schema/` 包
- **AND** `model/` 包通过 re-export 或直接引用 `schema/` 保持向后兼容

### Requirement: ModelRequester 接口拆分
系统 SHALL 将 `ModelRequester` 接口拆分为 `StreamRequester`（`GenerateRequestData` + `RequestModel`）和 `ResponseBroadcaster`（`BroadcastResponse`），因 `BroadcastResponse` 在 agent 生产路径未使用。

#### Scenario: 接口拆分后调用方按需实现
- **WHEN** 调用方仅需流式请求
- **THEN** 实现 `StreamRequester` 接口即可
- **AND** 不强制实现 `BroadcastResponse`

### Requirement: SessionBackend 接口拆分
系统 SHALL 将 `SessionBackend` 接口拆分为 `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore`，消除 3 处类型断言访问接口外方法。

#### Scenario: 拆分后无类型断言
- **WHEN** 检查 session 包代码
- **THEN** 不存在类型断言访问 `SessionBackend` 接口外方法
- **AND** 各拆分接口职责单一

### Requirement: inferflow 依赖适配
系统 SHALL 在架构调整完成后检查并适配 `inferflow` 的所有 inferglow 依赖，确保 `go build ./...` 和 `go test ./...` 全部通过。

#### Scenario: inferflow 构建通过
- **WHEN** 在 `inferflow/` 目录执行 `go build ./...`
- **THEN** 编译成功无错误
- **AND** 所有 import 路径已更新

#### Scenario: inferflow 测试通过
- **WHEN** 在 `inferflow/` 目录执行 `go test ./...`
- **THEN** 所有测试通过
- **AND** stub ModelRequester 实现适配拆分后的接口

## MODIFIED Requirements

### Requirement: sandbox/executor_sandbox.go buildPolicyFromInput
现有 `buildPolicyFromInput` 直接从 LLM 生成的 input map 读取所有策略字段。修改后引入 deny-by-default 服务端策略基线，LLM 参数只能在基线范围内收紧。

### Requirement: sandbox/manager.go ModeAuto
现有 `ModeAuto` 可回落到 `trusted_local`（无隔离）。修改后移除自动回落，仅显式配置允许。

### Requirement: session/session_ext.go AddToolResult
现有 `AddToolResult` 不经过安全检测。修改后接入 prompt injection detector 和 PII masker。

### Requirement: model/anthropic.go PreparePrompt
现有 `PreparePrompt` 对所有 Provider 产生 OpenAI 格式的 `tool_calls` / `tool_call_id`。修改后按 Provider 分支转换格式。

### Requirement: session/three_zone.go SaveJSON
现有 `SaveJSON` 只写不读，声称 crash recovery 但无 `LoadJSON`。修改后实现 `LoadJSON` 或移除声明。

## REMOVED Requirements

### Requirement: components/tool 包
**Reason**: `ActionToTool` 零生产调用，整包为死代码（P0-8）
**Migration**: 使用 `engine.buildToolDefinitions`（`engine.go:730-758`）替代

### Requirement: model.ActionResult 死类型
**Reason**: 与 `action.ActionResult` 字段完全不同，在实际流程中从未使用（P0-13 / P2-1）
**Migration**: 使用 `action.ActionResult`

### Requirement: ModelRequest.Actions 死字段
**Reason**: 引用死类型 `model.ActionResult`，从未被填充（P0-13）
**Migration**: 无需替代，直接删除
