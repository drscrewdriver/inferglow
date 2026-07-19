# Tasks

## 阶段 A：G1 推理能力收尾（G1-02 ~ G1-06）

- [x] **Task A1: G1-02 reasoning_content 字段适配**（核心实现已就位，测试已补齐）
  - [x] A1.1: 在 `model/openai.go` 的 `openAIChunk.Delta` 结构体新增 `ReasoningContent *string` 字段（json tag `reasoning_content,omitempty`）
  - [x] A1.2: 修改 `processOpenAILine`，优先读取 `c.Delta.ReasoningContent`，为空时回退至 `c.Delta.Reasoning`
  - [x] A1.3: 新增 `openai_reasoning_content_test.go`，覆盖 MiMo/Spark/Sensenova 的 `reasoning_content` 流式响应解析
  - [x] A1.4: 验证现有 `reasoning` 字段 Provider（如 OpenAI o-series）不受影响

- [x] **Task A2: G1-03 深度思考参数传递**（Options 通用透传机制已实现，正确性测试已补齐）
  - [x] A2.1: 在 `model/openai.go` 的请求体构造逻辑中，识别 `Options["thinking"]`，按 Provider 透传至请求体（MiMo/Stepfun/Sensenova）
  - [x] A2.2: 识别 `Options["reasoning_effort"]`，透传至 OpenAI o-series 请求体
  - [x] A2.3: 新增测试覆盖 `thinking` 和 `reasoning_effort` 参数透传
  - [x] A2.4: 在 `model/config.go` 文档中补充 `Options` 支持的思考参数键

- [x] **Task A3: G1-04 thinking tag 归一化**（`normalizeThinkingTags`/`hasThinkingTags` 已实现，BroadcastResponse 聚合阶段调用）
  - [x] A3.1: 在 `model/` 新增 `normalizeThinkingTags()` 函数，将 `<think>`/`</think>` 归一化为 `<thinking>`/`</thinking>`
  - [x] A3.2: 在流式输出聚合阶段调用归一化函数
  - [x] A3.3: 新增测试覆盖非标标签归一化场景

- [x] **Task A4: G1-05 推理内容预算控制**（通过 `MaxReasoningTokens` 结构体字段实现 per-provider 预算控制）
  - [x] A4.1: 在 `model/` 新增 `Options["reasoning_max_tokens"]` 解析逻辑（架构调整为 `MaxReasoningTokens` 字段）
  - [x] A4.2: 在流式接收阶段累计推理 token，超限时触发截断或回调
  - [x] A4.3: 新增测试覆盖预算超限截断场景

- [x] **Task A5: G1-06 推理 token 单独计费**（`CompletionTokensDetails` map + `ReasoningTokens()` 方法）
  - [x] A5.1: 在 `model/result.go` 的 `UsageInfo` 结构体新增 `ReasoningTokens int` 字段（采用 `CompletionTokensDetails` map 设计）
  - [x] A5.2: 在 `processOpenAILine` 的 usage 解析中，识别 `reasoning_tokens` 字段（如 Provider 返回）
  - [x] A5.3: 确保 `CompletionTokens` 不包含推理 token
  - [x] A5.4: 新增测试覆盖 `UsageInfo.ReasoningTokens` 解析

## 阶段 B：G1 性能与高可用（G1-07、G1-08）

- [x] **Task B1: G1-07 核心路径 Benchmark**
  - [x] B1.1: 为 `model/` 添加 `openai_bench_test.go`（覆盖请求构造、流式解析、usage 解析）
  - [x] B1.2: 为 `orchestrator/` 添加 `engine_bench_test.go`（覆盖 executeLoop 单轮迭代）
  - [x] B1.3: 为 `flow/` 添加 `engine_bench_test.go`（覆盖 Flow 执行）
  - [x] B1.4: 为 `schema/` 添加 `derive_bench_test.go`（已有，验证可运行）
  - [x] B1.5: 为 `session/` 添加 `three_zone_bench_test.go`（已有，验证可运行）
  - [x] B1.6: 为 `audit/` 添加 `chain_bench_test.go`（已有，验证可运行）
  - [x] B1.7: 执行 `go test -bench=. -benchmem ./...` 确认全部 Benchmark 可运行

