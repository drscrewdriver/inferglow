# Checklist

## 阶段 A：G1 推理能力收尾

- [x] G1-02: `model/openai.go` 的 `openAIChunk.Delta` 新增 `ReasoningContent *string` 字段
- [x] G1-02: `processOpenAILine` 优先读取 `reasoning_content`，回退 `reasoning`
- [x] G1-02: MiMo/Spark/Sensenova 的 `reasoning_content` 流式响应解析测试通过
- [x] G1-02: 现有 `reasoning` 字段 Provider 不受影响（回归测试通过）
- [x] G1-03: `Options["thinking"]` 透传至 MiMo/Stepfun/Sensenova 请求体
- [x] G1-03: `Options["reasoning_effort"]` 透传至 OpenAI o-series 请求体
- [x] G1-03: 思考参数透传测试通过
- [x] G1-04: `normalizeThinkingTags()` 函数实现，`<think>`/`</think>` 归一化为 `<thinking>`/`</thinking>`
- [x] G1-04: 流式输出聚合阶段调用归一化函数
- [x] G1-04: 非标标签归一化测试通过
- [x] G1-05: `Options["reasoning_max_tokens"]` 解析逻辑实现（通过 `MaxReasoningTokens` 字段）
- [x] G1-05: 推理 token 超限截断/回调逻辑实现
- [x] G1-05: 预算超限截断测试通过
- [x] G1-06: `model/result.go` 的 `UsageInfo` 新增 `ReasoningTokens` 字段（`CompletionTokensDetails` map）
- [x] G1-06: `processOpenAILine` usage 解析识别 `reasoning_tokens`
- [x] G1-06: `CompletionTokens` 不包含推理 token
- [x] G1-06: `UsageInfo.ReasoningTokens` 解析测试通过

## 阶段 B：G1 性能与高可用

- [x] G1-07: `model/openai_bench_test.go` 添加并运行通过
- [x] G1-07: `orchestrator/engine_bench_test.go` 添加并运行通过
- [x] G1-07: `flow/engine_bench_test.go` 添加并运行通过
- [x] G1-07: `schema/derive_bench_test.go` 验证可运行
- [x] G1-07: `session/three_zone_bench_test.go` 验证可运行
- [x] G1-07: `audit/chain_bench_test.go` 验证可运行
- [x] G1-07: `go test -bench=. -benchmem ./...` 全部 Benchmark 可运行
- [x] G1-08: `model/pool.go` 定义 `ModelPool`、`RoutingPolicy`、`fallbackChain`
- [x] G1-08: `ModelPool.RequestModel()` 按策略选择 Provider，失败时降级
- [x] G1-08: 切换事件接入 `audit/` 记录（通过 auditHook 注入）
- [x] G1-08: `model/pool_test.go` 覆盖正常路由、故障降级、全 Provider 故障
- [x] G1-08: 与 `AttemptRunner` 重试机制整合（互补不冲突）

## 阶段 C：安全模块建设

- [x] RateLimit: `security/ratelimit/bucket.go` 实现 `TokenBucket`
- [x] RateLimit: `security/ratelimit/provider_limiter.go` 实现 per-provider 限额
- [x] RateLimit: `security/ratelimit/policy.go` 定义 `LimitMode`（Hard/Soft）
- [x] RateLimit: `model/provider_factory.go` 创建 Provider 时绑定 RateLimit（通过 wrapper 接口）
- [x] RateLimit: `orchestrator/agent/engine.go` 的 `executeLoop` 调用 `RequestModel` 前检查（通过 hook 注入）
- [x] RateLimit: 硬限制拒绝、软限制排队、多 Provider 独立限额测试通过
- [x] Prompt 注入: `security/prompt_injection/detector.go` 实现 L1 规则层检测
- [x] Prompt 注入: `security/prompt_injection/config.go` 定义检测等级（off/strict/relaxed）
- [x] Prompt 注入: `session/session.go` 的 `AddMessage()` 前插入输入检测
- [x] Prompt 注入: `orchestrator/agent/engine.go` 的 `Run()` 返回前插入输出检测
- [x] Prompt 注入: 已知注入模式、检测等级、输入/输出双侧测试通过
- [x] PII: `security/pii/patterns.go` 定义 `PIIType` 和默认正则模式
- [x] PII: `security/pii/mask.go` 实现 `MaskConfig` 和脱敏函数
- [x] PII: `session/session.go` 的 `AddMessage()` 前插入输入脱敏
- [x] PII: `orchestrator/agent/engine.go` 的 `Run()` 返回前插入输出脱敏
- [x] PII: 各 PII 类型脱敏、KeepPrefix、双向配置测试通过
- [x] RBAC: `security/rbac/policy.go` 定义 `Role`（viewer/editor/admin/custom）
- [x] RBAC: `security/rbac/matrix.go` 实现 `PermissionMatrix`（默认拒绝）
- [x] RBAC: `security/rbac/context.go` 从 `context.Context` 提取角色
- [x] RBAC: `security/rbac/middleware.go` 实现 `RBACMiddleware`
- [x] RBAC: 与 `sandbox/approval.go` 的 `ApprovalService` 整合（通过 `Approver` 接口抽象）
- [x] RBAC: 各角色权限、默认拒绝、与 Approval 整合测试通过

