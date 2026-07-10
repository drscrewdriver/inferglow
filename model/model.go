// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import "context"

// ModelRequest 模型请求参数
type ModelRequest struct { //nolint:revive
	System       string
	Developer    string
	Instruct     string
	Input        string
	OutputFormat string
	ChatHistory  []ChatMessage
	Info         map[string]any
	Tools        []ToolDefinition
	Examples     []Example
	Attachment   []Attachment
	// ContentBlocks carries multimodal content (images, audio, etc.).
	// When non-empty, providers should convert these into their native
	// content block format (e.g. OpenAI image_url, Anthropic image source).
	ContentBlocks []ContentBlock
	Output       *OutputSchema
	EnsureAll    bool
	Options      map[string]any
	Model        string
	Temperature  float64
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
type ModelResponse struct { //nolint:revive
	Content   string
	Reasoning string
	Tools     []ToolCall
	Usage     UsageInfo
	Meta      map[string]any
	// ReasoningTruncated 表示推理内容因达到 MaxReasoningTokens 预算而被截断。
	// false 表示未截断或未设置预算。G1-05。
	ReasoningTruncated bool
	// ReasoningTokens 是 Provider 报告的推理 token 计数（来自
	// usage.completion_tokens_details.reasoning_tokens）。G1-06。
	// 当 Provider 未返回该字段时为 0。
	ReasoningTokens int
}

// StreamChunk SSE 流式传输中的一块数据
type StreamChunk struct {
	Delta     string
	Reasoning string
	Tools     []ToolCall
	IsDone    bool
	Usage     *UsageInfo
	Meta      map[string]any
	// ContentBlocks carries multimodal output in streaming responses
	// (e.g. generated images, audio clips). Providers populate these
	// when the model returns non-text content.
	ContentBlocks []ContentBlock
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
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
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

// StreamRequester is the core interface for streaming model requests. It
// covers provider identity plus the request-building and streaming path used
// by the agent production code (Engine). Callers that only need to drive a
// streaming request — such as orchestrator/agent.Engine — can depend on this
// narrower interface instead of the full ModelRequester.
type StreamRequester interface {
	// Name 返回 Provider 名称
	Name() string
	// GenerateRequestData 将 ModelRequest 转换为 Provider 特定格式
	GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error)
	// RequestModel 发送请求并返回流式 channel
	RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error)
}

// ResponseBroadcaster is the interface for multi-consumer response
// broadcasting. It converts a StreamChunk channel into a ResultEvent channel
// so that stream events (deltas, usage, reasoning, meta) can be fanned out to
// multiple consumers. The agent production path (Engine) does not require
// this capability; it is used by code that needs the richer event stream.
type ResponseBroadcaster interface {
	// BroadcastResponse 将 stream 广播为 ResultEvent channel
	BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error)
}

// ModelRequester 统一模型请求接口. It is the backward-compatible composition
// of StreamRequester and ResponseBroadcaster. Existing code that depends on
// the full provider capability (registry, pool, router, failover) continues
// to use this interface unchanged; providers implement both halves.
type ModelRequester interface { //nolint:revive
	StreamRequester
	ResponseBroadcaster
}
