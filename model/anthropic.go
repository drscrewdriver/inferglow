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

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/inferglow/model/internal/ssestream"
)

// AnthropicCompatibleProvider Anthropic Claude Messages API 兼容 Provider 实现
// 支持 Claude Messages API (/v1/messages) 的 SSE 流式协议
type AnthropicCompatibleProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// FullURL overrides BaseURL + defaultPath ("/v1/messages") when
	// non-empty. Spec: model-parity Phase 1 — full_url 覆盖.
	FullURL string
}

// Name 返回 Provider 名称
func (p *AnthropicCompatibleProvider) Name() string {
	return "anthropic"
}

// CacheCapability returns the prefix-cache profile for this provider.
// Looks up ProviderCacheProfiles[p.Name()] and returns the matching profile
// (or the conservative default if the name is unknown).
func (p *AnthropicCompatibleProvider) CacheCapability() CacheCapability {
	return CacheCapabilityFor(p.Name())
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback
// with a 5-minute timeout (long enough for streaming responses).
func (p *AnthropicCompatibleProvider) effectiveHTTPClient() *http.Client {
	return ssestream.EffectiveHTTPClient(p.HTTPClient)
}

// anthropicBlockState tracks the current content block during SSE parsing.
type anthropicBlockState struct {
	Type string
	ID   string
	Name string
	Args strings.Builder
}

// GenerateRequestData 将 ModelRequest 转换为 Anthropic API 格式
// System 消息存入 Options["_anthropic_system"]，max_tokens 默认 1024
func (p *AnthropicCompatibleProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	model := p.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	if req == nil {
		return nil, fmt.Errorf("model request cannot be nil")
	}

	// 构建 messages（不包含 system，system 单独存入 Options）
	messages := make([]ChatMessage, 0, len(req.ChatHistory)+2)
	messages = append(messages, req.ChatHistory...)

	// 当前请求：构建用户消息（合并 Instruct + Input）
	userMsg := ChatMessage{Role: "user"}
	if req.Instruct != "" {
		userMsg.Content = req.Instruct
	}
	if req.Input != "" {
		if userMsg.Content != "" {
			userMsg.Content += "\n\n" + req.Input
		} else {
			userMsg.Content = req.Input
		}
	}
	// ContentBlocks 多模态处理：当存在图片/音频/视频等内容时，构建多模态消息
	if len(req.ContentBlocks) > 0 {
		if err := gateMultimodal(model, req.ContentBlocks); err != nil {
			return nil, err
		}
		userMsg.ContentBlocks = append(userMsg.ContentBlocks, req.ContentBlocks...)
	}
	if userMsg.Content != "" || len(userMsg.ContentBlocks) > 0 {
		messages = append(messages, userMsg)
	}

	// 工具定义
	var tools []ToolDefinition
	if len(req.Tools) > 0 {
		tools = make([]ToolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = ToolDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			}
		}
	}

	temperature := req.Temperature

	// 复制 Options 并填充 Anthropic 特定字段
	options := make(map[string]any)
	for k, v := range req.Options {
		options[k] = v
	}

	// System 消息存入 Options["_anthropic_system"]
	// M-MEDIUM-9: Anthropic has no native response_format. When the caller
	// specifies an Output schema, inject a JSON instruction into the system
	// message so the model emits JSON matching the requested shape.
	systemMsg := req.System
	if req.Output != nil {
		jsonInstr := "Respond with a JSON object matching this schema: type=" + req.Output.Type
		if len(req.Output.Required) > 0 {
			jsonInstr += ", required=" + strings.Join(req.Output.Required, ",")
		}
		jsonInstr += ". Output ONLY valid JSON, no prose."
		if systemMsg == "" {
			systemMsg = jsonInstr
		} else {
			systemMsg = systemMsg + "\n\n" + jsonInstr
		}
	}
	if systemMsg != "" {
		options["_anthropic_system"] = systemMsg
	}

	// max_tokens 默认 1024，可被 req.Options["max_tokens"] 覆盖
	if _, ok := options["max_tokens"]; !ok {
		options["max_tokens"] = 1024
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Temperature: temperature,
		Options:     options,
		// M-MEDIUM-1: pass through tool_choice.
		ToolChoice: req.ToolChoice,
		// M-MEDIUM-9: pass through Output schema for downstream use.
		Output: req.Output,
	}, nil
}

