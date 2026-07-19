# Post-G1 Roadmap Spec

## Why

基于 `plan-20260721-future.md` 第九/十章审计结果，InferGlow 当前 G1 系列仅完成 G1-01（Provider 配置层 100%），G1-02 ~ G1-08 全部未实现。同时有 6 个核心模块完全缺失（`security/`、`builtins/actions/`、`rag/`、`model/pool.go`、`session/memory_plugin.go`、`observability/otel/`），与 LangChain/Agently 等对标框架在安全、可观测性、内置工具方面存在关键差距。

本 Spec 定义 G1 收尾及后续模块建设的实施路径，目标是把 InferGlow 从"单模块完成度高"推进到"全栈能力闭环"。

## What Changes

### 阶段 A：G1 推理能力收尾（G1-02 ~ G1-06）
- **G1-02** `reasoning_content` 字段适配：`model/openai.go` 的 `openAIChunk.Delta` 新增 `ReasoningContent *string`，`processOpenAILine` 优先读取 `reasoning_content`，回退 `reasoning`。影响 Provider：MiMo、Spark、Sensenova。
- **G1-03** 深度思考参数传递：`Options["thinking"]` 与 `Options["reasoning_effort"]` 在 Provider 层透传至请求体（MiMo/Stepfun/Sensenova 的 `thinking` 参数，OpenAI o-series 的 `reasoning_effort`）。
- **G1-04** `<thinking>` tag 归一化：新增 `normalizeThinkingTags()`，将 `</think>` 等非标闭合统一为 `</thinking>`。
- **G1-05** 推理内容预算控制：`Options["reasoning_max_tokens"]` 限制推理 token 上限，超限触发回调或截断。
- **G1-06** 推理 token 单独计费：`model.UsageInfo` 新增 `ReasoningTokens` 字段，`StreamChunk.Usage` 收集时区分推理/补全 token。

### 阶段 B：G1 性能与高可用（G1-07、G1-08）
- **G1-07** 核心路径 Benchmark：为 `model/`、`orchestrator/`、`flow/`、`schema/`、`session/`、`audit/` 添加 `func Benchmark*` 基线测试，建立性能回归基线。
- **G1-08** ModelPool 路由降级：新增 `model/pool.go`，支持 `RoutingPolicy`（Cost/Latency/Quality/Fallback）和 `fallbackChain`，与现有 `AttemptRunner` 重试机制互补。

### 阶段 C：安全模块建设（P0）
- **`security/ratelimit/`** TokenBucket + per-provider 限额（RPM/TPM/日限额），硬限制/软限制双模式。在 `model/provider_factory.go` 创建 Provider 时绑定，在 `Engine.executeLoop` 调用 `RequestModel` 前检查。
- **`security/prompt_injection/`** L1 规则层（关键词/正则）必做，L2 轻量分类可选，L3 LLM-as-Judge 远期。输入侧在 `Session.AddMessage()` 前检测，输出侧在 `Agent.Run()` 返回前检测。
- **`security/pii/`** 正则脱敏（邮箱/电话/身份证/卡号/IP/银行账号），`MaskConfig` 支持 `MaskOnInput`/`MaskOnOutput` 双向配置。
- **`security/rbac/`** Role（viewer/editor/admin/custom）+ PermissionMatrix（action_name → role → allowed），`RBACMiddleware` 在 `ActionDispatcher.Execute` 前检查，与 `sandbox/approval.go` 的 `ApprovalService` 配合（RBAC 决定谁能请求，Approval 决定是否执行）。

### 阶段 D：内置工具包（P1）
- **`builtins/actions/`** 8 个内置 Action：calculator、web_search、file_read、file_write、code_executor、url_fetch、bash_executor、json_processor。每个 Action 按 `ActionSpec.SideEffectLevel`/`ApprovalRequired`/`SandboxRequired` 声明风险等级。
- **`builtins/policies/`** 三套预设权限策略：restrictive（仅 read）、balanced（read + 受限 write）、permissive（全部允许）。
- **`builtins/tools/`** 工具描述生成：从 Go 函数签名生成 `ToolDefinition`，docstring 解析。

