# 非标准 OpenAI 兼容 Provider 适配 Spec

## Why

`inferglow/docs/non-standard-providers.md` 调研了 4 家具有非标准特征的国产 Provider（阶跃星辰、百度千帆、讯飞星火、商汤日日新），并给出了完整的适配方案。当前 `model/config.go` 的 `DEFAULT_SETTINGS` 仅覆盖 7 家标准 Provider，`model/provider_factory.go` 也只有对应的 5 个 factory 函数。用户需通过文档中描述的方案把这 4 家非标准 Provider 接入 InferGlow，让用户改配置文件即可使用。

文档明确指出："全部使用现有 `OpenAICompatibleProvider` / `AnthropicCompatibleProvider`"，特殊路径（`/v2`、`/agent/v1/`、`/step_plan`）、不同域名、API Key 格式差异都通过 `base_url` 配置解决，**不需要新增 Provider 类型**。

但文档同时明确指出，本次新增的 4 家非标准 Provider 中，有 3 家（讯飞星火 X2-Flash、商汤 SenseNova、以及 MiMo）使用 `reasoning_content` 字段而非标准的 `reasoning` 字段返回推理内容（文档"推理内容字段映射表"与"方法论 D：推理字段统一映射"）。文档"方法论 D"给出了 `processOpenAILine` 中的具体实现代码：SSE 解析时同时读取 `reasoning` 与 `reasoning_content`，`reasoning_content` 优先级高于 `reasoning`。

此外，文档"G1-04 `<think>` tag 归一化"与"方法论 D"指出，部分 Provider/代理在 `content` 字段中返回 `<think>...</think>` 标签包裹的推理内容（DeepSeek-R1/V4 非流式场景、商汤原生接口等）。虽然 InferGlow 当前为流式-only，但文档明确建议在 `BroadcastResponse` 输出前对累积内容做 `<think>` 标签归一化作为防御性处理。文档给出 `normalizeThinkingTags()` 函数的实现指导。

本次 spec 因此包含三部分：(1) 配置层 + factory 适配（已完成）；(2) `reasoning_content` 字段解析（G1-02）；(3) `<think>` 标签归一化（G1-04）。G1-03（`reasoning_effort`/`thinking.type` 参数透传）属于请求参数层，文档标注为独立任务，不在本次范围。

## What Changes

* 在 `model/config.go` 的 `DEFAULT_SETTINGS` 中新增 6 个 Provider 配置项：
  * `stepfun`（OpenAI 兼容，`https://api.stepfun.com/v1`）
  * `stepfun_anthropic`（Anthropic 兼容，`https://api.stepfun.com/step_plan`）
  * `baidu`（OpenAI 兼容，`https://qianfan.baidubce.com/v2` — `/v2` 路径）
  * `spark`（OpenAI 兼容，`https://spark-api-open.xf-yun.com/agent/v1/` — `/agent/v1/` 前缀）
  * `sensenova`（OpenAI 兼容，`https://token.sensenova.cn/v1` — Token Plan 域名）
  * `sensenova_anthropic`（Anthropic 兼容，`https://token.sensenova.cn/v1/messages`）

* 在 `model/provider_factory.go` 中新增 6 个 factory 函数：
  * `NewStepFunProviderFromConfig` → `OpenAICompatibleProvider`（`ProviderName: "stepfun"`）
  * `NewStepFunAnthropicProviderFromConfig` → `AnthropicCompatibleProvider`
  * `NewBaiduProviderFromConfig` → `OpenAICompatibleProvider`（`ProviderName: "baidu"`）
  * `NewSparkProviderFromConfig` → `OpenAICompatibleProvider`（`ProviderName: "spark"`）
  * `NewSenseNovaProviderFromConfig` → `OpenAICompatibleProvider`（`ProviderName: "sensenova"`）
  * `NewSenseNovaAnthropicProviderFromConfig` → `AnthropicCompatibleProvider`

* 在 `model/provider_factory_test.go`（或同包新增测试文件）中为每个 factory 增加单元测试，验证 `LoadProviderConfig` 正确加载配置且 factory 返回的 Provider 字段（`BaseURL`/`APIKey`/`Model`/`ProviderName`）符合预期。

* 不修改 `OpenAICompatibleProvider` / `AnthropicCompatibleProvider` 的实现代码（文档明确不需要）。

## Impact

