# Tasks

- [x] Task 1: model ↔ action 操作关系一致性分析
  - [x] SubTask 1.1: 追踪 ToolCall/ToolDefinition 数据流：从 model.ToolDefinition → engine.buildToolDefinitions → Provider SSE → model.ToolCall → SessionExtension.AddAssistantToolCalls → actionruntime.ActionCall → ActionRegistry.Execute → ActionResult → SessionExtension.AddToolResultNamed 的完整链路
  - [x] SubTask 1.2: 对比三种 Provider 的 function call 实现：OpenAI Chat Completions（tool_calls delta 累积）、OpenAI Responses API（response.output_item.added + function_call_arguments.delta/done）、Anthropic Messages API（content_block_start/delta/stop + input_json_delta 累积），列出 SSE 事件类型、ToolCall ID 格式、参数累积方式的差异表
  - [x]SubTask 1.3: 分析 native tool calls 与 legacy JSON decision 双路径并存的风险：executeLoop 中 `len(nativeToolCalls) > 0` 分支 vs `actionruntime.ParseDecision` 分支的代码路径差异、维护成本、潜在不一致
  - [x]SubTask 1.4: 评估 components/tool/adapt_action.go 的桥接完整性：ActionToTool 的 schemaToParams 转换是否丢失 schema 信息、Invoke 的 JSON 解析容错性
  - [x]SubTask 1.5: 检查 SessionExtension.AddAssistantToolCalls / AddToolResultNamed 对三种 Provider ToolCall ID 格式的兼容性

- [x]Task 2: usage 处理链路完整性分析
  - [x]SubTask 2.1: 追踪 UsageInfo 完整传递路径：Provider SSE 解析 → StreamChunk.Usage → BroadcastResponse → ResultEvent(MetaEvent) → ModelResponse.Usage → agent 层消费，标注每个环节的代码位置
  - [x]SubTask 2.2: 分析 executeLoop 跳过 BroadcastResponse 的影响：engine.go 直接消费 `<-chan *StreamChunk` 只读 Delta 和 Tools，UsageInfo/ReasoningDelta/MetaEvent 全部丢失，totalTokens 用字符数近似
  - [x]SubTask 2.3: 分析 failover.go 和 attempt.go 重试场景的 usage 累加逻辑：失败尝试的 usage 是否被记录、重试成功后总 usage 是否包含失败部分
  - [x]SubTask 2.4: 评估 OpenAI Responses API 的 usage 处理：该 Provider 的 processResponsesLine 中未解析 usage 字段，评估影响
  - [x]SubTask 2.5: 对比 eino 的 TokenUsage 标准化设计（PromptTokenDetails.CachedTokens、CompletionTokensDetails.ReasoningTokens）与 InferGlow 的 UsageInfo

- [x]Task 3: session 与回溯日志一致性分析
  - [x]SubTask 3.1: 分析 ThreeZoneSession 三区架构的缓存友好性：Zone 1 ImmutablePrefix 的 SetImmutablePrefix 一次性设置、Zone 2 AppendOnlyHistory 的 append 语义、Zone 3 VolatileScratch 的每轮清空
  - [x]SubTask 3.2: 分析 SmartCompressResizeHandler 压缩策略的推理链保留能力：压缩 tool 结果为 marker、保留 assistant/user 文本的策略评估
  - [x]SubTask 3.3: 评估 session 持久化机制：SaveJSON 只写不读、无自动加载/恢复机制、崩溃后 Zone 3 数据丢失
  - [x]SubTask 3.4: 评估 AuditChain 哈希链的完整性：HMAC 签名、PrevHash 链式、MaxEntries 软上限的丢数据风险
  - [x]SubTask 3.5: 分析 session 与审计日志的一致性：两者独立写入无事务性保证、decision 级审计缺少消息级回放能力、无法从审计日志重建三区状态
  - [x]SubTask 3.6: 对比 eino 的中间件机制（summarization/reduction/patchtoolcalls）与 InferGlow 的 resize 策略链

- [x]Task 4: 沙箱安全边界分析
  - [x]SubTask 4.1: 追踪 SandboxExecutor.Execute 的完整执行链路：input map 解析 → buildPolicyFromInput → ApprovalService 审批 → Manager.CreateHandle → handle.Start/Execute/Stop → ActionResult 映射
  - [x]SubTask 4.2: 分析 buildPolicyFromInput 的安全风险：timeout/network_access/path_allowlist/max_output_bytes 均从 LLM 生成的 input map 读取，LLM 可操控沙箱策略
  - [x]SubTask 4.3: 评估多后端隔离能力：Docker/gVisor/Bubblewrap/Landlock/Seatbelt/E2B 的策略覆盖差异、ModeAuto 选择逻辑的安全性
  - [x]SubTask 4.4: 分析间接注入风险：prompt injection detector 不检查 tool 返回内容、MCP 服务器输出可注入指令到 session
  - [x]SubTask 4.5: 评估 SandboxExecutor 的 approval 机制：ApprovalRequired 布尔值同样来自 input map，可被 LLM 绕过