// anthropicMessages converts provider-agnostic ChatMessages (OpenAI-style
// tool_calls / tool_call_id) into Anthropic's content-block message format.
//
// P0-1: without this adaptation layer, multi-turn native tool calls fail
// because the Anthropic API rejects OpenAI-style tool_calls/tool_call_id
// fields on the second round.
//
// Mapping:
//   - assistant + ToolCalls → content array with optional text block followed
//     by {"type":"tool_use","id":...,"name":...,"input":{...}} blocks
//   - role="tool" + tool_call_id → role="user" with content array containing
//     {"type":"tool_result","tool_use_id":...,"content":"..."} blocks.
//     Consecutive tool messages are merged into a single user message so the
//     result preserves the strictly alternating user/assistant roles that the
//     Anthropic API requires.
//   - plain text messages keep string content (Anthropic accepts both forms).
//   - user messages with ContentBlocks → content array with text + image blocks.
func anthropicMessages(msgs []ChatMessage) []map[string]any {
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.Role == "tool":
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			}
			// Merge into the previous message when it is a user message
			// already carrying tool_result blocks, to keep alternation.
			if len(result) > 0 {
				prev := result[len(result)-1]
				if prev["role"] == "user" {
					if blocks, ok := prev["content"].([]map[string]any); ok {
						prev["content"] = append(blocks, block)
						continue
					}
				}
			}
			result = append(result, map[string]any{
				"role":    "user",
				"content": []map[string]any{block},
			})
		case m.Role == "assistant" && len(m.ToolCalls) > 0:
			blocks := make([]map[string]any, 0, 1+len(m.ToolCalls))
			// G1-02: convert reasoning_content to thinking block for
			// Anthropic-compatible endpoints (Kimi/DeepSeek/MiMo Anthropic).
			if m.ReasoningContent != "" {
				blocks = append(blocks, map[string]any{
					"type":    "thinking",
					"thinking": m.ReasoningContent,
				})
			}
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			result = append(result, map[string]any{
				"role":    "assistant",
				"content": blocks,
			})
		case m.Role == "assistant" && m.ReasoningContent != "":
			// G1-02: assistant message with reasoning but no tool calls.
			// Convert to thinking block + text block for Anthropic endpoints.
			blocks := []map[string]any{
				{"type": "thinking", "thinking": m.ReasoningContent},
			}
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			result = append(result, map[string]any{
				"role":    "assistant",
				"content": blocks,
			})
		case m.Role == "user" && len(m.ContentBlocks) > 0:
			// P0: user message with multimodal content (images, audio).
			// Convert to Anthropic's content array format.
			blocks := make([]map[string]any, 0, 1+len(m.ContentBlocks))
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, cb := range m.ContentBlocks {
				switch cb.Type {
				case ContentImage:
					source := map[string]any{}
					if cb.IsRemote() {
						source["type"] = "url"
						source["url"] = cb.URL
					} else if cb.IsInline() {
						source["type"] = "base64"
						source["media_type"] = cb.MIMEType
						if source["media_type"] == "" {
							source["media_type"] = "image/png"
						}
						source["data"] = base64Encode(cb.Data)
					}
					blocks = append(blocks, map[string]any{
						"type":   "image",
						"source": source,
					})
				case ContentText:
					if len(cb.Data) > 0 {
						blocks = append(blocks, map[string]any{
							"type": "text",
							"text": string(cb.Data),
						})
					}
				}
			}
			result = append(result, map[string]any{
				"role":    "user",
				"content": blocks,
			})
		default:
			result = append(result, map[string]any{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}
	return result
}