* Affected specs: `inferglow/docs/non-standard-providers.md`（实现文档中描述的适配方案）、`inferglow/docs/tasks-20260721-goal-1.md` G1-01 第二批次
* Affected code:
  * `model/config.go` — `DEFAULT_SETTINGS` map 新增 6 个 key
  * `model/provider_factory.go` — 新增 6 个 factory 函数
  * `model/provider_factory_test.go`（或新测试文件）— 新增 6 组测试
* 无 **BREAKING** 变更：所有新增均为追加，不影响现有 7 家 Provider 的行为与 API。

## ADDED Requirements

### Requirement: 4 家非标准 Provider 配置层接入

系统 SHALL 在 `DEFAULT_SETTINGS` 中为阶跃星辰、百度千帆、讯飞星火、商汤日日新 4 家非标准 Provider 提供默认配置，让用户通过环境变量或配置文件覆盖即可接入。

#### Scenario: 阶跃星辰双协议配置

* **WHEN** 检查 `DEFAULT_SETTINGS["stepfun"]` 与 `DEFAULT_SETTINGS["stepfun_anthropic"]`
* **THEN** `stepfun` 的 `base_url` 为 `https://api.stepfun.com/v1`，`model` 为 `step-3.7-flash`
* **AND** `stepfun_anthropic` 的 `base_url` 为 `https://api.stepfun.com/step_plan`，`max_tokens` 为 1024（Anthropic 端点上限较低）

#### Scenario: 百度千帆 /v2 路径配置

* **WHEN** 检查 `DEFAULT_SETTINGS["baidu"]`
* **THEN** `base_url` 为 `https://qianfan.baidubce.com/v2`（注意是 `/v2` 而非 `/v1`）
* **AND** `model` 为 `ernie-5.0`

#### Scenario: 讯飞星火 /agent/v1/ 路径配置

* **WHEN** 检查 `DEFAULT_SETTINGS["spark"]`
* **THEN** `base_url` 为 `https://spark-api-open.xf-yun.com/agent/v1/`（注意含 `/agent/v1/` 前缀与尾斜杠）
* **AND** `model` 为 `spark-x`

#### Scenario: 商汤双协议配置

* **WHEN** 检查 `DEFAULT_SETTINGS["sensenova"]` 与 `DEFAULT_SETTINGS["sensenova_anthropic"]`
* **THEN** `sensenova` 的 `base_url` 为 `https://token.sensenova.cn/v1`（Token Plan 域名）
* **AND** `sensenova_anthropic` 的 `base_url` 为 `https://token.sensenova.cn/v1/messages`，`max_tokens` 为 1024

### Requirement: Provider Factory 函数

系统 SHALL 在 `provider_factory.go` 中提供 6 个 factory 函数，分别构造 4 家非标准 Provider 的 OpenAI 兼容与 Anthropic 兼容实例。

#### Scenario: OpenAI 兼容 factory 设置正确的 ProviderName

* **WHEN** 调用 `NewStepFunProviderFromConfig(cp)` / `NewBaiduProviderFromConfig(cp)` / `NewSparkProviderFromConfig(cp)` / `NewSenseNovaProviderFromConfig(cp)`
* **THEN** 返回的 `*OpenAICompatibleProvider` 的 `Name()` 分别返回 `"stepfun"` / `"baidu"` / `"spark"` / `"sensenova"`
* **AND** `BaseURL`、`APIKey`、`Model` 字段从 `LoadProviderConfig` 加载（覆盖默认值）

#### Scenario: Anthropic 兼容 factory 构造正确实例

* **WHEN** 调用 `NewStepFunAnthropicProviderFromConfig(cp)` 与 `NewSenseNovaAnthropicProviderFromConfig(cp)`
* **THEN** 返回 `*AnthropicCompatibleProvider`，`BaseURL`/`APIKey`/`Model` 从对应 `stepfun_anthropic` / `sensenova_anthropic` 配置加载

#### Scenario: API Key 缺失时返回包装错误

* **WHEN** 调用任一 factory 且 `ConfigProvider` 未提供对应 `<prefix>.api_key`
* **THEN** 返回的错误 `errors.Is(err, ErrMissingRequiredConfig)` 为 true
* **AND** 错误信息包含对应的 prefix（如 `"stepfun.api_key is required"`）

### Requirement: Factory 单元测试覆盖

