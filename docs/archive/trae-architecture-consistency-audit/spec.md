# 架构一致性审计与横向评估 Spec

## Why

InferGlow 的 model 模块历经长期迭代，OpenAI 模式（Chat Completions + Responses API）和 Anthropic 模式各自的 function call 能力是最近才整合进去的。这引发了架构层面的担忧：model 与 action 之间的操作关系变化是否在架构文档和代码两个层面做到了同步？储备的 usage 处理、session 回溯日志、沙箱与防注入机制是否存在潜在问题？需要参考 langchaingo、eino、agently 进行全面横向评估，输出一份系统化的架构一致性审计报告，为后续优化提供依据。

## What Changes

- 产出一份结构化的架构一致性审计报告，覆盖以下 10 个分析维度：
  1. **model ↔ action 操作关系一致性**：ToolCall/ToolDefinition 的定义与消费链路、三种 Provider（OpenAI Chat / OpenAI Responses / Anthropic）function call 实现差异、native tool calls 与 legacy JSON decision 双路径并存的风险
  2. **usage 处理链路完整性**：UsageInfo 从 Provider 提取 → StreamChunk → BroadcastResponse → ModelResponse → agent 层消费的完整链路、重试/failover 场景下的 usage 累加、executeLoop 跳过 BroadcastResponse 导致 usage 丢失的风险
  3. **session 与回溯日志一致性**：ThreeZoneSession 三区架构、SmartCompressResizeHandler 压缩策略、AuditChain 哈希链、session 持久化与审计日志的一致性保证、崩溃恢复能力
  4. **沙箱安全边界**：SandboxExecutor 执行链路、多后端隔离能力、策略可被 LLM 参数操控的风险、间接注入（tool 返回内容）未检测的缺口
  5. **防注入与 PII 脱敏覆盖度**：prompt injection detector 的规则覆盖、PII masker 对非 string content 的处理盲区、多语言变体检测缺失
  6. **模块边界与依赖方向**：包间依赖是否合理、桥接层（如 adapt_action.go）是否冗余、是否有循环依赖或跨层调用，直接回答"哪些代码组织是无用的"
  7. **代码重复与接口抽象**：三个 Provider SSE 解析的重复逻辑、resize handlers 的可合并性、ModelRequester/ActionExecutor/SessionBackend 接口职责是否过宽
  8. **包结构与目录组织**：是否有 god package、internal/ 划分合理性、目录结构是否反映职责边界
  9. **扩展性评估**：量化新增一个 Provider / action 执行器 / 安全检查点需要改动的文件数和接触点，评估架构可维护性
  10. **横向对比（langchaingo / eino / agently）**：泛型接口设计、tool 绑定不可变性、中间件机制、多模态支持、usage 标准化、安全集成模式

- 每个维度给出：现状描述、潜在问题（按严重程度 P0/P1/P2 分级）、与参考框架的差距、改进建议
- 维度 6-9 额外给出：可消除的冗余代码/包清单、接口拆分建议、包重组方案、扩展点改造建议
- 最终汇总为一张问题矩阵和优先级排序表，以及一张架构调整建议总表（含"删除/合并/拆分/保留"标注）

## Impact

- **Affected specs**:
  - `maturity-analysis-inferglow-vs-eino`（本审计在此基础上深入到架构一致性层面）
  - `post-g1-roadmap`（审计结果可能影响后续路线图优先级）
  - `adapt-non-standard-providers`（function call 差异分析相关）
- **Affected code**: 无代码变更，纯分析产出
- **Deliverable**: `docs/architecture-consistency-audit.md` 审计报告

## ADDED Requirements

### Requirement: model ↔ action 链路分析
分析 SHALL 追踪 ToolCall 从 Provider SSE 流到 action 执行的完整数据流，覆盖三种 Provider 的 function call 实现差异，并识别双路径（native tool calls vs legacy JSON decision）并存的维护风险。

#### Scenario: executeLoop 跳过 BroadcastResponse
- **WHEN** 分析 executeLoop 的 stream 消费逻辑
- **THEN** 报告指出 executeLoop 直接消费 `<-chan *StreamChunk`（只读取 Delta 和 Tools），跳过了 BroadcastResponse
- **AND** 说明这导致 UsageInfo、ReasoningDelta、MetaEvent 等事件无法传递到 agent 层