// RequestModel 发送 HTTP POST 请求到 {BaseURL}/v1/messages，使用 SSE 流式
func (p *AnthropicCompatibleProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	client := p.effectiveHTTPClient()

	// Spec: model-parity Phase 1 — FullURL overrides BaseURL + defaultPath.
	// When FullURL is empty, ResolveURL degrades to the legacy
	// TrimRight(baseURL,"/") + "/v1/messages" behavior.
	url := ResolveURL(baseURL, "/v1/messages", p.FullURL)

	// 构建请求体。P0-1: messages 经过 anthropicMessages 转换为 Anthropic
	// content-block 格式，使多轮 native tool call 可正确往返。
	reqBody := map[string]any{
		"model":      data.Model,
		"messages":   anthropicMessages(data.Messages),
		"stream":     true,
		"max_tokens": 1024,
	}

	if data.Temperature > 0 {
		reqBody["temperature"] = data.Temperature
	}

	// 应用 Options：_anthropic_system → 顶级 system；max_tokens 覆盖默认
	if len(data.Options) > 0 {
		for k, v := range data.Options {
			if k == "_anthropic_system" {
				reqBody["system"] = v
				continue
			}
			reqBody[k] = v
		}
	}

	// Tools 转换为 Anthropic 工具格式
	if len(data.Tools) > 0 {
		toolsList := make([]map[string]any, 0, len(data.Tools))
		for _, t := range data.Tools {
			toolsList = append(toolsList, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			})
		}
		reqBody["tools"] = toolsList
	}
	// M-MEDIUM-1: tool_choice support — pass through to request body when set.
	if data.ToolChoice != nil {
		reqBody["tool_choice"] = data.ToolChoice
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	// M-MEDIUM-2: anthropic-version updated from outdated "2023-06-01" to
	// "2024-10-22" (Messages API current versioned release).
	httpReq.Header.Set("anthropic-version", "2024-10-22")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// SSE goroutine skeleton, emit closure, ctx.Done() polling, and
	// EOF/error handling are provided by ssestream.RunLines (D7-4 deduplication).
	// 跟踪当前 content block 状态
	blocks := make(map[int]*anthropicBlockState)

	parse := func(line string, emit func(*StreamChunk)) bool {
		return p.processAnthropicLine(line, blocks, emit)
	}

	errorChunk := func(err error) *StreamChunk {
		return &StreamChunk{
			IsDone: true,
			Meta:   map[string]any{"error": err.Error()},
		}
	}

	return ssestream.RunLines(ctx, resp.Body, parse, errorChunk), nil
}

// processAnthropicLine parses one SSE line and emits any resulting
// StreamChunk via emit. Returns true if the goroutine should stop (message_stop).
func (p *AnthropicCompatibleProvider) processAnthropicLine(
	line string,
	blocks map[int]*anthropicBlockState,
	emit func(*StreamChunk),
) bool {
	payload, ok := ssestream.ParseDataLine(line)
	if !ok || payload == "" {
		return false
	}

	var event anthropicEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return false
	}

	switch event.Type {
	case "message_start":
		// M-MEDIUM-11: message_start carries the initial usage (input_tokens)
		// at the start of the response. Capture and emit it so consumers see
		// prompt token counts even when message_delta is missing/partial.
		// The usage is nested under event.message.usage in the Anthropic API.
		if event.Message != nil && event.Message.Usage != nil {
			usage := &UsageInfo{
				PromptTokens:     event.Message.Usage.InputTokens,
				CompletionTokens: event.Message.Usage.OutputTokens,
				TotalTokens:      event.Message.Usage.InputTokens + event.Message.Usage.OutputTokens,
			}
			emit(&StreamChunk{Usage: usage})
		}

	case "content_block_start":
		if event.ContentBlock != nil {
			blocks[event.Index] = &anthropicBlockState{
				Type: event.ContentBlock.Type,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		}

	case "content_block_delta":
		if event.Delta == nil {
			return false
		}
		switch event.Delta.Type {
		case "text_delta":
			emit(&StreamChunk{Delta: event.Delta.Text})
		case "thinking_delta":
			emit(&StreamChunk{Reasoning: event.Delta.Thinking})
		case "input_json_delta":
			if b, ok := blocks[event.Index]; ok {
				b.Args.WriteString(event.Delta.PartialJSON)
			}
		}

	case "content_block_stop":
		b, ok := blocks[event.Index]
		if !ok {
			return false
		}
		if b.Type == "tool_use" {
			var args map[string]any
			if b.Args.Len() > 0 {
				if err := json.Unmarshal([]byte(b.Args.String()), &args); err != nil {
					args = map[string]any{"_raw": b.Args.String()}
				}
			}
			emit(&StreamChunk{
				Tools: []ToolCall{{
					ID:        b.ID,
					Name:      b.Name,
					Arguments: args,
				}},
			})
		}
		delete(blocks, event.Index)

	case "message_delta":
		var usage *UsageInfo
		if event.Usage != nil {
			usage = &UsageInfo{
				PromptTokens:     event.Usage.InputTokens,
				CompletionTokens: event.Usage.OutputTokens,
				TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
			}
		}
		if event.Delta != nil && event.Delta.StopReason != "" {
			emit(&StreamChunk{
				Usage: usage,
				Meta:  map[string]any{"stop_reason": event.Delta.StopReason},
			})
		} else if usage != nil {
			emit(&StreamChunk{Usage: usage})
		}

	case "message_stop":
		emit(&StreamChunk{IsDone: true})
		return true
	}
	return false
}