- [x]Task 5: 防注入与 PII 脱敏覆盖度分析
  - [x]SubTask 5.1: 评估 prompt injection detector 的规则覆盖：7 个关键词 + 8 个正则模式的覆盖面、未覆盖的注入变体（多语言、Unicode 混淆、分段注入、编码绕过）
  - [x]SubTask 5.2: 分析 PII masker 的处理盲区：只对 string 类型 content 执行 MaskInput、[]ContentBlock 多模态内容不脱敏、Meta map 中的 PII 不脱敏
  - [x]SubTask 5.3: 评估 security_hook 的集成完整性：sessionhook 只在 AddMessage 前检查、agent 层 outputHook 只检查最终响应、tool 结果回传时不检查
  - [x]SubTask 5.4: 分析 RBAC 与 approval 模块的集成：access_policy.go 的权限矩阵是否覆盖 action 执行权限、approval_integration.go 的审批流程是否被沙箱执行器正确调用
  - [x]SubTask 5.5: 对比 langchaingo/eino 的安全集成模式（如有）与 InferGlow 的差异

- [x]Task 6: 模块边界与依赖方向分析
  - [x]SubTask 6.1: 分析 components/tool/adapt_action.go 的 ActionToTool 所有调用点，判断是否与 engine.buildToolDefinitions 路径重复，评估是否可删除
  - [x]SubTask 6.2: 通过 go dep 分析或 import 关系扫描，列出 model/action/orchestrator/session/security/components 包间的所有 import 关系，识别跨层调用和潜在循环依赖
  - [x]SubTask 6.3: 评估 orchestrator/agent 包是否过度耦合 model 和 action 的内部细节（如直接引用 model.ToolCall、action.ActionResult）
  - [x]SubTask 6.4: 评估 security 子包（prompt_injection/pii/rbac/agenthook/sessionhook）之间的依赖方向是否合理，是否存在职责重叠
  - [x]SubTask 6.5: 产出可消除的无用代码/桥接层清单，每项标注"删除/合并/保留"及理由

- [x]Task 7: 代码重复与接口抽象分析
  - [x]SubTask 7.1: 对比 openai.go、anthropic.go、openai_responses.go 的 SSE 行解析逻辑，量化重复代码行数，识别可提取的公共逻辑（SSE 行读取、JSON 解析、error 处理、stream goroutine 管理）
  - [x]SubTask 7.2: 对比 resize.go 中 SimpleCutResizeHandler/SummaryFirstResizeHandler/TokenAwareResizeHandler/SmartCompressResizeHandler 的重复逻辑，评估可合并性
  - [x]SubTask 7.3: 分析 ModelRequester 接口（GenerateRequestData + RequestModel + BroadcastResponse）的每个方法的调用者，判断是否所有调用者都需要全部三个方法，给出拆分建议
  - [x]SubTask 7.4: 分析 ActionExecutor 接口是否统一了 local/sandbox/mcp 三种执行器，评估是否有执行器绕过接口直接调用
  - [x]SubTask 7.5: 分析 SessionBackend 接口的抽象层次是否合适，PreparePrompt/AddMessage/SaveJSON 是否应拆分为不同接口

- [x]Task 8: 包结构与目录组织分析
  - [x]SubTask 8.1: 统计各包的文件数和导出符号数，列出文件数 >10 或导出符号数 >30 的包，评估 god package 风险
  - [x]SubTask 8.2: 检查 internal/ 目录的使用情况，列出不应对外暴露但未放入 internal/ 的包
  - [x]SubTask 8.3: 评估目录结构是否反映职责边界（如 model 包是否混杂了 Provider 实现、接口定义、数据结构），给出拆分建议
  - [x]SubTask 8.4: 对比 eino 的包组织方式（components/ + schema/ + compose/ + adk/ 分层）与 InferGlow 的扁平结构