系统 SHALL 为 6 个新 factory 提供单元测试，验证配置加载与字段映射正确。

#### Scenario: 每个 factory 都有正向测试

* **WHEN** 执行 `go test ./model/ -run "NewStepFun|NewBaidu|NewSpark|NewSenseNova"`
* **THEN** 所有测试通过，覆盖：默认配置加载、环境变量覆盖 base_url/model、APIKey 缺失错误

#### Scenario: 不修改 Provider 核心实现

* **WHEN** 检查 `openai.go` 与 `anthropic.go` 的 git diff
* **THEN** config.go 与 provider_factory.go 的改动仅为配置与 factory 追加；openai.go 的改动仅限 reasoning_content 解析与 think 标签归一化（见下方两个 Requirement）

### Requirement: `reasoning_content` 字段解析（G1-02）

系统 SHALL 在 `OpenAICompatibleProvider` 的 SSE 解析中同时读取 `reasoning` 与 `reasoning_content` 两个字段，`reasoning_content` 优先级高于 `reasoning`，统一映射到 `StreamChunk.Reasoning`。文档"方法论 D"与"推理内容字段映射表"明确要求此适配以支持 MiMo、讯飞星火 X2-Flash、商汤 SenseNova。

#### Scenario: openAIChunk 结构包含 reasoning_content 字段

* **WHEN** 检查 `openAIChunk.Delta` 结构体定义
* **THEN** 包含 `ReasoningContent *string json:"reasoning_content,omitempty"` 字段

#### Scenario: processOpenAILine 优先读取 reasoning_content

* **WHEN** SSE chunk 的 `delta` 同时包含 `reasoning` 与 `reasoning_content`
* **THEN** `StreamChunk.Reasoning` 取 `reasoning_content` 的值（优先级高）
* **AND** 当只有 `reasoning` 时，`StreamChunk.Reasoning` 取 `reasoning` 的值（向后兼容）

#### Scenario: 仅含 reasoning_content 的 chunk 正确解析

* **WHEN** SSE chunk 的 `delta` 仅包含 `reasoning_content`（如 `{"delta":{"reasoning_content":"思考..."}}`）
* **THEN** `StreamChunk.Reasoning` 为 "思考..."
* **AND** `StreamChunk.Delta` 为空

#### Scenario: toStreamChunk 向后兼容函数同步更新

* **WHEN** 调用 `toStreamChunk` 解析含 `reasoning_content` 的 chunk
* **THEN** 返回的 `StreamChunk.Reasoning` 正确反映 `reasoning_content` 值

### Requirement: `<think>` 标签归一化（G1-04）

系统 SHALL 提供 `normalizeThinkingTags` 与 `hasThinkingTags` 函数，在 `BroadcastResponse` 输出最终 `ModelResponse` 前对累积的 `Content` 做防御性归一化：若 `Reasoning` 为空且 `Content` 含 `<think>...</think>` 标签，提取标签内容到 `Reasoning` 并从 `Content` 中清除。文档"G1-04"与"方法论 D"明确要求此适配以兼容 DeepSeek-R1/V4 非流式场景与部分代理返回的标签包裹内容。

#### Scenario: hasThinkingTags 检测标签存在

* **WHEN** 调用 `hasThinkingTags("<think>推理</think> 回答")`
* **THEN** 返回 true
* **WHEN** 调用 `hasThinkingTags("纯文本回答")`
* **THEN** 返回 false

#### Scenario: normalizeThinkingTags 提取并清除标签

* **WHEN** 调用 `normalizeThinkingTags("<think>推理过程</think>最终回答")`
* **THEN** 返回 `reasoning="推理过程"`，`cleaned="最终回答"`（去除前导空白）
* **WHEN** 调用 `normalizeThinkingTags("无标签内容")`
* **THEN** 返回 `reasoning=""`，`cleaned="无标签内容"`

#### Scenario: BroadcastResponse 对最终响应做归一化

* **WHEN** 流式累积的 `fullContent` 含 `<think>...</think>` 且 `fullReasoning` 为空
* **THEN** 最终 `ModelResponse.Reasoning` 为标签内内容
* **AND** `ModelResponse.Content` 为清除标签后的内容
* **WHEN** `fullReasoning` 非空（标准 reasoning 字段已填充）
* **THEN** 不触发归一化，保持原样（避免双重处理）
