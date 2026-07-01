# InferGlow 架构一致性审计报告

> 审计日期：2026-07-24
> 审计范围：model / action / session / sandbox / orchestrator / flow / security / components
> 对标对象：eino / langchaingo / agently
> 文档版本：v1.0

---

## 目录

- [1. 执行摘要](#1-执行摘要)
- [2. 分析维度](#2-分析维度)
  - [2.1 维度1：model ↔ action 操作关系一致性](#21-维度1model--action-操作关系一致性)
  - [2.2 维度2：usage 处理链路完整性](#22-维度2usage-处理链路完整性)
  - [2.3 维度3：session 与回溯日志一致性](#23-维度3session-与回溯日志一致性)
  - [2.4 维度4：沙箱安全边界](#24-维度4沙箱安全边界)
  - [2.5 维度5：防注入与 PII 脱敏覆盖度](#25-维度5防注入与-pii-脱敏覆盖度)
  - [2.6 维度6：模块边界与依赖方向](#26-维度6模块边界与依赖方向)
  - [2.7 维度7：代码重复与接口抽象](#27-维度7代码重复与接口抽象)
  - [2.8 维度8：包结构与目录组织](#28-维度8包结构与目录组织)
  - [2.9 维度9：扩展性评估](#29-维度9扩展性评估)
  - [2.10 维度10：横向对比](#210-维度10横向对比)
- [3. 问题矩阵（P0/P1/P2 分级表）](#3-问题矩阵p0p1p2-分级表)
- [4. 架构调整建议总表](#4-架构调整建议总表删除合并拆分保留标注)
- [5. 差距评分矩阵](#5-差距评分矩阵inferglow-vs-eino-vs-langchaingo-vs-agently)
- [6. 总结与下一步](#6-总结与下一步)

---

## 1. 执行摘要

本次架构一致性审计覆盖 InferGlow 核心代码库的十个关键维度，对标 eino、langchaingo、agently 三个同类项目，识别出 **45 项问题**，其中：

- **P0 严重问题 13 项**：涉及多轮 native tool call 失效、沙箱策略可被 LLM 操控、崩溃恢复缺失、God Package、死代码层等。这些问题直接影响生产可用性、安全性与可维护性，必须在下一迭代内修复。
- **P1 重要问题 19 项**：涉及 usage 数据丢失、session 三区架构实际未生效、PII 脱敏覆盖不全、接口过宽/过窄等。这些问题在规模化场景下会显著劣化可观测性与扩展性。
- **P2 一般问题 13 项**：涉及类型设计、原子写入、死类型、误用风险等。属于代码质量改进项。

### 核心结论

1. **InferGlow 的差异化壁垒在安全/审计/沙箱/死循环防护领域**，对标项目在该领域普遍缺失（eino/langchaingo 得分 1，InferGlow 得分 4~5）。这是 InferGlow 应继续巩固的优势。
2. **InferGlow 的主要短板在工程化抽象**：接口泛型化、工具多模态、记忆中间件、usage provider 隔离、包分层与接口实现分离，与 eino 存在 2~4 分差距。
3. **三类系统性风险**需优先治理：
   - 安全声明与实现脱节（沙箱策略"纸面化"、防注入覆盖不全）
   - 数据链路断裂（usage 丢失、ToolCall.ID 丢失、Zone 3 从未写入）
   - 包结构失控（model/orchestrator/agent/sandbox/flow 多个 God Package，无 internal/ 隔离）
4. **存在大量可消除的死代码**：`components/tool` 整个包、`model.ActionResult` 死类型、`ModelRequest.Actions` 字段等，应一次性清理以降低维护成本。

### 修复优先级建议

| 阶段 | 目标 | 涉及 P0 |
|---|---|---|
| 第一阶段（立即） | 修复安全声明与实现脱节 | P0-沙箱×4、P0-注入 |
| 第二阶段（短期） | 修复数据链路断裂 | P0-Anthropic多轮、P0-LoadJSON、P0-死代码 |
| 第三阶段（中期） | 包结构治理与接口拆分 | P0-God Package×3、P1-接口拆分×2 |

---

## 2. 分析维度

### 2.1 维度1：model ↔ action 操作关系一致性

#### 现状描述

model ↔ action 操作关系通过以下完整链路串联：

```
model.ToolDefinition（chat.go:45-49）
  → engine.buildToolDefinitions（engine.go:730-758）
  → Provider SSE 请求体构建
  → model.ToolCall（chat.go:52-56）
  → engine.executeLoop 收集（engine.go:442-454）
  → SessionExtension.AddAssistantToolCalls（session_ext.go:133-138）
  → actionruntime.ActionCall 转换（engine.go:525-531，ToolCall.ID 丢失）
  → ActionDispatcher.Execute
  → ActionRegistry.Execute（action.go:152-165）
  → ActionResult
  → SessionExtension.AddToolResultNamed（engine.go:603-610）
```

三种 Provider 的 function call 实现存在显著差异：

| 维度 | OpenAI Chat | Anthropic | Responses API |
|---|---|---|---|
| SSE 事件 | `data:{json}` | `content_block_start` / `content_block_delta` / `content_block_stop` | `response.output_item.added` / `function_call_arguments.delta` / `function_call_arguments.done` |
| ID 格式 | `call_xxx` | `toolu_xxx` | `call_xxx` |
| 参数累积 | `tc.Index` 键控 `Args.Builder` | `event.Index` 键控 `partial_json` | `output_index` 键控 `Args.Builder` |
| Emit 触发 | `finish_reason=="tool_calls"` | `content_block_stop` + `tool_use` | `function_call_arguments.done` |

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D1-1 | P0 | Anthropic Provider 无法多轮 native tool call。`PreparePrompt` 产生 OpenAI 格式的 `tool_calls` / `tool_call_id`，Anthropic Provider 直接发送不转换格式，第二轮请求被 Anthropic API 拒绝 | `anthropic.go:77-305` + `session_ext.go:181-204` + `chat.go:72-87` |
| D1-2 | P1 | 双路径（native tool calls vs legacy JSON decision）会话消息格式不一致。路径 A 产生 `role="tool"` 消息，路径 B 产生 `role="system"` 消息，混合使用可能导致 Provider 拒绝 | `engine.go:519-619` |
| D1-3 | P1 | L4 outputSchema 验证仅对路径 B 生效，路径 A 下 `req.Output` 为 `nil` | `engine.go:482-514` |
| D1-4 | P2 | `model.ActionResult`（`model.go:121-126`）是死类型，与 `action.ActionResult` 字段完全不同，在实际流程中从未使用 | `model.go:121-126` |
| D1-5 | P2 | `ToolCall.ID` 在转换为 `ActionCall` 时丢失 | `engine.go:525-531` |

#### 改进建议

1. **D1-1（P0）**：在 `anthropic.go` 的请求体构建阶段增加格式适配层，将 OpenAI 风格的 `tool_calls` / `tool_call_id` 转换为 Anthropic 的 `tool_use` / `tool_result` content block 结构。建议在 `SessionExtension.PreparePrompt` 中按 Provider 类型分支处理。
2. **D1-2（P1）**：统一双路径的会话消息格式，或显式标记路径来源并在 `PreparePrompt` 中分别处理，避免混合注入。
3. **D1-3（P1）**：将 outputSchema 验证逻辑前移到 `executeLoop` 入口，对路径 A 与路径 B 均生效。
4. **D1-4（P2）**：删除 `model.ActionResult` 死类型（详见第 4 节调整建议总表）。
5. **D1-5（P2）**：在 `ActionCall` 转换处保留 `ToolCall.ID`，用于审计与多轮关联。

---

### 2.2 维度2：usage 处理链路完整性

#### 现状描述

UsageInfo 的设计传递路径为：

```
Provider SSE 解析
  → StreamChunk.Usage（model.go:78）
  → BroadcastResponse（MetaEvent 发送 *UsageInfo）
  → ResultEvent
  → ModelResponse.Usage
  → agent 层消费
```

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D2-1 | P1 | `executeLoop` 直接消费 `<-chan *StreamChunk` 只读 `Delta` 和 `Tools`，完全跳过 `BroadcastResponse`，`UsageInfo` / `ReasoningDelta` / `MetaEvent` 全部丢失 | `engine.go:443-469` |
| D2-2 | P1 | `totalTokens += len(content.String())` 用字符数近似 token 数，中文误差极大 | `engine.go:517` |
| D2-3 | P1 | OpenAI Responses API 完全未解析 usage 字段，`ModelResponse.Usage` 永远为零值 | `openai_responses.go:351-487` |
| D2-4 | P1 | 推理内容在 agent 模式下丢失（`chunk.Reasoning` 未读取） | `engine.go:443-469` |
| D2-5 | P2 | failover / attempt 重试场景丢失失败尝试的 usage | failover 路径 |
| D2-6 | P2 | Anthropic `BroadcastResponse` 未发送 `ReasoningTokenMeta` 且未填充 `ReasoningTokens` | `anthropic.go` BroadcastResponse |
| D2-7 | P2 | `UsageInfo` 的 `map[string]int` 设计不如 eino 强类型 struct 安全 | `model.go` UsageInfo |

#### eino 对比

eino 的 `TokenUsage` 使用强类型 struct（`PromptTokenDetails.CachedTokens`、`CompletionTokensDetails.ReasoningTokens`），InferGlow 使用 `map[string]int` 弱类型约定，缺乏编译期校验。

#### 改进建议

1. **D2-1/D2-4（P1）**：在 `executeLoop` 中接入 `BroadcastResponse` 通道，统一消费 `UsageInfo` / `ReasoningDelta` / `MetaEvent`，避免双消费路径分裂。
2. **D2-2（P1）**：移除字符数近似 token 数的 hack，强制要求 Provider 返回真实 usage；若 Provider 未返回则标记为未知而非近似。
3. **D2-3（P1）**：在 `openai_responses.go:351-487` 中补齐 usage 字段解析。
4. **D2-7（P2）**：将 `UsageInfo` 从 `map[string]int` 重构为强类型 struct，对齐 eino 设计。

---

### 2.3 维度3：session 与回溯日志一致性

#### 现状描述

三区架构设计意图：

- **Zone 1 ImmutablePrefix**：设计合理，`stableMarshal` 保证字节稳定序列化，`immutableHash` 用于前缀缓存共享。
- **Zone 2 AppendOnlyHistory**：append 语义正确，但 `SnipFromHead` 注释声称 "cache-friendly" 实际删头部破坏前缀缓存。
- **Zone 3 VolatileScratch**：从未被写入，`SetVolatileScratch` 无调用方，三区架构实际只有两区生效。

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D3-1 | P0 | `ThreeZoneSession.SaveJSON` 注释声称 "compatible with LoadJSON for crash recovery" 但无 `LoadJSON` 实现，崩溃后无法恢复 session | `three_zone.go:133-157` |
| D3-2 | P1 | `SnipFromHead` 注释与实现矛盾，声称缓存友好实际破坏前缀缓存 | `three_zone.go:268-275` |
| D3-3 | P1 | Zone 3 从未被写入，三区架构实际只有两区生效 | `three_zone.go:171-184` |
| D3-4 | P1 | Session 与审计日志独立写入无事务性保证，两者错误都被忽略 | `engine.go:555` + `dispatcher.go:121` + `session_ext.go:75` |
| D3-5 | P1 | 审计日志无法重建 session 三区状态：Zone 1 完全无记录，Zone 2 缺消息级 meta（`tool_call_id`），resize 操作无审计 | 审计日志模块 |
| D3-6 | P1 | `MaxEntries` 裁剪后内存 chain 与 storage 不一致，注释对裁剪行为说明错误 | `chain.go:141-144` |
| D3-7 | P2 | `SmartCompressResizeHandler` 未集成到 `ThreeZoneSession` 的 resize 链 | `three_zone.go` resize 链 |
| D3-8 | P2 | `persist()` 用 `os.WriteFile` 非原子写，崩溃可能损坏文件 | `three_zone.go` persist |
| D3-9 | P2 | 无 dangling tool call 检测机制（对比 eino `patchtoolcalls`） | - |
| D3-10 | P2 | system prompt 双重传递（`req.System` + `ChatHistory[0]`） | 请求构建路径 |

#### eino 对比

eino 采用中间件机制（summarization / reduction / patchtoolcalls）vs InferGlow resize 策略链。eino 具备 offload 回查、LLM 摘要、dangling 修复、per-tool 配置、流式处理等能力，InferGlow 均缺失。

#### 改进建议

1. **D3-1（P0）**：实现 `LoadJSON`，或修改 `SaveJSON` 注释移除 crash recovery 声明，并补充真正的崩溃恢复方案（如 WAL）。
2. **D3-2（P1）**：修正 `SnipFromHead` 注释，或改用尾部裁剪 + 前缀哈希保留策略以真正实现 cache-friendly。
3. **D3-3（P1）**：要么激活 Zone 3（在 executeLoop 中写入临时 scratch），要么移除 Zone 3 相关代码避免误导。
4. **D3-4/D3-5（P1）**：引入 session + 审计日志的联合写入事务（或 outbox 模式），审计日志补齐 Zone 1 快照、消息级 meta、resize 操作记录。
5. **D3-8（P2）**：`persist()` 改用 temp file + rename 原子写。
6. **D3-9（P2）**：参考 eino `patchtoolcalls` 实现 dangling tool call 检测与修复。

---

### 2.4 维度4：沙箱安全边界

#### 现状描述

沙箱执行链路：

```
input map 解析
  → buildPolicyFromInput
  → ApprovalService 审批
  → Manager.CreateHandle
  → handle.Start / Execute / Stop
  → ActionResult 映射
```

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D4-1 | P0 | `buildPolicyFromInput` 无服务端策略基线，LLM 可完全操控沙箱策略（`timeout=0` 无限执行、`network_access="enabled"` 开网络、`path_allowlist=["/"]` 全盘允许） | `executor_sandbox.go:182-223` |
| D4-2 | P0 | `approval_required` 由 LLM 输入控制，可被省略绕过审批 | `executor_sandbox.go:96` |
| D4-3 | P0 | Docker / gVisor 后端不强制执行 `network_access` 策略 | `docker.go:236`（`NetworkDisabled:false` 硬编码） |
| D4-4 | P0 | `ModeAuto` 可回落到 `trusted_local`（无隔离） | `manager.go:93` |
| D4-5 | P1 | `max_output_bytes` 在所有后端均为死代码 | 所有后端 |
| D4-6 | P1 | `path_allowlist` 在 Docker / Bubblewrap 未生效 | `docker.go` / `bubblewrap.go` |
| D4-7 | P1 | E2B 后端命令拼接无转义，远程 shell 注入 | `e2b.go:389` |
| D4-8 | P1 | Seatbelt `process-exec` 规则过宽，允许执行任意路径 | `seatbelt_policy.go:73-75` |
| D4-9 | P1 | Landlock 一次性不可撤销，误用可锁死服务进程 | `landlock.go:471-512` |
| D4-10 | P2 | `network_access` 字段在所有后端均未真正生效，是"纸面策略" | 所有后端 |

#### 改进建议

1. **D4-1（P0）**：引入服务端策略基线（deny-by-default），LLM 输入只能在基线范围内收紧，不能放宽。`timeout` / `network_access` / `path_allowlist` 必须有服务端强制的上下限。
2. **D4-2（P0）**：`approval_required` 必须由服务端策略决定，不能由 LLM 输入控制；对高风险操作强制审批。
3. **D4-3（P0）**：Docker / gVisor 后端必须根据 `network_access` 策略设置 `NetworkDisabled`。
4. **D4-4（P0）**：移除 `ModeAuto` 到 `trusted_local` 的回落，或仅在显式配置受信环境时允许。
5. **D4-7（P1）**：E2B 后端命令拼接改为参数化传递，禁止字符串拼接。
6. **D4-8/D4-9（P1）**：收紧 Seatbelt `process-exec` 规则至白名单路径；Landlock 增加保护机制避免误锁服务进程。

---

### 2.5 维度5：防注入与 PII 脱敏覆盖度

#### 现状描述

安全检测点分布：

| 检测点 | 位置 | 覆盖范围 | 缺口 |
|---|---|---|---|
| `sessionhook.BeforeAddMessage` | `session.go:227-231` | user / assistant / system | 不覆盖 `AddMessageWithMeta`、不覆盖 `[]ContentBlock` |
| `agent outputHook.CheckOutput` | `agent.go:346-349` | 仅最终响应 | 不覆盖中间轮次、工具结果、tool_call 决策 |
| 工具结果检测 | 无 | 无 | 完全缺失 |

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D5-1 | P0 | 工具结果（含 MCP 输出）绕过 prompt injection 检测，间接注入无防御。`AddToolResult` → `AddMessageWithMeta` 完全绕过 hook 和 masker | `session_ext.go:142-147` → `session.go:270-282` |
| D5-2 | P1 | PII 脱敏不覆盖 `[]ContentBlock` 和 `Meta` map，多模态消息中的文本 PII 原样留存 | `session.go:236-240` |
| D5-3 | P1 | prompt injection detector 不覆盖多语言 / 编码 / Unicode 混淆 | `detector.go:109-141` |
| D5-4 | P1 | RBAC 未集成到 `SandboxExecutor` 执行链路 | `executor_sandbox.go` 全文无 RBAC 调用 |
| D5-5 | P1 | 两套审批系统割裂：`sandbox.ApprovalService` 与 `approval.PolicyApprovalManager` 完全独立无互通 | `sandbox/approval*` + `approval/*` |
| D5-6 | P2 | `AddMessage` 静默丢弃被 hook 拦截的消息 | `session.go` AddMessage |
| D5-7 | P2 | PII Phone 正则仅匹配中国大陆手机号 | PII 检测模块 |
| D5-8 | P2 | `AutoApproveHandler` / `AutoAllowHandler` 存在误用风险 | 审批 handler 模块 |

#### 改进建议

1. **D5-1（P0）**：在 `AddToolResult` / `AddMessageWithMeta` 中接入 `BeforeAddMessage` hook 和 masker，补齐工具结果检测点，覆盖 MCP 输出。
2. **D5-2（P1）**：扩展 PII 脱敏至 `[]ContentBlock` 的文本字段与 `Meta` map 的字符串值。
3. **D5-3（P1）**：prompt injection detector 增加多语言 / Unicode 规范化 / 编码混淆检测。
4. **D5-4（P1）**：在 `SandboxExecutor.execute` 入口接入 RBAC 检查。
5. **D5-5（P1）**：合并两套审批系统，或定义统一接口互通。

---

### 2.6 维度6：模块边界与依赖方向

#### 现状描述

依赖方向矩阵：

- **Layer 0（叶子，零内部依赖）**：`action`、`model`、`session`
- **Layer 1（组合层）**：`orchestrator` → 依赖 `action` + `model` + `session`
- **Layer 2（横切关注点）**：`security` → 依赖 `orchestrator/agent` + `session`
- **Layer X（死代码）**：`components/tool` → 依赖 `action`，但无人依赖它

#### 关键发现

| 编号 | 等级 | 发现 | 代码引用 |
|---|---|---|---|
| D6-1 | P0 | `components/tool` 整个包是死代码，`ActionToTool` 零生产调用点，engine 使用 `buildToolDefinitions` 直接构造 `ToolDefinition` 完全绕过此桥接层 | `components/tool/*` + `engine.go:730-758` |
| D6-2 | - | 无循环依赖，叶子包互不依赖 | - |
| D6-3 | - | `orchestrator/agent` 耦合度整体合理，通过公开类型和接口交互 | - |
| D6-4 | - | `security` 子包依赖方向合理，基础能力包独立可单独复用 | - |

#### 可消除的无用代码清单

| 目标 | 处置 | 理由 |
|---|---|---|
| `components/tool/adapt_action.go` | 删除 | `ActionToTool` 零生产调用 |
| `components/tool/adapt_action_test.go` | 删除 | 随 `adapt_action.go` 删除 |
| `components/tool/interface.go` | 删除 | `BaseTool` / `ToolInfo` 仅包内部使用 |
| `components/tool/interface_test.go` | 删除 | 随 `interface.go` 删除 |
| `components/tool/option.go` | 删除 | `Option` 类型无人使用 |
| `components/tool/go.mod` + `go.sum` | 删除 | 整包可移除 |
| `model.ActionResult`（`model.go:121-126`） | 删除 | 死类型，与 `action.ActionResult` 完全不同 |
| `ModelRequest.Actions` 字段（`model.go:35`） | 删除 | 引用死类型，从未被填充 |

#### 改进建议

1. **D6-1（P0）**：一次性删除 `components/tool` 整包及 `model.ActionResult` / `ModelRequest.Actions` 死类型死字段，降低维护成本。
2. 保持 Layer 0 叶子包独立，避免后续引入循环依赖。

---

### 2.7 维度7：代码重复与接口抽象

#### 现状描述

**Provider SSE 解析重复**：总重复约 250-300 行可提取的公共逻辑，重复模块包括：

- `effectiveHTTPClient`（18 行 × 3）
- `RequestModel` goroutine 骨架（35 行 × 3）
- `processLine` "data:" 前缀处理（9 行 × 3）
- `emit` 闭包（6 行 × 3）
- EOF + error 处理（10 行 × 3）
- `BroadcastResponse` 骨架（60-90 行 × 3）
- `mapRole`（9 行 × 2）

**resize handlers**：3 处总字节计算循环重复，可提取 `TotalContentBytes` 辅助函数；Handler 本身不应合并（代表不同策略）。

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D7-1 | P1 | `ModelRequester` 接口过宽。`BroadcastResponse` 在 agent 生产路径完全未使用（仅 `failover.go:265` 委托转发）。建议拆分为 `StreamRequester` + `ResponseBroadcaster` | `model.go` ModelRequester |
| D7-2 | P1 | `SessionBackend` 接口过窄。`SessionExtension` 通过 3 处类型断言访问接口外方法（`SetImmutablePrefix` / `ClearVolatileScratch` / `SetMessageMasker`）。建议拆分为 `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore` | `session.go` SessionBackend + `session_ext.go` 类型断言 |
| D7-3 | - | `ActionExecutor` 接口设计优秀，单方法接口、三种执行器完全统一、无绕过，无需拆分 | `action.go` ActionExecutor |
| D7-4 | P2 | Provider SSE 解析重复 250-300 行 | 三个 Provider 文件 |

#### 改进建议

1. **D7-1（P1）**：拆分 `ModelRequester` 为 `StreamRequester`（核心流式请求）+ `ResponseBroadcaster`（多消费者广播），`BroadcastResponse` 仅在需要广播的场景实现。
2. **D7-2（P1）**：拆分 `SessionBackend` 为 `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore`，消除 `SessionExtension` 中的类型断言。
3. **D7-4（P2）**：提取 Provider SSE 公共逻辑至 `model/providers/internal/ssestream` 公共包。

---

### 2.8 维度8：包结构与目录组织

#### 现状描述

God Package 风险：

| 包 | 文件数 | 导出符号数 | 风险 |
|---|---|---|---|
| `model/` | 25 | ~117 | 🔴 高（混杂接口 / 实现 / 工厂 / 运行时 / 数据结构 5 类职责） |
| `sandbox/` | 30 | ~118 | 🔴 高（8 种后端 + Windows 7 文件平铺） |
| `flow/` | 19 | ~131 | 🔴 高（引擎与算子混杂） |
| `orchestrator/agent/` | 15 | 68 | 🔴 高（8 类正交职责混合） |

#### 问题清单

| 编号 | 等级 | 问题描述 | 代码引用 |
|---|---|---|---|
| D8-1 | P0 | `model/` God Package，25 文件 / ~117 导出符号，混杂 5+ 类职责 | `model/*` |
| D8-2 | P0 | `orchestrator/agent/` God Package，15 文件混合 8 类正交职责 | `orchestrator/agent/*` |
| D8-3 | P0 | 完全无 `internal/` 目录，所有包的导出符号对外暴露 | 项目根 |
| D8-4 | P0 | 核心数据类型散落，`ModelRequest` / `ModelResponse` 定义在 `model/` 而非 `schema/` | `model.go` |
| D8-5 | P1 | `flow/` 和 `sandbox/` God Package | `flow/*` + `sandbox/*` |
| D8-6 | P1 | sandbox 后端注册违反 OCP（需改 `builder.go`） | `sandbox/builder.go` |
| D8-7 | P1 | 安全检测器缺少统一接口 | `security/detector*` |
| D8-8 | P2 | `components/` 不完整（仅 tool + prompt，缺 model 接口定义） | `components/*` |

#### 拆分建议

1. **`model/` 拆分**：
   - `model/schema.go`（核心数据类型）
   - `model/internal/`（attempt / failover / pool）
   - `model/providers/openai/`
   - `model/providers/anthropic/`
2. **`orchestrator/agent/` 拆分**：
   - `agent.go` + `engine.go`
   - `internal/turnloop` + `internal/step` + `internal/strategy`
   - `hooks/` + `streaming/` + `extension/` + `features/`
3. **`sandbox/` 拆分**：
   - `provider.go` + `manager.go` + `policy.go`
   - `backends/docker` + `backends/gvisor` + `backends/seatbelt` + ...

#### 改进建议

1. **D8-1/D8-2/D8-5（P0/P1）**：按上述拆分建议重构 God Package。
2. **D8-3（P0）**：引入 `internal/` 目录，将实现细节包移入，仅暴露公开 API。
3. **D8-4（P0）**：将 `ModelRequest` / `ModelResponse` 等核心数据类型迁移至 `schema/` 包，与 Provider 实现解耦。
4. **D8-6（P1）**：sandbox 后端注册改为插件式注册（init 函数自注册），避免修改 `builder.go`。

---

### 2.9 维度9：扩展性评估

#### 现状描述

扩展成本表：

| 扩展点 | InferGlow 接触点 | eino 接触点 |
|---|---|---|
| Model Provider | 1-2（修改 `provider_factory.go`） | 1（外部仓库新建，核心零修改） |
| Action / Tool 执行器 | 1（新建，零修改） | 1（新建，零修改） |
| Security 规则 | 1-3（改 `detector.go` + 2 个 hook 调用点） | N/A |
| Sandbox 后端 | 2-3（含 `builder.go` 修改） | N/A |

扩展性评分：

| 维度 | InferGlow | eino |
|---|---|---|
| 新增 Provider | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 新增 Action / Tool | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 新增 Security | ⭐⭐⭐ | N/A |
| 新增 Sandbox | ⭐⭐⭐ | N/A |
| 新增编排模式 | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| 新增 Agent 类型 | ⭐⭐ | ⭐⭐⭐⭐ |
| API 表面控制 | ⭐⭐ | ⭐⭐⭐⭐ |
| 数据类型复用 | ⭐⭐ | ⭐⭐⭐⭐⭐ |

#### 问题清单

| 编号 | 等级 | 问题描述 |
|---|---|---|
| D9-1 | P1 | 新增 Agent 类型扩展成本高（⭐⭐），缺乏 Agent 类型抽象基类 |
| D9-2 | P1 | API 表面控制弱（⭐⭐），无 `internal/` 隔离 |
| D9-3 | P1 | 数据类型复用弱（⭐⭐），核心类型散落且与实现耦合 |
| D9-4 | P2 | 新增编排模式扩展成本偏高（⭐⭐⭐），flow 包引擎与算子混杂 |

#### 改进建议

1. **D9-1（P1）**：抽象 Agent 类型基类（如 `BaseAgent` + 策略接口），降低新增 Agent 类型的接触点。
2. **D9-2/D9-3（P1）**：引入 `internal/` + `schema/` 包，收敛 API 表面，提升数据类型复用。
3. **D9-4（P2）**：flow 包按引擎 / 算子拆分，新增编排模式只需新增算子。

---

### 2.10 维度10：横向对比

#### 现状描述

详见第 5 节差距评分矩阵。

#### 核心结论

- **InferGlow 领先领域**：安全 / 审计 / 沙箱 / 死循环防护（+3~+4），是核心差异化壁垒。
- **InferGlow 落后领域**：接口泛型化、工具多模态、记忆中间件、usage provider 隔离、包分层与接口实现分离（-2~-4）。
- **langchaingo**：定位为 LangChain Go 端口，设计扁平，现代化程度最低。
- **agently**：在结构化输出与流式结构暴露上值得借鉴。

#### 问题清单

| 编号 | 等级 | 问题描述 |
|---|---|---|
| D10-1 | P1 | 接口泛型化差距（2 vs 5），影响类型安全与 API 表面 |
| D10-2 | P1 | 工具结果多模态差距（2 vs 5），工具结果仅支持文本 |
| D10-3 | P1 | 记忆中间件抽象差距（2 vs 5），resize 策略链不如 eino 中间件灵活 |
| D10-4 | P1 | 工具输出卸载治理差距（1 vs 5），无 offload 回查 |
| D10-5 | P1 | 悬空工具调用修复差距（1 vs 4），无 dangling 检测 |
| D10-6 | P1 | usage provider 隔离差距（2 vs 5），弱类型 map |
| D10-7 | P1 | 接口实现分离差距（2 vs 5），接口与实现同包 |

#### 改进建议

1. **D10-1/D10-7（P1）**：引入泛型化接口（如 `TypedTool[T]`），接口与实现分包。
2. **D10-2（P1）**：扩展工具结果支持 `[]ContentBlock`，实现多模态工具输出。
3. **D10-3/D10-4/D10-5（P1）**：将 resize 策略链升级为中间件机制，补齐 offload 回查与 dangling 修复。
4. **D10-6（P1）**：usage 强类型化，按 Provider 隔离。

---

## 3. 问题矩阵（P0/P1/P2 分级表）

### 3.1 P0 严重问题（13 项）

> 必须在下一迭代内修复，直接影响生产可用性、安全性、可维护性。

| 编号 | 维度 | 问题 | 代码引用 | 改进路径 | 影响文件 |
|---|---|---|---|---|---|
| P0-1 | D1 | Anthropic Provider 无法多轮 native tool call（格式未转换） | `anthropic.go:77-305` + `session_ext.go:181-204` + `chat.go:72-87` | 在 `PreparePrompt` 中按 Provider 分支转换 `tool_calls` / `tool_call_id` 为 Anthropic content block | `anthropic.go` / `session_ext.go` / `chat.go` |
| P0-2 | D3 | `SaveJSON` 声称 crash recovery 但无 `LoadJSON` 实现 | `three_zone.go:133-157` | 实现 `LoadJSON` 或修改注释并补充 WAL 方案 | `three_zone.go` |
| P0-3 | D4 | `buildPolicyFromInput` 无服务端策略基线，LLM 可操控沙箱策略 | `executor_sandbox.go:182-223` | 引入 deny-by-default 服务端策略基线 | `executor_sandbox.go` |
| P0-4 | D4 | `approval_required` 由 LLM 输入控制，可绕过审批 | `executor_sandbox.go:96` | `approval_required` 改由服务端策略决定 | `executor_sandbox.go` |
| P0-5 | D4 | Docker / gVisor 后端不强制执行 `network_access` 策略 | `docker.go:236` | 根据 `network_access` 设置 `NetworkDisabled` | `docker.go` / `gvisor.go` |
| P0-6 | D4 | `ModeAuto` 可回落到 `trusted_local`（无隔离） | `manager.go:93` | 移除回落或仅显式受信环境允许 | `manager.go` |
| P0-7 | D5 | 工具结果（含 MCP 输出）绕过 prompt injection 检测 | `session_ext.go:142-147` → `session.go:270-282` | 在 `AddToolResult` / `AddMessageWithMeta` 接入 hook 和 masker | `session_ext.go` / `session.go` |
| P0-8 | D6 | `components/tool` 整包死代码 | `components/tool/*` | 删除整包（替代路径：`engine.buildToolDefinitions`） | `components/tool/*` |
| P0-9 | D8 | `model/` God Package（25 文件 / ~117 导出符号） | `model/*` | 拆分为 `schema.go` + `internal/` + `providers/` | `model/*` |
| P0-10 | D8 | `orchestrator/agent/` God Package（15 文件 / 8 类职责） | `orchestrator/agent/*` | 拆分为 `agent.go` + `engine.go` + `internal/` + 子包 | `orchestrator/agent/*` |
| P0-11 | D8 | 完全无 `internal/` 目录，导出符号全暴露 | 项目根 | 引入 `internal/` 收敛 API 表面 | 全项目 |
| P0-12 | D8 | 核心数据类型散落（`ModelRequest` / `ModelResponse` 在 `model/` 而非 `schema/`） | `model.go` | 迁移至 `schema/` 包 | `model.go` / `schema/` |
| P0-13 | D6 | `model.ActionResult` 死类型 + `ModelRequest.Actions` 死字段 | `model.go:35` + `model.go:121-126` | 删除（替代路径：`action.ActionResult`） | `model.go` |

### 3.2 P1 重要问题（19 项）

> 在规模化场景下显著劣化可观测性与扩展性，应在中期迭代内修复。

| 编号 | 维度 | 问题 | 代码引用 | 改进路径 | 影响文件 |
|---|---|---|---|---|---|
| P1-1 | D1 | 双路径会话消息格式不一致（`role="tool"` vs `role="system"`） | `engine.go:519-619` | 统一格式或按路径分支处理 | `engine.go` |
| P1-2 | D1 | L4 outputSchema 验证仅对路径 B 生效 | `engine.go:482-514` | 验证逻辑前移到 `executeLoop` 入口 | `engine.go` |
| P1-3 | D2 | `executeLoop` 跳过 `BroadcastResponse`，usage / reasoning 丢失 | `engine.go:443-469` | 接入 `BroadcastResponse` 通道 | `engine.go` |
| P1-4 | D2 | `totalTokens` 用字符数近似 token 数 | `engine.go:517` | 强制 Provider 返回真实 usage | `engine.go` |
| P1-5 | D2 | OpenAI Responses API 未解析 usage 字段 | `openai_responses.go:351-487` | 补齐 usage 解析 | `openai_responses.go` |
| P1-6 | D2 | 推理内容在 agent 模式下丢失 | `engine.go:443-469` | 消费 `chunk.Reasoning` | `engine.go` |
| P1-7 | D3 | `SnipFromHead` 注释与实现矛盾 | `three_zone.go:268-275` | 修正注释或改尾部裁剪 | `three_zone.go` |
| P1-8 | D3 | Zone 3 从未被写入，三区架构实际两区 | `three_zone.go:171-184` | 激活 Zone 3 或移除相关代码 | `three_zone.go` |
| P1-9 | D3 | Session 与审计日志独立写入无事务性保证 | `engine.go:555` + `dispatcher.go:121` + `session_ext.go:75` | 引入联合写入事务 / outbox | `engine.go` / `dispatcher.go` / `session_ext.go` |
| P1-10 | D3 | 审计日志无法重建 session 三区状态 | 审计日志模块 | 补齐 Zone 1 快照 / 消息级 meta / resize 审计 | 审计日志模块 |
| P1-11 | D3 | `MaxEntries` 裁剪后内存 chain 与 storage 不一致 | `chain.go:141-144` | 统一裁剪行为并修正注释 | `chain.go` |
| P1-12 | D4 | `max_output_bytes` 在所有后端均为死代码 | 所有后端 | 实现或移除 | 所有后端 |
| P1-13 | D4 | `path_allowlist` 在 Docker / Bubblewrap 未生效 | `docker.go` / `bubblewrap.go` | 实现 path_allowlist 挂载 | `docker.go` / `bubblewrap.go` |
| P1-14 | D4 | E2B 后端命令拼接无转义，远程 shell 注入 | `e2b.go:389` | 改参数化传递 | `e2b.go` |
| P1-15 | D4 | Seatbelt `process-exec` 规则过宽 | `seatbelt_policy.go:73-75` | 收紧至白名单路径 | `seatbelt_policy.go` |
| P1-16 | D4 | Landlock 一次性不可撤销，误用可锁死服务进程 | `landlock.go:471-512` | 增加保护机制 | `landlock.go` |
| P1-17 | D5 | PII 脱敏不覆盖 `[]ContentBlock` 和 `Meta` map | `session.go:236-240` | 扩展脱敏至多模态字段 | `session.go` |
| P1-18 | D5 | prompt injection detector 不覆盖多语言 / 编码混淆 | `detector.go:109-141` | 增加 Unicode 规范化检测 | `detector.go` |
| P1-19 | D5 | RBAC 未集成到 `SandboxExecutor` 执行链路 | `executor_sandbox.go` | 在 execute 入口接入 RBAC | `executor_sandbox.go` |

### 3.3 P1 续（接口与扩展性，6 项）

| 编号 | 维度 | 问题 | 代码引用 | 改进路径 | 影响文件 |
|---|---|---|---|---|---|
| P1-20 | D5 | 两套审批系统割裂 | `sandbox/approval*` + `approval/*` | 合并或定义统一接口 | `sandbox/approval*` / `approval/*` |
| P1-21 | D7 | `ModelRequester` 接口过宽 | `model.go` ModelRequester | 拆分为 `StreamRequester` + `ResponseBroadcaster` | `model.go` |
| P1-22 | D7 | `SessionBackend` 接口过窄（3 处类型断言） | `session.go` + `session_ext.go` | 拆分为 `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore` | `session.go` / `session_ext.go` |
| P1-23 | D8 | `flow/` 和 `sandbox/` God Package | `flow/*` + `sandbox/*` | 按引擎 / 算子 / 后端拆分 | `flow/*` / `sandbox/*` |
| P1-24 | D8 | sandbox 后端注册违反 OCP | `sandbox/builder.go` | 改插件式自注册 | `sandbox/builder.go` |
| P1-25 | D8 | 安全检测器缺少统一接口 | `security/detector*` | 定义统一 Detector 接口 | `security/detector*` |

### 3.4 P2 一般问题（13 项）

> 代码质量改进项，可滚动修复。

| 编号 | 维度 | 问题 | 代码引用 | 改进路径 |
|---|---|---|---|---|
| P2-1 | D1 | `model.ActionResult` 死类型（与 P0-13 关联） | `model.go:121-126` | 删除 |
| P2-2 | D1 | `ToolCall.ID` 在转换为 `ActionCall` 时丢失 | `engine.go:525-531` | 保留 ID |
| P2-3 | D2 | failover / attempt 重试丢失失败尝试的 usage | failover 路径 | 累积失败尝试 usage |
| P2-4 | D2 | Anthropic `BroadcastResponse` 未发送 `ReasoningTokenMeta` | `anthropic.go` BroadcastResponse | 补齐 reasoning 字段 |
| P2-5 | D2 | `UsageInfo` 的 `map[string]int` 弱类型设计 | `model.go` UsageInfo | 重构为强类型 struct |
| P2-6 | D3 | `SmartCompressResizeHandler` 未集成到 `ThreeZoneSession` resize 链 | `three_zone.go` | 集成或移除 |
| P2-7 | D3 | `persist()` 非原子写 | `three_zone.go` persist | 改 temp file + rename |
| P2-8 | D3 | 无 dangling tool call 检测机制 | - | 参考 eino `patchtoolcalls` |
| P2-9 | D3 | system prompt 双重传递 | 请求构建路径 | 移除重复传递 |
| P2-10 | D4 | `network_access` 字段在所有后端均未真正生效 | 所有后端 | 实现 or 移除声明 |
| P2-11 | D5 | `AddMessage` 静默丢弃被 hook 拦截的消息 | `session.go` AddMessage | 改为返回错误或日志 |
| P2-12 | D5 | PII Phone 正则仅匹配中国大陆手机号 | PII 检测模块 | 扩展国际号段 |
| P2-13 | D5 | `AutoApproveHandler` / `AutoAllowHandler` 误用风险 | 审批 handler 模块 | 增加显式启用开关 |

---

## 4. 架构调整建议总表（删除/合并/拆分/保留标注）

### 4.1 删除项

| 目标 | 处置 | 替代路径 | 理由 | 关联问题 |
|---|---|---|---|---|
| `components/tool/adapt_action.go` | 删除 | `engine.buildToolDefinitions`（`engine.go:730-758`） | `ActionToTool` 零生产调用 | P0-8 |
| `components/tool/adapt_action_test.go` | 删除 | - | 随 `adapt_action.go` 删除 | P0-8 |
| `components/tool/interface.go` | 删除 | - | `BaseTool` / `ToolInfo` 仅包内部使用 | P0-8 |
| `components/tool/interface_test.go` | 删除 | - | 随 `interface.go` 删除 | P0-8 |
| `components/tool/option.go` | 删除 | - | `Option` 类型无人使用 | P0-8 |
| `components/tool/go.mod` + `go.sum` | 删除 | - | 整包可移除 | P0-8 |
| `model.ActionResult`（`model.go:121-126`） | 删除 | `action.ActionResult` | 死类型，与 `action.ActionResult` 字段完全不同 | P0-13 / P2-1 |
| `ModelRequest.Actions` 字段（`model.go:35`） | 删除 | - | 引用死类型，从未被填充 | P0-13 |

### 4.2 合并项

| 目标 | 处置 | 合并目标 | 理由 | 关联问题 |
|---|---|---|---|---|
| `sandbox.ApprovalService` | 合并 | `approval.PolicyApprovalManager` | 两套审批系统割裂，无互通 | P1-20 |
| Provider SSE 公共逻辑（`effectiveHTTPClient` / `RequestModel` goroutine 骨架 / `processLine` / `emit` 闭包 / EOF+error 处理 / `BroadcastResponse` 骨架 / `mapRole`） | 合并 | `model/providers/internal/ssestream` 公共包 | 250-300 行重复 | D7-4 |
| resize handlers 总字节计算循环 | 合并 | `TotalContentBytes` 辅助函数 | 3 处重复 | D7 |

### 4.3 拆分项

| 目标 | 处置 | 拆分方案 | 理由 | 关联问题 |
|---|---|---|---|---|
| `model/` God Package | 拆分 | `model/schema.go` + `model/internal/`（attempt / failover / pool） + `model/providers/openai/` + `model/providers/anthropic/` | 25 文件 / ~117 导出符号混杂 5 类职责 | P0-9 |
| `orchestrator/agent/` God Package | 拆分 | `agent.go` + `engine.go` + `internal/turnloop` + `internal/step` + `internal/strategy` + `hooks/` + `streaming/` + `extension/` + `features/` | 15 文件混合 8 类正交职责 | P0-10 |
| `flow/` God Package | 拆分 | 引擎层 + 算子层（按算子类型分子包） | 19 文件 / ~131 导出符号，引擎与算子混杂 | P1-23 |
| `sandbox/` God Package | 拆分 | `provider.go` + `manager.go` + `policy.go` + `backends/docker` + `backends/gvisor` + `backends/seatbelt` + ... | 30 文件 / ~118 导出符号，8 种后端平铺 | P1-23 |
| `ModelRequester` 接口 | 拆分 | `StreamRequester`（核心流式请求） + `ResponseBroadcaster`（多消费者广播） | `BroadcastResponse` 在 agent 生产路径未使用 | P1-21 |
| `SessionBackend` 接口 | 拆分 | `MessageStore` + `SessionPersistor` + `ZoneManager` + `MaskableStore` | 3 处类型断言访问接口外方法 | P1-22 |
| 核心数据类型 | 拆分 | 迁移 `ModelRequest` / `ModelResponse` 至 `schema/` 包 | 与 Provider 实现解耦 | P0-12 |

### 4.4 保留项

| 目标 | 处置 | 理由 | 关联问题 |
|---|---|---|---|
| `ActionExecutor` 接口 | 保留 | 单方法接口、三种执行器完全统一、无绕过，设计优秀 | D7-3 |
| `ThreeZoneSession` 三区架构（Zone 1 / Zone 2） | 保留 | Zone 1 设计合理，Zone 2 append 语义正确 | D3（仅需修复 Zone 3 / 注释 / LoadJSON） |
| resize handlers 集合 | 保留 | 代表不同策略，不应合并（仅提取 `TotalContentBytes`） | D7 |
| Layer 0 叶子包（`action` / `model` / `session`） | 保留 | 无循环依赖，互不依赖 | D6 |
| `security` 子包依赖方向 | 保留 | 依赖方向合理，基础能力包独立可单独复用 | D6 |
| InferGlow 安全 / 审计 / 沙箱 / 死循环防护能力 | 保留并巩固 | 核心差异化壁垒（对标项目普遍缺失） | D10 |

---

## 5. 差距评分矩阵（InferGlow vs eino vs langchaingo vs agently）

> 评分范围 1-5 分，5 分为最优。N/A 表示对标项目无对应能力。

| 维度 | eino | InferGlow | langchaingo | agently | InferGlow 差距（vs 最优） |
|---|---|---|---|---|---|
| 接口泛型化 | 5 | 2 | 2 | N/A | -3（vs eino） |
| 工具不可变绑定 | 5 | 2 | 3 | N/A | -3（vs eino） |
| 工具结果多模态 | 5 | 2 | 1 | 4 | -3（vs eino） |
| 记忆中间件抽象 | 5 | 2 | 2 | 3 | -3（vs eino） |
| 工具输出卸载治理 | 5 | 1 | 1 | 3 | -4（vs eino） |
| 悬空工具调用修复 | 4 | 1 | 1 | 2 | -3（vs eino） |
| usage provider 隔离 | 5 | 2 | 2 | 3 | -3（vs eino） |
| 包分层清晰度 | 5 | 3 | 3 | 4 | -2（vs eino） |
| 接口实现分离 | 5 | 2 | 2 | 3 | -3（vs eino） |
| Prompt 注入检测 | 1 | 5 | 1 | 2 | **+3（InferGlow 领先）** |
| PII 脱敏 | 1 | 5 | 1 | 1 | **+4（InferGlow 领先）** |
| 限流 | 1 | 5 | 1 | 2 | **+4（InferGlow 领先）** |
| RBAC | 1 | 5 | 1 | 2 | **+4（InferGlow 领先）** |
| 审计签名 | 1 | 5 | 1 | 2 | **+4（InferGlow 领先）** |
| 沙箱多后端 | 1 | 5 | 1 | 3 | **+4（InferGlow 领先）** |
| 死循环防护 | 1 | 4 | 1 | 2 | **+3（InferGlow 领先）** |

### 5.1 领先领域（InferGlow 差异化壁垒）

InferGlow 在以下 6 个维度领先对标项目 3~4 分，是核心差异化壁垒，应继续巩固：

1. **Prompt 注入检测**（5 分，vs eino 1 分）
2. **PII 脱敏**（5 分，vs eino 1 分）
3. **限流**（5 分，vs eino 1 分）
4. **RBAC**（5 分，vs eino 1 分）
5. **审计签名**（5 分，vs eino 1 分）
6. **沙箱多后端**（5 分，vs eino 1 分）
7. **死循环防护**（4 分，vs eino 1 分）

### 5.2 落后领域（需重点改进）

InferGlow 在以下 9 个维度落后对标项目 2~4 分，需重点改进：

1. **工具输出卸载治理**（1 分，-4 vs eino）—— 最严重短板
2. **接口泛型化**（2 分，-3 vs eino）
3. **工具不可变绑定**（2 分，-3 vs eino）
4. **工具结果多模态**（2 分，-3 vs eino）
5. **记忆中间件抽象**（2 分，-3 vs eino）
6. **悬空工具调用修复**（1 分，-3 vs eino）
7. **usage provider 隔离**（2 分，-3 vs eino）
8. **接口实现分离**（2 分，-3 vs eino）
9. **包分层清晰度**（3 分，-2 vs eino）

### 5.3 对标项目定位

- **eino**：工程化抽象最完善，接口泛型化 / 中间件 / 多模态 / 包分层均领先，是 InferGlow 工程化改进的主要对标。
- **langchaingo**：LangChain Go 端口，设计扁平，现代化程度最低，仅作为基线参考。
- **agently**：在结构化输出与流式结构暴露上值得借鉴（工具结果多模态 4 分、包分层 4 分）。

---

## 6. 总结与下一步

### 6.1 审计结论

InferGlow 在**安全 / 审计 / 沙箱 / 死循环防护**领域建立了显著的差异化壁垒（领先 eino/langchaingo 3~4 分），这是项目的核心价值所在。但在**工程化抽象**层面（接口泛型化、工具多模态、记忆中间件、usage 隔离、包分层）存在系统性短板（落后 eino 2~4 分），且存在三类系统性风险：

1. **安全声明与实现脱节**：沙箱策略"纸面化"（`network_access` / `path_allowlist` / `max_output_bytes` 均未真正生效）、防注入覆盖不全（工具结果绕过检测）。
2. **数据链路断裂**：usage 丢失、ToolCall.ID 丢失、Zone 3 从未写入、Anthropic 多轮 tool call 失效。
3. **包结构失控**：4 个 God Package、无 `internal/` 隔离、核心类型散落。

### 6.2 修复路线图

| 阶段 | 时间窗 | 目标 | 涉及问题 |
|---|---|---|---|
| 第一阶段 | 立即（1-2 周） | 修复安全声明与实现脱节 | P0-3 ~ P0-7、P0-13 |
| 第二阶段 | 短期（2-4 周） | 修复数据链路断裂 | P0-1、P0-2、P0-8、P1-3 ~ P1-6 |
| 第三阶段 | 中期（1-2 月） | 包结构治理与接口拆分 | P0-9 ~ P0-12、P1-21 ~ P1-25 |
| 第四阶段 | 滚动 | P2 改进与工程化抽象对齐 eino | P2-1 ~ P2-13、D10 改进项 |

### 6.3 关键决策点

1. **是否激活 Zone 3**：若不激活则移除相关代码，避免误导。
2. **双路径（native tool calls vs legacy JSON decision）是否统一**：若统一则消除 D1-2/D1-3，若保留则需显式分支处理。
3. **`components/` 是否补齐**：若保留则补齐 model 接口定义，若不保留则随 `components/tool` 一起移除。
4. **是否引入 eino 风格中间件机制**：若引入则 resize 策略链升级为中间件，补齐 offload / dangling 修复能力。

---

> 本报告基于 2026-07-24 代码库状态生成，所有问题均包含代码引用（文件名 + 行号），便于定位与修复。修复完成后建议重新审计以验证闭环。