#### Scenario: 三种 Provider function call 格式差异
- **WHEN** 对比 OpenAI Chat Completions、OpenAI Responses API、Anthropic Messages API 的 tool call 实现
- **THEN** 报告列出三者在 SSE 事件类型、ToolCall ID 格式、参数累积方式上的具体差异
- **AND** 评估这些差异是否被 SessionExtension.AddAssistantToolCalls / AddToolResultNamed 正确处理

### Requirement: usage 处理链路分析
分析 SHALL 追踪 UsageInfo 从 Provider 提取到上层消费的完整路径，识别 usage 丢失、不精确估算、重试场景未累加等风险。

#### Scenario: executeLoop 中 usage 丢失
- **WHEN** 分析 executeLoop 的 token 计数逻辑
- **THEN** 报告指出 `totalTokens += len(content.String())` 使用字符数近似 token 数
- **AND** 说明真实的 UsageInfo（含 PromptTokens/CompletionTokens/ReasoningTokens）因跳过 BroadcastResponse 而完全丢失

#### Scenario: failover/attempt 重试时 usage 未累加
- **WHEN** 分析 failover.go 和 attempt.go 的重试逻辑
- **THEN** 报告说明重试成功后是否累加之前失败尝试的 usage
- **AND** 评估这对计费精度的影响

### Requirement: session 与审计日志一致性分析
分析 SHALL 评估 ThreeZoneSession 持久化与 AuditChain 审计日志之间的一致性保证，以及崩溃恢复能力。

#### Scenario: session 与审计日志无事务性保证
- **WHEN** 分析 session.SaveJSON 和 audit.Storage.Save 的调用时机
- **THEN** 报告指出两者独立写入，没有事务性保证
- **AND** 说明崩溃后可能出现 session 已保存但审计日志未写入（或反之）的不一致状态

#### Scenario: 回溯重建能力缺失
- **WHEN** 评估从审计日志重建 session 状态的能力
- **THEN** 报告指出当前审计日志记录的是 decision 级别事件，缺少完整的消息级回放能力
- **AND** 说明无法从审计日志精确重建 ThreeZoneSession 的三区状态

### Requirement: 沙箱安全边界分析
分析 SHALL 评估 SandboxExecutor 的执行链路安全性，识别策略可被 LLM 参数操控和间接注入的风险。

#### Scenario: 策略可被 LLM 参数操控
- **WHEN** 分析 buildPolicyFromInput 的输入来源
- **THEN** 报告指出 timeout/network_access/path_allowlist/max_output_bytes 均从 input map 读取
- **AND** 说明 LLM 生成的 tool call 参数可以直接操控沙箱策略，存在逃逸风险

#### Scenario: 间接注入未检测
- **WHEN** 分析 prompt injection detector 的检测范围
- **THEN** 报告指出 detector 只检查用户输入文本，不检查 tool 返回的内容
- **AND** 说明恶意 tool 返回内容（如 MCP 服务器的输出）可以注入指令到 session 中

### Requirement: 防注入与 PII 覆盖度分析
分析 SHALL 评估 prompt injection detector 的规则覆盖度和 PII masker 的处理盲区。

#### Scenario: PII masker 对非 string content 失效
- **WHEN** 分析 Session.AddMessageChecked 的 masker 逻辑
- **THEN** 报告指出 masker 只对 `string` 类型 content 执行 MaskInput
- **AND** 说明 `[]ContentBlock` 类型的多模态内容不会被脱敏

#### Scenario: 防注入规则覆盖度不足
- **WHEN** 评估 detector.go 的关键词和正则规则
- **THEN** 报告列出未覆盖的注入变体（多语言、Unicode 混淆、分段注入等）
- **AND** 对比 eino/langchaingo 的安全集成模式

### Requirement: 模块边界与依赖方向分析
分析 SHALL 评估所有包之间的依赖关系，识别冗余的桥接层、循环依赖和跨层调用，产出可消除的无用代码组织清单。

#### Scenario: adapt_action.go 桥接层冗余评估
- **WHEN** 分析 components/tool/adapt_action.go 的 ActionToTool 使用情况
- **THEN** 报告追踪 ActionToTool 的所有调用点，判断是否与 engine.buildToolDefinitions 路径重复
- **AND** 如果重复，建议删除或合并路径

#### Scenario: 跨层调用与循环依赖检测
- **WHEN** 分析 model/action/orchestrator/session/security 包间的 import 关系
- **THEN** 报告列出所有跨层调用（如 model 包 import action 包）和潜在循环依赖
- **AND** 给出依赖方向修正建议

