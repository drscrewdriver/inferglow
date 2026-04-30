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

// OpenAICompatibleProvider OpenAI 兼容协议的 Provider 实现
// 支持 OpenAI / DeepSeek / Qwen / Ollama 等兼容 API
type OpenAICompatibleProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Model      string
}

// Name 返回 Provider 名称
func (p *OpenAICompatibleProvider) Name() string {
	return "openai-compatible"
}

// GenerateRequestData 将 ModelRequest 转换为 OpenAI API 格式
func (p *OpenAICompatibleProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	model := p.Model
	if model == "" {
		model = "gpt-4"
	}

	if req == nil {
		return nil, fmt.Errorf("model request cannot be nil")
	}

	// 构建 messages
	messages := make([]ChatMessage, 0, len(req.ChatHistory)+3)

	// 系统消息
	if req.System != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: req.System})
	}

	// 开发消息
	if req.Developer != "" {
		messages = append(messages, ChatMessage{Role: "developer", Content: req.Developer})
	}

	// 聊天历史
	messages = append(messages, req.ChatHistory...)

	// 当前请求：构建用户消息
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
	if temperature == 0 {
		temperature = 0.7
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Temperature: temperature,
		Options:     req.Options,
	}, nil
}

// RequestModel 发送 HTTP POST 请求，支持 SSE 流式
func (p *OpenAICompatibleProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"

	// 构建请求体
	reqBody := map[string]any{
		"model":    data.Model,
		"messages": data.Messages,
		"stream":   true,
	}
	if data.Temperature > 0 {
		reqBody["temperature"] = data.Temperature
	}
	if len(data.Tools) > 0 {
		reqBody["tools"] = data.Tools
	}
	if len(data.Options) > 0 {
		for k, v := range data.Options {
			reqBody[k] = v
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// 启动 goroutine 解析 SSE 流
	stream := make(chan *StreamChunk, 64)
	go func() {
		defer close(stream)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var usage *UsageInfo
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}

			var chunk openAIChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			schunk := toStreamChunk(chunk, usage)
			stream <- schunk

			if schunk.IsDone {
				break
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
func (p *OpenAICompatibleProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
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

// openAIChunk OpenAI API 的 SSE chunk 结构
type openAIChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int `json:"index"`
		Delta        struct {
			Role          string     `json:"role,omitempty"`
			Content       *string    `json:"content,omitempty"`
			Reasoning     *string    `json:"reasoning,omitempty"`
			ToolCalls     []toolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *UsageInfo `json:"usage"`
}

type toolCall struct {
	Index  int          `json:"index"`
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func toStreamChunk(chunk openAIChunk, usage *UsageInfo) *StreamChunk {
	if len(chunk.Choices) == 0 {
		return &StreamChunk{IsDone: true, Usage: usage}
	}

	c := chunk.Choices[0]
	result := &StreamChunk{
		IsDone: c.FinishReason != "",
		Usage:  usage,
	}

	if c.Delta.Content != nil {
		result.Delta = *c.Delta.Content
	}

	if c.Delta.Reasoning != nil {
		result.Reasoning = *c.Delta.Reasoning
	}

	// 合并 tool calls
	for _, tc := range c.Delta.ToolCalls {
		// 解析 JSON arguments
		var args map[string]any
		if tc.Function.Arguments != "" {
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		result.Tools = append(result.Tools, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	if chunk.Usage != nil {
		result.Usage = chunk.Usage
	}

	return result
}
