# model - LLM Provider 统一抽象层

**模块路径**: `github.com/inferglow/model`

## 概述

model 模块是 inferglow 最底层的基础设施，提供统一的 LLM Provider 抽象层。它屏蔽了不同模型供应商（OpenAI、Anthropic、Ollama 等）的 API 差异，通过统一的接口发送请求并接收响应（含流式输出）。

## 设计定位

- **被谁依赖**: `schema`、`session` 模块
- **依赖谁**: 无（仅依赖 stdlib + `gopkg.in/yaml.v3`）
- **对标 Python**: `agently/core/model/` 下的 ModelProvider 抽象
- **独立可用性**: 可独立使用，不依赖 inferglow 其他模块

## 核心类型

### ModelRequest - 模型请求

```go
type ModelRequest struct {
    System       string          // 系统提示词
    Developer    string          // 开发者提示词
    Instruct     string          // 用户指令
    Input        string          // 用户输入
    OutputFormat string          // 输出格式
    ChatHistory  []ChatMessage   // 对话历史
    Tools        []ToolDefinition // 工具定义
    Output       *OutputSchema   // 结构化输出契约（可选）
    Model        string          // 模型名称
    Temperature  float64         // 温度参数
}
```

### ModelResponse - 模型响应

```go
type ModelResponse struct {
    Content   string      // 回复内容
    Reasoning string      // 推理过程（如 Claude）
    Tools     []ToolCall  // 工具调用
    Usage     UsageInfo   // Token 用量
    Meta      map[string]any
}
```

### StreamChunk - 流式传输块

```go
type StreamChunk struct {
    Delta     string      // 增量内容
    Reasoning string      // 推理增量
    Tools     []ToolCall  // 工具调用
    IsDone    bool        // 是否完成
    Usage     *UsageInfo  // 用量信息
}
```

### ModelRequester - 统一接口

```go
type ModelRequester interface {
    Name() string
    GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error)
    RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error)
    BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error)
}
```

## Provider 实现

| Provider | 文件 | 说明 |
|----------|------|------|
| OpenAICompatibleProvider | openai.go | 兼容 OpenAI API 的所有 Provider（OpenAI、DeepSeek、Qwen、Ollama 等） |
| AnthropicCompatibleProvider | anthropic.go | Anthropic Claude 系列模型 |
| OllamaProvider | ollama.go | Ollama 本地模型 |

## 配置系统

通过 `ConfigProvider` 接口抽象配置源，支持三种实现：

```go
// 环境变量: INFERGLOW_OPENAI_API_KEY=mykey
EnvConfigProvider

// YAML/JSON 文件
FileConfigProvider

// 静态 map（测试用）
StaticConfigProvider
```

默认配置在 `DEFAULT_SETTINGS` 中定义，包含各 Provider 的默认参数。

## 重试与验证

- `AttemptRunner` - 带指数退避的重试控制器
- `OutputValidator` - 输出验证器，支持自动重试校验

## 核心接口一览

```
ModelRequest          → 请求参数
ModelResponse         → 响应结果
StreamChunk           → 流式传输块
ChatMessage           → 聊天消息
ToolDefinition/ToolCall → 工具定义与调用
OutputSchema          → 结构化输出契约（与 model 包复用）
ModelRequester        → 统一请求接口
ModelRequesterRegistry → Provider 注册中心
ConfigProvider        → 配置源抽象
```

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── Session → 准备 ChatMessage → ModelRequest
  ├── Schema → 生成 OutputSchema → ModelRequest.Output
  ├── Action → 生成 ToolDefinition → ModelRequest.Tools
  └── ModelProvider → ModelRequester → 实际调用 LLM
```
