# example_model - 模型模块 / Model Module

## 概述 / Overview

本示例演示如何使用 `model` 模块构造 LLM Provider、配置重试机制、执行 Schema 校验，以及处理流式推理内容。涵盖 OpenAI、Anthropic、Ollama 和 OpenAI Responses 四种 Provider 的构造方式，以及错误分类、指数退避重试、输出校验和思维链内容分离等高级功能。

This example demonstrates how to use the `model` module to construct LLM Providers, configure retry mechanisms, perform schema validation, and handle streaming reasoning content. It covers the construction of OpenAI, Anthropic, Ollama, and OpenAI Responses providers, along with advanced features such as error classification, exponential backoff retry, output validation, and chain-of-thought content separation.

## 核心概念 / Core Concepts

- **Provider / 提供者**: 抽象 LLM API 的接口，支持 OpenAI、Anthropic、Ollama、OpenAI Responses 等
- **ModelRequest / 模型请求**: 统一的请求结构，包含 System Prompt、ChatHistory、Tools、OutputSchema 等
- **GenerateRequestData / 请求数据转换**: 将 ModelRequest 转换为 Provider 特定的请求体结构
- **ClassifyError / 错误分类**: 将 API 错误分为 Fatal（401/403）、BackoffRetry（429）、Retry（5xx）等类别
- **AttemptRunner / 重试运行器**: 支持指数退避的重试机制，Fatal 错误跳过重试
- **OutputValidator / 输出校验器**: 基于 JSON Schema 对 LLM 输出进行校验，支持重试拉取
- **LeadingThinkNormalizer / 思维链分离器**: 从流式输出中分离 reasoning 和 answer 内容
- **content_mapping / 内容映射**: 从非标 SSE JSON 路径提取 delta/reasoning 内容
- **full_url / 完整 URL 覆盖**: 完全覆盖 base_url 的默认路径拼接

## 前置条件 / Prerequisites

- Go 1.21+
- 本示例不发起真实网络请求，所有 Provider 仅构造和展示配置
- This example does not make real network requests; all providers are constructed and configured only for demonstration

## 使用示例 / Usage Example

代码演示了以下 8 个场景：

1. **配置加载与 Provider 构造**: 使用 `StaticConfigProvider` 配置 OpenAI、Anthropic、Ollama 三个 Provider，并展示各自的 Name、BaseURL 和 Model。
2. **ModelRequest 与 GenerateRequestData**: 构造包含 System Prompt、对话历史、工具定义和输出 Schema 的请求，并转换为 OpenAI 格式的请求体。
3. **错误分类**: 演示 `ClassifyError` 对 401、429、500、connection refused 等错误的分类结果。
4. **AttemptRunner 重试配置**: 展示 `NewAttemptRunner` 的默认配置（MaxAttempts、BackoffBase、BackoffMax），说明指数退避策略和 Fatal 错误跳过逻辑。
5. **OutputValidator 校验**: 基于 JSON Schema 构造校验器，展示校验失败时通过 ResponseFetcher 重新拉取 LLM 响应的机制。
6. **full_url 与 content_mapping**: 演示 `full_url` 完全覆盖默认路径，以及 `content_mapping` 从非标 SSE JSON 路径提取 delta 和 reasoning 内容。
7. **OpenAIResponsesProvider 构造**: 展示使用 `/responses` 端点的 Provider 构造方式。
8. **LeadingThinkNormalizer 流式 thinking 分离**: 演示 `FeedDelta` 在流式场景中分离 reasoning 和 answer，以及 `FeedDone` 在非流式场景中的一次性提取。

The code demonstrates 8 scenarios:

1. **Config Loading and Provider Construction**: Configure OpenAI, Anthropic, and Ollama providers using `StaticConfigProvider`, showing their Name, BaseURL, and Model.
2. **ModelRequest and GenerateRequestData**: Construct a request with System Prompt, chat history, tool definitions, and output schema, then convert it to the OpenAI request body format.
3. **Error Classification**: Show `ClassifyError` results for 401, 429, 500, and connection refused errors.
4. **AttemptRunner Retry Configuration**: Display the default configuration of `NewAttemptRunner` (MaxAttempts, BackoffBase, BackoffMax), explaining the exponential backoff strategy and Fatal error skip logic.
5. **OutputValidator Validation**: Build a validator based on JSON Schema, showing the mechanism to re-fetch LLM responses via ResponseFetcher on validation failure.
6. **full_url and content_mapping**: Demonstrate `full_url` overriding the default path, and `content_mapping` extracting delta and reasoning from non-standard SSE JSON paths.
7. **OpenAIResponsesProvider Construction**: Show the provider construction using the `/responses` endpoint.
8. **LeadingThinkNormalizer Streaming Thinking Separation**: Demonstrate `FeedDelta` separating reasoning and answer in streaming scenarios, and `FeedDone` for one-shot extraction in non-streaming scenarios.

## 运行验证 / Running the Example

```
cd examples
go run example_model.go
```

预期输出会依次展示：

- 三个 Provider 的名称、BaseURL 和模型名
- ModelRequest 的各字段内容及转换后的 RequestData 结构
- 四种错误类型的分类结果
- AttemptRunner 的默认配置参数
- OutputValidator 的 Schema 配置和重试次数
- full_url 覆盖后的最终 URL，以及 content_mapping 的路径提取结果
- OpenAIResponsesProvider 的构造信息
- 流式和非流式场景下 reasoning 与 answer 的分离结果

Expected output shows:

- Name, BaseURL, and Model for three providers
- ModelRequest fields and the converted RequestData structure
- Classification results for four error types
- Default configuration parameters of AttemptRunner
- OutputValidator schema configuration and retry count
- The resolved URL after full_url override, and content_mapping path extraction results
- OpenAIResponsesProvider construction info
- Reasoning and answer separation results in both streaming and non-streaming scenarios

## 预期输出 / Expected Output

输出着重展示 model 模块的 Provider 抽象能力和辅助工具链：统一的请求模型屏蔽了不同 LLM API 的差异，错误分类和重试机制提供了健壮的调用保障，输出校验确保了 LLM 响应的结构化合规性，思维链分离器则便于消费推理过程。

The output highlights the model module's provider abstraction and utility toolchain: the unified request model shields differences between various LLM APIs, error classification and retry mechanisms provide robust invocation guarantees, output validation ensures structural compliance of LLM responses, and the chain-of-thought normalizer facilitates consuming reasoning processes.