### Requirement: 代码重复与接口抽象分析
分析 SHALL 识别三个 Provider SSE 解析逻辑的重复代码、resize handlers 的可合并性，并评估核心接口的职责宽度。

#### Scenario: Provider SSE 解析重复代码
- **WHEN** 对比 openai.go、anthropic.go、openai_responses.go 的 SSE 行解析逻辑
- **THEN** 报告量化重复代码行数，识别可提取的公共逻辑（如 SSE 行读取、JSON 解析、error 处理）
- **AND** 给出提取方案（如 sse_parser.go 公共模块）

#### Scenario: ModelRequester 接口职责过宽
- **WHEN** 分析 ModelRequester 接口的三个方法（GenerateRequestData + RequestModel + BroadcastResponse）
- **THEN** 报告评估每个方法的调用者，判断是否所有调用者都需要全部三个方法
- **AND** 给出接口拆分建议（如拆分为 RequestBuilder + StreamProvider + ResultBroadcaster）

### Requirement: 包结构与目录组织分析
分析 SHALL 评估包结构是否反映职责边界，识别 god package 和 internal/ 划分不合理之处。

#### Scenario: god package 检测
- **WHEN** 统计各包的文件数和导出符号数
- **THEN** 报告列出文件数 >10 或导出符号数 >30 的包，评估其是否承担了过多职责
- **AND** 给出拆分建议

#### Scenario: internal/ 划分合理性
- **WHEN** 检查哪些包应标记为 internal/ 但未标记
- **THEN** 报告列出不应对外暴露的包（如 orchestrator 内部辅助包），建议移入 internal/

### Requirement: 扩展性评估
分析 SHALL 量化新增 Provider、action 执行器、安全检查点需要改动的文件数和接触点，评估架构可维护性。

#### Scenario: 新增 Provider 的扩展成本
- **WHEN** 追踪新增一个 Model Provider 需要修改的所有文件
- **THEN** 报告列出需要新建和修改的文件清单，计算总接触点数
- **AND** 对比 eino 新增 Provider 的扩展成本

#### Scenario: 新增安全检查点的扩展成本
- **WHEN** 追踪新增一个安全检查规则需要修改的所有文件
- **THEN** 报告列出需要修改的文件清单（detector.go、config.go、security_hook.go 等）
- **AND** 给出降低扩展成本的架构建议（如插件化安全规则）

### Requirement: 横向对比分析
分析 SHALL 将 InferGlow 的实现与 langchaingo、eino、agently 在 10 个维度上进行横向对比，每个维度给出差距评分和改进建议。

#### Scenario: 泛型接口设计差距
- **WHEN** 对比 eino 的 `BaseModel[M]` 泛型接口与 InferGlow 的非泛型 `ModelRequester` 接口
- **THEN** 报告说明 eino 通过泛型实现了类型安全的消息传递，InferGlow 依赖 `any` 类型 Content

#### Scenario: tool 绑定不可变性差距
- **WHEN** 对比 eino 的 `ToolCallingChatModel.WithTools`（返回新实例）与 InferGlow 的 `buildToolDefinitions`（每轮重建）
- **THEN** 报告说明 eino 的不可变设计支持并发安全的多 tool set 共享，InferGlow 每轮重建存在性能开销

### Requirement: 问题矩阵与架构调整建议总表
分析 SHALL 在报告末尾提供一张问题矩阵（按 P0/P1/P2 分级）和一张架构调整建议总表（含"删除/合并/拆分/保留"标注）。

#### Scenario: P0 级问题识别
- **WHEN** 查看问题矩阵
- **THEN** executeLoop 跳过 BroadcastResponse 导致 usage 丢失、沙箱策略可被 LLM 参数操控等问题被标记为 P0
- **AND** 每个问题包含具体改进路径和影响的文件列表

#### Scenario: 架构调整建议总表
- **WHEN** 查看架构调整建议总表
- **THEN** 每个识别的冗余/问题代码被标注为"删除/合并/拆分/保留"之一
- **AND** 标注为"删除"的条目包含替代路径说明
- **AND** 标注为"合并"的条目包含合并目标
- **AND** 标注为"拆分"的条目包含拆分方案

## MODIFIED Requirements

无。本 Spec 为纯分析产出，不修改任何现有需求或代码。

## REMOVED Requirements

无。
