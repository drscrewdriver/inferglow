# Responses API 支持 Spec（proxy-compressor）

## Why

proxy-compressor 目前只代理 `POST /v1/chat/completions`、`/v1/embeddings`、`/v1/completions`、`GET /v1/models`，**对 OpenAI 官方推荐的 `/v1/responses` 完全不透明**：Responses 客户端经代理时流量原样透传、无审计。而 Responses 已是 OpenAI 官方指定给新 agent 项目的协议（内置工具、服务端状态、`previous_response_id` 串联）。不支持的后果是：审计覆盖出现"协议盲区"，无法追溯这类最重要的新项目流量。因此需要规划支持，让代理对所有 OpenAI 协议端点透明、可审计、可压缩。

## 核心概念澄清（回应三个待确认问题）

### 1. Responses 以什么为核心？
以 **`response` 对象 + `items`** 为核心，**不是 messages**：
- 输入字段是 `input`（一个 `string` 或 items 数组），而非 `messages`。
- 输出字段是 `output`（一个 **items 数组**），item 是类型联合：`message`、`function_call`、`function_call_output`、`reasoning`、`web_search_call` 等。去掉了 `n`（不再并行 choices）。
- 因此现有审计解析（`choices[0].message.content`、`choices[0].delta.content`）**全部失效**，需重写为"从 output items 提取 message 文本 / 从事件流聚合 output_text"。

### 2. 附属机制（session 恢复）是否存在？
**存在，且是核心特性**，共三层：
- **`previous_response_id`**：把上一轮的 response id 传给当前请求，服务端自动补全历史上下文，客户端每轮只发**增量 input**。
- **`store: true`**（默认开启）：response 持久化在 OpenAI 服务端，**默认保留 30 天**（不可配置，可显式删除）。
- **Conversations API**：可选的持久会话对象（`conversations.create`），跨会话/设备复用，作为 `previous_response_id` 串联的超集。

> **对代理的关键影响**：服务端状态意味着**出向请求通常是增量 input（本 turn 只有新 items），本地看不到完整历史**。现有的 `compute_session_id`（从第一条 user message 哈希推导）在 Responses 下**不可靠**——必须改用 `previous_response_id` / `conversation_id` 作为多轮关联键。

### 3. py/go、Chat、出向/入向、本地记录优先级复杂度是否上升？
**是，上升但可控**，体现在四个层面：

| 层面 | 复杂度上升点 | 可控性 |
|---|---|---|
| **出向（转发）** | 新增 `/v1/responses` 代理路径；双语言（py+go）各一份 | 低——与 chat 转发结构同构，可复用通用 `_forward`/`forwardRequest` |
| **入向（响应解析/审计）** | 流式是**事件流**（`response.created` / `response.output_text.delta` / `response.completed` / `response.failed`），非 SSE `choices/delta`；content 重建与 usage 提取逻辑不同 | 中——需新增 responses 专用重建器 |
| **本地记录** | usage 字段为 `input_tokens`/`output_tokens`（可能含 `prompt_tokens_details.cached_tokens`）；`total_tokens` 语义需映射 | 中——需新增字段映射 |
| **优先级处理** | 审计"永远先于校验、不阻塞、全记录"原则**保持**；但"全记录"语义需**重定义**（服务端状态→本 turn 只存增量） | 关键——需明确边界 |

**结论**：核心原则不变，但"全记录"的语义从"记录完整 messages 往返"变为"记录本 turn 的增量 input + 完整 output"，并新增 protocol 维度。

## What Changes

