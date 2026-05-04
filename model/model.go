package model

import "context"

// ModelRequest 模型请求参数
type ModelRequest struct {
	System        string
	Developer     string
	Instruct      string
	Input         string
	OutputFormat  string
	ChatHistory   []ChatMessage
	Info          map[string]any
	Tools         []ToolDefinition
	Actions       []ActionResult
	Examples      []Example
	Attachment    []Attachment
	Output        *OutputSchema
	EnsureAll     bool
	Options       map[string]any
	Model         string
	Temperature   float64
	// TemperatureSet indicates that the caller explicitly set Temperature (even
	// to 0). When false (zero value), providers may apply their own default.
	// This allows callers to request deterministic temperature=0 without it
	// being silently overridden by the provider default.
	TemperatureSet bool
	// ToolChoice controls tool selection behavior. May be a string
	// ("auto"/"none"/"required") or a structured object
	// ({"type":"function","function":{"name":"..."}} for OpenAI,
	// {"type":"auto"|"any"|"tool","name":...} for Anthropic).
	// M-MEDIUM-1: pass through to provider request body.
	ToolChoice any
}

// ModelResponse 模型响应结果
type ModelResponse struct {
	Content   string
	Reasoning string
	Tools     []ToolCall
	Usage     UsageInfo
	Meta      map[string]any
}

// StreamChunk SSE 流式传输中的一块数据
type StreamChunk struct {
	Delta     string
	Reasoning string
	Tools     []ToolCall
	IsDone    bool
	Usage     *UsageInfo
	Meta      map[string]any
}

// RequestData 发送给 Provider 的请求数据
type RequestData struct {
	Model       string
	Messages    []ChatMessage
	Tools       []ToolDefinition
	Temperature float64
	MaxTokens   int
	Options     map[string]any
	// ToolChoice is the provider-agnostic form of ModelRequest.ToolChoice;
	// providers copy it into their request body when non-nil.
	// M-MEDIUM-1: tool_choice support.
	ToolChoice any
	// Output is the optional structured-output schema. Providers use it to
	// enable JSON mode (e.g. Ollama format=json, OpenAI response_format,
	// Anthropic system-prompt instruction).
	// M-MEDIUM-9: provider handling of Options/Output.
	Output *OutputSchema
}

// OutputSchema 定义期望的输出格式
type OutputSchema struct {
	Type       string            `json:"type"`
	Properties map[string]any    `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
}

// Example 示例
type Example struct {
	Input  string
	Output string
}

// Attachment 附件
type Attachment struct {
	Type string
	Data any
}

// ActionResult 动作执行结果
type ActionResult struct {
	Name    string
	Success bool
	Output  any
	Error   string
}

// ModelRequester 统一模型请求接口
type ModelRequester interface {
	// Name 返回 Provider 名称
	Name() string
	// GenerateRequestData 将 ModelRequest 转换为 Provider 特定格式
	GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error)
	// RequestModel 发送请求并返回流式 channel
	RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error)
	// BroadcastResponse 将 stream 广播为 ResultEvent channel
	BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error)
}