### 阶段 E：MCP HTTP/SSE 传输（P0）
- 新增 `action/mcp/transport_http.go`：`HTTPTransport` 实现 `Transport` 接口，SSE 长连接接收 + HTTP POST 发送。补齐 MCP 规范网络层，支持所有通过 HTTP 暴露的 MCP server。

### 阶段 F：可观测性（P0）
- **`observability/otel/`** OpenTelemetry 集成：语义化 Span（`SpanAgentRun`/`SpanLLMCall`/`SpanToolCall`/`SpanFlowExecute`/`SpanPause`/`SpanResume`），语义属性（`llm.model_name`/`llm.provider_name`/`llm.usage.prompt_tokens` 等），导出器（Jaeger/Prometheus）。

### 阶段 G：跨模块端到端测试（P2）
- 新增 `model + orchestrator + action` 串联测试：覆盖 LLM 调用 → Action 调度 → 审计记录完整链路。

### 延后项（不在本 Spec 范围）
- `rag/` 管道（P1）— 依赖向量库选型，单独 Spec 处理
- `session/memory_plugin.go`（P2）— 依赖记忆系统架构设计
- `observability/langfuse/`（P1）— 依赖 OTel 集成完成
- `observability/eval/`（P1）— 依赖评估框架设计
- `observability/abtest/`（P2）— 远期
- Windows 真实 OS 隔离（P3）— 需 Windows 环境
- REST/WebSocket 服务（P1）— 依赖 MCP HTTP/SSE 完成

## Impact

- **Affected specs**: 
  - `adapt-non-standard-providers`（G1-02 reasoning_content 与非标 Provider 相关）
  - `add-examples-and-sandbox-verification`（Benchmark 与已有示例互补）
- **Affected code**:
  - `model/openai.go`、`model/config.go`、`model/provider_factory.go`、`model/pool.go`（新）、`model/result.go`（UsageInfo 扩展）
  - `session/session.go`（PII 输入脱敏钩子）
  - `orchestrator/agent/engine.go`（PII 输出脱敏、Prompt 注入输出检测、RateLimit 检查、OTel Span）
  - `orchestrator/actionruntime/dispatcher.go`（RBAC 拦截器）
  - `action/mcp/transport_http.go`（新）
  - `security/`（新模块）、`builtins/`（新模块）、`observability/`（新模块）
  - 全模块 `*_bench_test.go`（新增 Benchmark）

## ADDED Requirements

### Requirement: G1-02 reasoning_content 字段适配
系统 SHALL 在 OpenAI 兼容协议的流式响应中识别 `reasoning_content` 字段，与现有 `reasoning` 字段等价处理，确保 MiMo、Spark、Sensenova 等 Provider 的推理内容正确捕获。

#### Scenario: MiMo 返回 reasoning_content
- **WHEN** MiMo Provider 流式返回 `{"delta":{"reasoning_content":"思考中..."}}`
- **THEN** `StreamChunk.Reasoning` 字段包含 "思考中..."
- **AND** 不影响现有 `reasoning` 字段的 Provider（如 OpenAI o-series）

### Requirement: G1-03 深度思考参数传递
系统 SHALL 支持 `Options["thinking"]` 和 `Options["reasoning_effort"]` 参数透传至 Provider 请求体，适配 MiMo/Stepfun/Sensenova 的 `thinking` 参数和 OpenAI o-series 的 `reasoning_effort` 参数。

#### Scenario: MiMo thinking 参数
- **WHEN** 调用方设置 `Options["thinking"] = map[string]any{"type":"enabled"}`
- **THEN** MiMo Provider 请求体包含 `"thinking":{"type":"enabled"}`

#### Scenario: OpenAI reasoning_effort 参数
- **WHEN** 调用方设置 `Options["reasoning_effort"] = "high"`
- **THEN** OpenAI Provider 请求体包含 `"reasoning_effort":"high"`

### Requirement: G1-04 thinking tag 归一化
系统 SHALL 将流式输出中的 `</think>`、`<think>` 等非标闭合标签归一化为 `</thinking>`、`<thinking>`，确保下游解析一致。

#### Scenario: 非标 think 标签
- **WHEN** LLM 输出包含 `</think>` 闭合标签
- **THEN** 归一化后输出为 `</thinking>`