- [x] **Task B2: G1-08 ModelPool 路由降级**（Option 模式 + 4 种 RoutingPolicy + 42 个测试）
  - [x] B2.1: 新建 `model/pool.go`，定义 `ModelPool` 结构体、`RoutingPolicy` 枚举（Cost/Latency/Quality/Fallback）、`fallbackChain`
  - [x] B2.2: 实现 `ModelPool.RequestModel()`，按策略选择 Provider，失败时按 `fallbackChain` 切换
  - [x] B2.3: 切换事件接入 `audit/` 记录（通过 auditHook 注入，不直接依赖 audit/）
  - [x] B2.4: 新建 `model/pool_test.go`，覆盖正常路由、故障降级、全部 Provider 故障场景
  - [x] B2.5: 与现有 `AttemptRunner` 重试机制整合（互补不冲突）

## 阶段 C：安全模块建设（P0）

- [x] **Task C1: security/ratelimit 速率控制**（TokenBucket + ProviderLimiter + Hard/Soft Policy + 测试通过）
  - [x] C1.1: 新建 `security/ratelimit/bucket.go`，实现 `TokenBucket`（RPM/TPM/日限额）
  - [x] C1.2: 新建 `security/ratelimit/provider_limiter.go`，实现 per-provider 独立限额
  - [x] C1.3: 新建 `security/ratelimit/policy.go`，定义 `LimitMode`（LimitHard/LimitSoft）
  - [x] C1.4: 在 `model/provider_factory.go` 创建 Provider 时绑定 RateLimit（通过 wrapper 接口避免循环依赖）
  - [x] C1.5: 在 `orchestrator/agent/engine.go` 的 `executeLoop` 调用 `RequestModel` 前检查（通过 hook 注入）
  - [x] C1.6: 新建 `security/ratelimit/*_test.go`，覆盖硬限制拒绝、软限制排队、多 Provider 独立限额

- [x] **Task C2: security/prompt_injection 注入防护**（L1 规则层检测器 + 输入/输出双侧 hook 整合）
  - [x] C2.1: 新建 `security/prompt_injection/detector.go`，实现 L1 规则层检测（关键词 + 正则）
  - [x] C2.2: 新建 `security/prompt_injection/config.go`，定义检测等级（off/strict/relaxed）
  - [x] C2.3: 在 `session/session.go` 的 `AddMessage()` 前插入输入检测钩子
  - [x] C2.4: 在 `orchestrator/agent/engine.go` 的 `Run()` 返回前插入输出检测
  - [x] C2.5: 新建 `security/prompt_injection/*_test.go`，覆盖已知注入模式、检测等级、输入/输出双侧

- [x] **Task C3: security/pii PII 脱敏**（6 种 PII 类型 + 输入/输出双向脱敏 + MessageMasker 接口）
  - [x] C3.1: 新建 `security/pii/patterns.go`，定义 `PIIType` 枚举和默认正则模式（邮箱/电话/身份证/信用卡/IP/银行账号）
  - [x] C3.2: 新建 `security/pii/mask.go`，实现 `MaskConfig` 和脱敏函数（支持 `MaskChar`/`KeepPrefix`/`MaskOnInput`/`MaskOnOutput`）
  - [x] C3.3: 在 `session/session.go` 的 `AddMessage()` 前插入输入脱敏（按 `MaskOnInput` 配置）
  - [x] C3.4: 在 `orchestrator/agent/engine.go` 的 `Run()` 返回前插入输出脱敏（按 `MaskOnOutput` 配置）
  - [x] C3.5: 新建 `security/pii/*_test.go`，覆盖各 PII 类型脱敏、KeepPrefix、双向配置