## 阶段 D：内置工具包

- [x] builtins/actions: `calculator.go` 实现且 `ActionSpec` 声明正确（none/false/false）
- [x] builtins/actions: `web_search.go` 实现且 `ActionSpec` 声明正确（read/false/false）
- [x] builtins/actions: `url_fetch.go` 实现且 `ActionSpec` 声明正确（read/false/false）
- [x] builtins/actions: `file_read.go` 实现且 `ActionSpec` 声明正确（read/false/false）
- [x] builtins/actions: `file_write.go` 实现且 `ActionSpec` 声明正确（write/true/false）
- [x] builtins/actions: `code_executor.go` 实现且 `ActionSpec` 声明正确（exec/true/true）
- [x] builtins/actions: `bash_executor.go` 实现且 `ActionSpec` 声明正确（exec/true/true）
- [x] builtins/actions: `json_processor.go` 实现且 `ActionSpec` 声明正确（none/false/false）
- [x] builtins/actions: 每个 Action 对应测试通过
- [x] builtins/policies: `restrictive.go` 仅注册 read/none Action
- [x] builtins/policies: `balanced.go` 注册 read + 受限 write Action
- [x] builtins/policies: `permissive.go` 注册全部 Action
- [x] builtins/policies: 各策略注册的 Action 集合测试通过
- [x] builtins/tools: `schema_from_func.go` 从 Go 函数签名生成 `ToolDefinition`
- [x] builtins/tools: `docstring_parser.go` 解析 docstring 生成描述
- [x] builtins/tools: 函数签名解析、docstring 解析测试通过

## 阶段 E：MCP HTTP/SSE 传输

- [x] `action/mcp/transport_http.go` 定义 `HTTPTransport` 结构体
- [x] `Start(ctx)` 建立 SSE 长连接，启动后台 reader goroutine
- [x] `Send(ctx, msg)` HTTP POST 发送 JSON-RPC 请求
- [x] `Recv(ctx)` 从 SSE stream 读取 events，解析 `data: {...}` 行
- [x] `Stop(ctx)` 关闭 SSE 连接
- [x] `action/mcp/transport_http_test.go` 覆盖握手、工具列表、工具调用、错误处理

## 阶段 F：可观测性

- [x] OTel: `observability/otel/tracer.go` 封装 OTel Tracer，定义 `SpanKind` 枚举
- [x] OTel: 定义语义属性常量（`llm.model_name` 等）
- [x] OTel: `agent_span.go` 在 `Agent.Run()` 创建 `SpanAgentRun` Span
- [x] OTel: `llm_span.go` 在 LLM 调用创建 `SpanLLMCall` Span（含 model_name、usage 属性）
- [x] OTel: `tool_span.go` 在 Tool 调用创建 `SpanToolCall` Span（含 tool.name 属性）
- [x] OTel: `exporters.go` 提供 OTLP(Jaeger)/Prometheus 导出器配置
- [x] OTel: Span 创建、属性设置、导出器配置测试通过

## 阶段 G：跨模块端到端测试

- [x] `examples/cross_module_integration_test.go` 创建，mock LLM 返回 action_calls
- [x] 验证 ActionDispatcher 执行 Action，结果回传 LLM
- [x] 验证 audit/ 记录完整链路（LLM 调用 → Action 调度 → 结果）
- [x] 覆盖正常链路、Action 失败、LLM 失败重试场景
- [x] 跨模块集成测试全部通过