### Requirement: G1-05 推理内容预算控制
系统 SHALL 支持 `Options["reasoning_max_tokens"]` 限制推理 token 上限，超限时触发配置的回调或截断推理流。

#### Scenario: 推理超预算截断
- **WHEN** 推理 token 累计超过 `reasoning_max_tokens` 阈值
- **THEN** 截断推理流，正常进入补全阶段

### Requirement: G1-06 推理 token 单独计费
系统 SHALL 在 `model.UsageInfo` 中区分 `ReasoningTokens` 与 `CompletionTokens`，支持按推理 token 单独计费。

#### Scenario: UsageInfo 包含推理 token
- **WHEN** Provider 返回的 usage 中包含推理 token 计数
- **THEN** `UsageInfo.ReasoningTokens` 字段记录该值
- **AND** `UsageInfo.CompletionTokens` 不包含推理 token

### Requirement: G1-07 核心路径 Benchmark
系统 SHALL 为 `model/`、`orchestrator/`、`flow/`、`schema/`、`session/`、`audit/` 模块提供 `func Benchmark*` 基线测试，建立性能回归基线。

#### Scenario: Benchmark 可运行
- **WHEN** 执行 `go test -bench=. -benchmem ./...`
- **THEN** 各模块 Benchmark 测试正常运行并输出性能指标

### Requirement: G1-08 ModelPool 路由降级
系统 SHALL 提供 `model/pool.go` 的 `ModelPool`，支持 `RoutingPolicy`（Cost/Latency/Quality/Fallback）和 `fallbackChain`，在 Provider 故障时自动切换。

#### Scenario: Provider 故障降级
- **WHEN** 主 Provider 连续失败超过阈值
- **THEN** ModelPool 按 `fallbackChain` 切换至备用 Provider
- **AND** 切换事件记录至 audit/

### Requirement: security/ratelimit 速率控制
系统 SHALL 提供 TokenBucket 实现，支持 per-provider 的 RPM/TPM/日限额，硬限制（直接拒绝）和软限制（排队等待）双模式。

#### Scenario: 硬限制超限拒绝
- **WHEN** Provider RPM 超过限额且模式为 `LimitHard`
- **THEN** 请求被拒绝，返回明确的限流错误

### Requirement: security/prompt_injection 注入防护
系统 SHALL 提供 L1 规则层 Prompt 注入检测（关键词 + 正则），在 `Session.AddMessage()` 前检测输入，在 `Agent.Run()` 返回前检测输出。

#### Scenario: 检测已知注入模式
- **WHEN** 用户消息包含 "Ignore previous instructions"
- **THEN** 检测器返回注入告警，按配置等级处理（拒绝/标记/放行）

### Requirement: security/pii PII 脱敏
系统 SHALL 提供正则脱敏能力，支持邮箱、电话、身份证、信用卡、IP、银行账号等 PII 类型，支持输入/输出双向配置。

#### Scenario: 邮箱脱敏
- **WHEN** 输入消息包含 `user@example.com` 且 `MaskOnInput` 启用
- **THEN** 消息进入 Session 前替换为 `***@example.com`（按 `KeepPrefix` 配置）

### Requirement: security/rbac RBAC 访问控制
系统 SHALL 提供 Role（viewer/editor/admin/custom）和 PermissionMatrix（action_name → role → allowed），`RBACMiddleware` 在 `ActionDispatcher.Execute` 前检查权限，默认拒绝。

#### Scenario: editor 角色请求 admin 工具
- **WHEN** role=editor 的用户请求执行 `code_executor`（仅 admin 允许）
- **THEN** RBACMiddleware 拒绝执行，返回权限错误

### Requirement: builtins/actions 内置工具包
系统 SHALL 提供 8 个内置 Action（calculator、web_search、file_read、file_write、code_executor、url_fetch、bash_executor、json_processor），每个 Action 按 `ActionSpec` 声明风险等级和审批/沙箱要求。

#### Scenario: calculator 无审批执行
- **WHEN** 用户调用 `calculator` Action
- **THEN** 直接执行，无需审批（`SideEffectLevel=none`、`ApprovalRequired=false`）