- [x] **Task C4: security/rbac RBAC 访问控制**（4 角色 + PermissionMatrix + RBACApprovalAdapter + 44 个测试）
  - [x] C4.1: 新建 `security/rbac/policy.go`，定义 `Role`（viewer/editor/admin/custom）
  - [x] C4.2: 新建 `security/rbac/matrix.go`，实现 `PermissionMatrix`（action_name → role → allowed，默认拒绝）
  - [x] C4.3: 新建 `security/rbac/context.go`，从 `context.Context` 提取用户角色
  - [x] C4.4: 新建 `security/rbac/middleware.go`，实现 `RBACMiddleware`，在 `ActionDispatcher.Execute` 前检查
  - [x] C4.5: 与 `sandbox/approval.go` 的 `ApprovalService` 整合（通过 `Approver` 接口抽象避免循环依赖）
  - [x] C4.6: 新建 `security/rbac/*_test.go`，覆盖各角色权限、默认拒绝、与 Approval 整合

## 阶段 D：内置工具包（P1）

- [x] **Task D1: builtins/actions 内置 Action 实现**（8 个 Action + 77 个测试用例 + SideEffectExec 常量补齐）
  - [x] D1.1: 新建 `builtins/actions/calculator.go`（`SideEffectLevel=none`、`ApprovalRequired=false`、`SandboxRequired=false`）
  - [x] D1.2: 新建 `builtins/actions/web_search.go`（`SideEffectLevel=read`、`ApprovalRequired=false`）
  - [x] D1.3: 新建 `builtins/actions/url_fetch.go`（`SideEffectLevel=read`、`ApprovalRequired=false`）
  - [x] D1.4: 新建 `builtins/actions/file_read.go`（`SideEffectLevel=read`、`ApprovalRequired=false`）
  - [x] D1.5: 新建 `builtins/actions/file_write.go`（`SideEffectLevel=write`、`ApprovalRequired=true`）
  - [x] D1.6: 新建 `builtins/actions/code_executor.go`（`SideEffectLevel=exec`、`ApprovalRequired=true`、`SandboxRequired=true`）
  - [x] D1.7: 新建 `builtins/actions/bash_executor.go`（`SideEffectLevel=exec`、`ApprovalRequired=true`、`SandboxRequired=true`）
  - [x] D1.8: 新建 `builtins/actions/json_processor.go`（`SideEffectLevel=none`、`ApprovalRequired=false`）
  - [x] D1.9: 每个 Action 新建对应 `*_test.go`，覆盖正常执行、错误处理、ActionSpec 声明

- [x] **Task D2: builtins/policies 预设权限策略**（restrictive 5 个 / balanced 6 个 / permissive 8 个 + 7 个测试）
  - [x] D2.1: 新建 `builtins/policies/restrictive.go`（仅注册 `SideEffectLevel=read`/`none` 的 Action）
  - [x] D2.2: 新建 `builtins/policies/balanced.go`（注册 read + 受限 write 的 Action）
  - [x] D2.3: 新建 `builtins/policies/permissive.go`（注册全部 Action）
  - [x] D2.4: 新建 `builtins/policies/*_test.go`，覆盖各策略下注册的 Action 集合

- [x] **Task D3: builtins/tools 工具描述生成**（reflect 签名解析 + docstring 解析 + 31 个测试）
  - [x] D3.1: 新建 `builtins/tools/schema_from_func.go`，从 Go 函数签名生成 `ToolDefinition`
  - [x] D3.2: 新建 `builtins/tools/docstring_parser.go`，解析 docstring 生成描述
  - [x] D3.3: 新建 `builtins/tools/*_test.go`，覆盖函数签名解析、docstring 解析

## 阶段 E：MCP HTTP/SSE 传输（P0）

- [x] **Task E1: action/mcp/transport_http.go 实现**（SSE 长连接 + HTTP POST + 13 个测试）
  - [x] E1.1: 新建 `action/mcp/transport_http.go`，定义 `HTTPTransport` 结构体（`baseURL`、`httpClient`、`reader`）
  - [x] E1.2: 实现 `Start(ctx)`：建立 SSE 长连接，启动后台 reader goroutine
  - [x] E1.3: 实现 `Send(ctx, msg)`：HTTP POST 发送 JSON-RPC 请求
  - [x] E1.4: 实现 `Recv(ctx)`：从 SSE stream 读取 events，解析 `data: {...}` 行
  - [x] E1.5: 实现 `Stop(ctx)`：关闭 SSE 连接
  - [x] E1.6: 新建 `action/mcp/transport_http_test.go`，覆盖握手、工具列表、工具调用、错误处理