// BroadcastResponse 将 SSE 流转换为 ResultEvent 流
// 独立实现，逻辑参考 OpenAICompatibleProvider.BroadcastResponse
func (p *AnthropicCompatibleProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)

		var fullContent strings.Builder
		var fullReasoning strings.Builder
		// M-MEDIUM-10: track the last seen Usage so it can be propagated to
		// the final ModelResponse (EventDone payload).
		var lastUsage *UsageInfo

		// P1: streaming <think> tag state machine for providers that
		// embed reasoning inside text deltas.
		var normalizer LeadingThinkNormalizer

		for chunk := range stream {
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if chunk.IsDone {
				if meta, ok := chunk.Meta["error"]; ok {
					events <- &ResultEvent{
						EventType: ErrorEvent,
						Payload:   meta,
					}
					continue
				}
				// P1: flush any content buffered inside the normalizer
				// before assembling the final ModelResponse.
				if et, payload := normalizer.FeedDelta(""); payload != "" {
					switch et {
					case "delta":
						fullContent.WriteString(payload)
						events <- &ResultEvent{EventType: EventDelta, Payload: payload}
					case "reasoning_delta":
						fullReasoning.WriteString(payload)
						events <- &ResultEvent{EventType: ReasoningDelta, Payload: payload}
					}
				}
				resp := &ModelResponse{
					Content:   fullContent.String(),
					Reasoning: fullReasoning.String(),
				}
				if lastUsage != nil {
					resp.Usage = *lastUsage
				}
				// P1: defensive <think> tag normalization. Only triggers
				// when reasoning is empty and content contains think tags.
				if resp.Reasoning == "" && hasThinkingTags(resp.Content) {
					reasoning, cleaned := normalizeThinkingTags(resp.Content)
					resp.Reasoning = reasoning
					resp.Content = cleaned
				}
				events <- &ResultEvent{
					EventType: EventDone,
					Payload:   resp,
				}
				continue
			}

			if chunk.Delta != "" {
				if chunk.Reasoning != "" {
					// P1: reasoning field already separated — do NOT
					// route through the normalizer.
					fullContent.WriteString(chunk.Delta)
					events <- &ResultEvent{EventType: EventDelta, Payload: chunk.Delta}
				} else {
					et, payload := normalizer.FeedDelta(chunk.Delta)
					switch et {
					case "delta":
						fullContent.WriteString(payload)
						events <- &ResultEvent{EventType: EventDelta, Payload: payload}
					case "reasoning_delta":
						fullReasoning.WriteString(payload)
						events <- &ResultEvent{EventType: ReasoningDelta, Payload: payload}
					case "reasoning_done":
						events <- &ResultEvent{EventType: ReasoningDone, Payload: ""}
					}
				}
			}

			if chunk.Reasoning != "" {
				fullReasoning.WriteString(chunk.Reasoning)
				events <- &ResultEvent{
					EventType: ReasoningDelta,
					Payload:   chunk.Reasoning,
				}
			}

			if len(chunk.Tools) > 0 {
				events <- &ResultEvent{
					EventType: ToolCallsEvent,
					Payload:   chunk.Tools,
				}
			}

			if chunk.Usage != nil {
				events <- &ResultEvent{
					EventType: MetaEvent,
					Payload:   chunk.Usage,
				}
			}
		}
	}()

	return events, nil
}

// anthropicEvent Anthropic SSE 事件结构
type anthropicEvent struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
	Delta        *anthropicDelta        `json:"delta,omitempty"`
	Usage        *anthropicUsage        `json:"usage,omitempty"`
	// M-MEDIUM-11: message_start carries the initial usage nested under
	// message.usage.
	Message *anthropicMessage `json:"message,omitempty"`
}

// anthropicMessage is the inner "message" object of message_start events.
type anthropicMessage struct {
	ID    string          `json:"id,omitempty"`
	Usage *anthropicUsage `json:"usage,omitempty"`
}

type anthropicContentBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Text  string         `json:"text,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}