#### Scenario: code_executor 需沙箱+审批
- **WHEN** 用户调用 `code_executor` Action
- **THEN** 必须在沙箱内执行且需审批（`SideEffectLevel=exec`、`ApprovalRequired=true`、`SandboxRequired=true`）

### Requirement: builtins/policies 预设权限策略
系统 SHALL 提供三套预设权限策略：restrictive（仅 read）、balanced（read + 受限 write）、permissive（全部允许）。

#### Scenario: restrictive 策略仅允许 read
- **WHEN** 加载 restrictive 策略
- **THEN** 仅 `SideEffectLevel=read` 或 `none` 的 Action 被注册

### Requirement: builtins/tools 工具描述生成
系统 SHALL 提供从 Go 函数签名生成 `ToolDefinition` 的能力，支持 docstring 解析。

#### Scenario: 从函数生成 ToolDefinition
- **WHEN** 传入 Go 函数 `func Add(a, b int) int` 及其 docstring
- **THEN** 生成包含参数 `a`、`b`（类型 int）和返回值（类型 int）的 `ToolDefinition`

### Requirement: action/mcp HTTP/SSE 传输
系统 SHALL 提供 `action/mcp/transport_http.go` 的 `HTTPTransport`，实现 `Transport` 接口，支持 SSE 长连接接收 + HTTP POST 发送，覆盖所有通过 HTTP 暴露的 MCP server。

#### Scenario: HTTP MCP server 连接
- **WHEN** 配置 MCP server 的 baseURL 为 `http://example.com/mcp`
- **THEN** `HTTPTransport.Start()` 建立 SSE 长连接
- **AND** `Send()` 通过 HTTP POST 发送 JSON-RPC 请求
- **AND** `Recv()` 从 SSE stream 读取响应

### Requirement: observability/otel OpenTelemetry 集成
系统 SHALL 提供语义化 Span（`SpanAgentRun`/`SpanLLMCall`/`SpanToolCall`/`SpanFlowExecute`/`SpanPause`/`SpanResume`）和语义属性（`llm.model_name`/`llm.provider_name`/`llm.usage.prompt_tokens`/`llm.usage.completion_tokens`/`inferglow.session_id`/`inferglow.run_id`/`tool.name`），支持 Jaeger/Prometheus 导出器。

#### Scenario: Agent.Run 创建 Span
- **WHEN** `Agent.Run()` 执行
- **THEN** 创建 `SpanAgentRun` Span，包含 `inferglow.session_id`、`inferglow.run_id` 属性
- **AND** 子操作（LLM 调用、Tool 调用）创建子 Span

### Requirement: 跨模块端到端测试
系统 SHALL 提供 `model + orchestrator + action` 串联测试，覆盖 LLM 调用 → Action 调度 → 审计记录完整链路。

#### Scenario: 端到端链路验证
- **WHEN** 执行跨模块集成测试
- **THEN** mock LLM 返回 action_calls → ActionDispatcher 执行 → audit/ 记录完整链路
- **AND** 测试通过

## MODIFIED Requirements

### Requirement: model.UsageInfo
现有 `model.UsageInfo` 仅包含 `PromptTokens`、`CompletionTokens`、`TotalTokens`。修改后新增 `ReasoningTokens` 字段，区分推理 token 与补全 token，支持 G1-06 单独计费。

### Requirement: model/openai.go processOpenAILine
现有 `processOpenAILine` 仅读取 `c.Delta.Reasoning`。修改后优先读取 `c.Delta.ReasoningContent`，回退至 `c.Delta.Reasoning`，支持 G1-02 `reasoning_content` 字段。

### Requirement: orchestrator/agent/engine.go executeLoop
现有 `executeLoop` 直接调用 `RequestModel`。修改后在调用前插入 RateLimit 检查（阶段 C），在返回前插入 PII 输出脱敏和 Prompt 注入输出检测（阶段 C），并创建 OTel Span（阶段 F）。

### Requirement: orchestrator/actionruntime/dispatcher.go Execute
现有 `Execute` 直接调度 Action。修改后在调度前插入 `RBACMiddleware` 权限检查（阶段 C），并创建 OTel `SpanToolCall`（阶段 F）。

## REMOVED Requirements

无移除项。本 Spec 全部为新增能力，不破坏现有功能。