## 阶段 F：可观测性（P0）

- [x] **Task F1: observability/otel OpenTelemetry 集成**（Tracer + 6 SpanKind + 语义属性 + OTLP/Prometheus 导出器 + 17 个测试）
  - [x] F1.1: 新建 `observability/otel/tracer.go`，封装 OTel Tracer，定义 `SpanKind` 枚举（SpanAgentRun/SpanLLMCall/SpanToolCall/SpanFlowExecute/SpanPause/SpanResume）
  - [x] F1.2: 定义语义属性常量（`llm.model_name`/`llm.provider_name`/`llm.usage.prompt_tokens`/`llm.usage.completion_tokens`/`inferglow.session_id`/`inferglow.run_id`/`tool.name`）
  - [x] F1.3: 新建 `observability/otel/agent_span.go`，在 `Agent.Run()` 创建 `SpanAgentRun` Span
  - [x] F1.4: 新建 `observability/otel/llm_span.go`，在 LLM 调用创建 `SpanLLMCall` Span（含 model_name、usage 属性）
  - [x] F1.5: 新建 `observability/otel/tool_span.go`，在 Tool 调用创建 `SpanToolCall` Span（含 tool.name 属性）
  - [x] F1.6: 新建 `observability/otel/exporters.go`，提供 OTLP(Jaeger)/Prometheus 导出器配置
  - [x] F1.7: 新建 `observability/otel/*_test.go`，覆盖 Span 创建、属性设置、导出器配置

## 阶段 G：跨模块端到端测试（P2）

- [x] **Task G1: model + orchestrator + action 串联测试**（4 个跨模块集成测试全部通过）
  - [x] G1.1: 新建 `examples/cross_module_integration_test.go`，mock LLM 返回 action_calls
  - [x] G1.2: 验证 ActionDispatcher 执行 Action，结果回传 LLM
  - [x] G1.3: 验证 audit/ 记录完整链路（LLM 调用 → Action 调度 → 结果）
  - [x] G1.4: 覆盖正常链路、Action 失败、LLM 失败重试场景

# Task Dependencies

- **A1（reasoning_content）** 无依赖，可独立开始
- **A2（思考参数）** 无依赖，可与 A1 并行
- **A3（thinking tag）** 依赖 A1（推理内容解析完成后归一化）
- **A4（推理预算）** 依赖 A1、A5（需要 reasoning_content 和 ReasoningTokens 字段）
- **A5（推理计费）** 依赖 A1（需要正确解析推理内容）
- **B1（Benchmark）** 无依赖，可与阶段 A 并行
- **B2（ModelPool）** 依赖 A1-A5 完成（推理能力稳定后再做高可用）
- **C1（RateLimit）** 依赖 B2（与 ModelPool 整合）
- **C2（Prompt 注入）** 无依赖，可与阶段 A/B 并行
- **C3（PII）** 无依赖，可与阶段 A/B 并行
- **C4（RBAC）** 依赖 D1（builtins/actions 完成后，RBAC 矩阵有实际 Action 可配置）
- **D1（内置 Action）** 无依赖，可与阶段 A/B/C 并行
- **D2（预设策略）** 依赖 D1（策略基于 Action 集合）
- **D3（工具描述生成）** 依赖 D1（生成 ToolDefinition）
- **E1（MCP HTTP/SSE）** 无依赖，可与阶段 A/B/C/D 并行
- **F1（OTel）** 依赖 B2（ModelPool 完成后，Span 覆盖路由降级场景）
- **G1（跨模块测试）** 依赖 A1-A5、B2、D1（核心能力完成后做端到端验证）

# Parallelizable Work

以下任务组可并行执行：
1. **阶段 A 内部**：A1 + A2 可并行；A3、A4、A5 串行（依赖关系）
2. **阶段 B 与阶段 A**：B1（Benchmark）与阶段 A 并行
3. **阶段 C 内部**：C2（Prompt 注入）+ C3（PII）+ C4（RBAC，待 D1 完成后）可并行
4. **阶段 D 与阶段 A/B/C**：D1（内置 Action）可与阶段 A/B/C 并行
5. **阶段 E 与其他**：E1（MCP HTTP/SSE）独立，可全程并行
