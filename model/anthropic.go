package model

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicCompatibleProvider Anthropic Claude Messages API 兼容 Provider 实现
// 支持 Claude Messages API (/v1/messages) 的 SSE 流式协议
type AnthropicCompatibleProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// Name 返回 Provider 名称
func (p *AnthropicCompatibleProvider) Name() string {
	return "anthropic"
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
	if userMsg.Content != "" {
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
	if req.System != "" {
		options["_anthropic_system"] = req.System
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
	}, nil
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

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	url := strings.TrimRight(baseURL, "/") + "/v1/messages"

	// 构建请求体
	reqBody := map[string]any{
		"model":      data.Model,
		"messages":   data.Messages,
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
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	stream := make(chan *StreamChunk, 64)
	go func() {
		defer close(stream)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// 增大 buffer 以支持长 tool_use arguments
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		// 跟踪当前 content block 状态
		type blockState struct {
			Type string
			ID   string
			Name string
			Args strings.Builder
		}
		blocks := make(map[int]*blockState)

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			payload := strings.TrimPrefix(line, "data: ")
			if payload == "" {
				continue
			}

			var event anthropicEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_start":
				if event.ContentBlock != nil {
					blocks[event.Index] = &blockState{
						Type: event.ContentBlock.Type,
						ID:   event.ContentBlock.ID,
						Name: event.ContentBlock.Name,
					}
				}

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "text_delta":
					stream <- &StreamChunk{Delta: event.Delta.Text}
				case "thinking_delta":
					stream <- &StreamChunk{Reasoning: event.Delta.Thinking}
				case "input_json_delta":
					if b, ok := blocks[event.Index]; ok {
						b.Args.WriteString(event.Delta.PartialJSON)
					}
				}

			case "content_block_stop":
				b, ok := blocks[event.Index]
				if !ok {
					continue
				}
				if b.Type == "tool_use" {
					var args map[string]any
					if b.Args.Len() > 0 {
						if err := json.Unmarshal([]byte(b.Args.String()), &args); err != nil {
							args = map[string]any{"_raw": b.Args.String()}
						}
					}
					stream <- &StreamChunk{
						Tools: []ToolCall{{
							ID:        b.ID,
							Name:      b.Name,
							Arguments: args,
						}},
					}
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
					stream <- &StreamChunk{
						Usage: usage,
						Meta:  map[string]any{"stop_reason": event.Delta.StopReason},
					}
				} else if usage != nil {
					stream <- &StreamChunk{Usage: usage}
				}

			case "message_stop":
				stream <- &StreamChunk{IsDone: true}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			stream <- &StreamChunk{
				IsDone: true,
				Meta:   map[string]any{"error": err.Error()},
			}
		}
	}()

	return stream, nil
}

// BroadcastResponse 将 SSE 流转换为 ResultEvent 流
// 独立实现，逻辑参考 OpenAICompatibleProvider.BroadcastResponse
func (p *AnthropicCompatibleProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)

		var fullContent strings.Builder
		var fullReasoning strings.Builder

		for chunk := range stream {
			if chunk.IsDone {
				if meta, ok := chunk.Meta["error"]; ok {
					events <- &ResultEvent{
						EventType: ErrorEvent,
						Payload:   meta,
					}
					continue
				}
				events <- &ResultEvent{
					EventType: EventDone,
					Payload: &ModelResponse{
						Content:   fullContent.String(),
						Reasoning: fullReasoning.String(),
					},
				}
				continue
			}

			if chunk.Delta != "" {
				fullContent.WriteString(chunk.Delta)
				events <- &ResultEvent{
					EventType: EventDelta,
					Payload:   chunk.Delta,
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