- [x]Task 9: 扩展性评估
  - [x]SubTask 9.1: 追踪新增一个 Model Provider 需要修改的所有文件（新 Provider 文件 + model.go 注册 + 测试 + 可能的 engine.go 适配），计算总接触点数，对比 eino 的扩展成本
  - [x]SubTask 9.2: 追踪新增一个 action 执行器需要修改的所有文件（新 executor + action.go 注册 + actionruntime 适配），计算总接触点数
  - [x]SubTask 9.3: 追踪新增一个安全检查规则需要修改的所有文件（detector.go + config.go + security_hook.go + 测试），计算总接触点数，给出插件化建议
  - [x]SubTask 9.4: 追踪新增一个沙箱后端需要修改的所有文件（新后端 + manager.go 注册 + policy.go 适配），计算总接触点数
  - [x]SubTask 9.5: 汇总扩展性评分矩阵（Provider/action/security/sandbox 四类扩展点，InferGlow vs eino）

- [x]Task 10: 横向对比分析（langchaingo / eino / agently）
  - [x]SubTask 10.1: 接口设计对比：eino 泛型 BaseModel[M] vs InferGlow 非泛型 ModelRequester、eino ToolCallingChatModel.WithTools 不可变性 vs InferGlow buildToolDefinitions 每轮重建
  - [x]SubTask 10.2: tool 集成对比：eino EnhancedInvokableTool 多模态结果 vs InferGlow ActionResult.Result any 类型、eino ToolsNode 编排节点 vs InferGlow ActionRegistry 直接调度
  - [x]SubTask 10.3: session/memory 对比：eino 中间件机制（summarization/reduction）vs InferGlow resize 策略链、eino AgenticMessage vs InferGlow ChatMessage
  - [x]SubTask 10.4: usage 标准化对比：eino TokenUsage 结构化字段 vs InferGlow UsageInfo、eino ResponseMeta 携带方式 vs InferGlow ModelResponse.Usage
  - [x]SubTask 10.5: 安全集成对比：eino internal/safe/panic 恢复 vs InferGlow LoopGuard、eino 无原生安全模块 vs InferGlow prompt_injection/pii/rbac/sandbox 全栈
  - [x]SubTask 10.6: 包结构对比：eino components/+schema/+compose/+adk 分层 vs InferGlow 扁平结构
  - [x]SubTask 10.7: 通过 WebSearch 调研 langchaingo 和 agently 在 model-action 集成、session 管理、安全、包结构方面的设计模式

- [x]Task 11: 生成问题矩阵与架构调整建议总表
  - [x]SubTask 11.1: 汇总所有识别的问题，按 P0（严重安全/数据丢失）/P1（功能缺陷/架构缺陷）/P2（改进建议）分级
  - [x]SubTask 11.2: 为每个问题给出具体改进路径和影响的文件列表
  - [x]SubTask 11.3: 生成差距评分矩阵（InferGlow vs eino vs langchaingo vs agently，10 维度 1-5 分）
  - [x]SubTask 11.4: 生成架构调整建议总表，每个识别的冗余/问题代码标注"删除/合并/拆分/保留"，删除项含替代路径，合并项含合并目标，拆分项含拆分方案

- [x]Task 12: 输出最终审计报告
  - [x]SubTask 12.1: 将所有分析内容整合为 `docs/architecture-consistency-audit.md`
  - [x]SubTask 12.2: 确保报告结构清晰（10 个分析维度 + 问题矩阵 + 架构调整建议总表）、问题可追溯、建议可执行，包含代码引用和行号

# Task Dependencies
- [Task 2] 部分依赖 [Task 1]（usage 链路分析需要理解 model-action 数据流）
- [Task 3] 独立，可与 Task 1/2 并行
- [Task 4] 部分依赖 [Task 5]（沙箱间接注入与防注入覆盖度相关）
- [Task 6] 依赖 [Task 1]（需要理解 model-action 数据流才能评估桥接层冗余）
- [Task 7] 依赖 [Task 1]（需要理解 Provider 实现才能评估重复代码）和 [Task 3]（需要理解 session 接口才能评估抽象层次）
- [Task 8] 依赖 [Task 6]（包结构分析需要模块边界分析结果）
- [Task 9] 依赖 [Task 6]-[Task 8]（扩展性评估需要理解当前模块边界和接口抽象）
- [Task 10] 依赖 [Task 1]-[Task 9]（横向对比需要先完成全部自身分析）
- [Task 11] 依赖 [Task 1]-[Task 10]（汇总所有分析结果）
- [Task 12] 依赖 [Task 11]（整合为最终报告）
- 可并行：Task 1+2+3+4+5 可部分并行；Task 6+7 可在 Task 1 完成后并行；Task 8 在 Task 6 后启动