- **协议层**：py 与 go 各新增 `POST /v1/responses` 代理处理，支持流式（事件流）与非流式；`/v1/responses` 透传 `store`、`previous_response_id`、`instructions`、`tools` 等字段（不解释、原样转发）。
- **解析层**：新增 Responses 专用解析器——从 `input`/`output` items 提取 `model`、`usage`（input/output tokens）、重建 `output_text`；流式通过聚合 `response.output_text.delta` 事件重建，检测 `response.completed`/`response.failed` 判定完成。
- **审计模型**：审计记录新增 `protocol` 维度（`chat`/`responses`）；usage 字段映射 `input_tokens→prompt_tokens`、`output_tokens→completion_tokens`（缺失则估算）；新增可选 `response_id`、`previous_response_id`、`conversation_id` 字段。
- **session 关联**：`compute_session_id` 扩展——优先用 `conversation_id`，其次 `previous_response_id`（哈希），fallback 回第一条 user message（保留 Chat 行为）。
- **schema 校验**：新增 `ResponsesRequest` Pydantic 模型（py）/ validator 结构体（go）：`model` 必填、`input` 非空（string 或 items 数组）、`store`/`previous_response_id` 类型校验。**校验失败仍记录审计**（审计先于校验）。
- **查询/Dashboard**：列表与筛选支持按 `protocol` 过滤（可选增强）。

## Impact

- 修改代码：
  - `py/src/proxy/schema.py`、`py/src/proxy/handler.py`
  - `py/src/audit/storage.py`（session 计算扩展）
  - `go/internal/proxy/proxy.go`、`go/internal/proxy/validate.go`
  - `go/internal/audit/models.go`（protocol/response_id 字段）
- 受影响 spec：`proxy-compressor-auditor`（在其基础上扩展，不重构既有 Chat 路径）
- **BREAKING**：无——既有 `/v1/chat/completions` 等路径保持行为不变，Responses 为纯新增分支。

---

## ADDED Requirements

### Requirement: Responses 端点代理
系统 SHALL 代理 `POST /v1/responses`，支持流式与非流式，并保持审计优先原则。

#### Scenario: 非流式 Responses
- **WHEN** 客户端 `POST /v1/responses`（非 stream）
- **THEN** 系统转发到上游
- **AND** 返回完整 `response` 对象给客户端
- **AND** 记录审计（含 model、usage、重建 output_text）

#### Scenario: 流式 Responses（事件流）
- **WHEN** 客户端 `POST /v1/responses` 且 `stream: true`
- **THEN** 系统实时转发每个事件到客户端
- **AND** 聚合 `response.output_text.delta` 重建完整 output_text
- **AND** 以 `response.completed`/`response.failed` 判定流完整性
- **AND** 记录审计（含重建 output_text）

#### Scenario: 校验失败仍审计
- **WHEN** Responses 请求体非法（如缺 `model` 或 `input` 为空）
- **THEN** 仍记录审计日志（`response_status` 标记 4xx），再返回错误——审计先于校验

### Requirement: Responses 审计字段提取
系统 SHALL 从 Responses 请求/响应提取 model、usage 与 output 文本。

#### Scenario: usage 映射
- **WHEN** 上游响应含 `usage.input_tokens` / `usage.output_tokens`
- **THEN** 映射到 `prompt_tokens` / `completion_tokens` 记录
- **WHEN** 缺失
- **THEN** 按字符/4 估算

#### Scenario: protocol 标记
- **WHEN** 记录任一条 Responses 请求
- **THEN** `protocol` 字段记为 `responses`（Chat 请求记为 `chat`）

### Requirement: session 关联
系统 SHALL 用 conversation_id / previous_response_id 关联 Responses 多轮。

#### Scenario: 关联键优先级
- **WHEN** 请求含 `conversation_id`
- **THEN** session_id 由它派生
- **WHEN** 无 conversation_id 但含 `previous_response_id`
- **THEN** session_id 由它哈希派生
- **WHEN** 两者皆无
- **THEN** fallback 回第一条 user message 哈希（与 Chat 一致）

### Requirement: Responses schema 校验
系统 SHALL 校验 Responses 入站请求。

#### Scenario: 校验规则
- **WHEN** `model` 缺失或 `input` 为空
- **THEN** 返回 400 结构化错误
- **WHEN** `store`/`previous_response_id` 类型非法
- **THEN** 返回 400

## MODIFIED Requirements

### Requirement: 审计优先原则（扩展）
原"所有请求都记录"原则 SHALL 扩展到 `/v1/responses`。
**Reason**: Responses 的服务端状态导致出向为增量 input，本地只存本 turn。
**细化**: 对本 turn 的完整 input（增量）+ 完整 output 做记录；历史上下文由服务端持有，不要求本地重建完整对话历史。

## REMOVED Requirements

无。
