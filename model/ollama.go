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

// OllamaProvider 支持 Ollama 本地 LLM 模型接入
// 默认地址: http://localhost:11434，无需 API Key
type OllamaProvider struct {
	BaseURL    string
	APIKey     string // Ollama 不需要 API Key，保留字段以兼容接口
	HTTPClient *http.Client
	Model      string
}

// NewOllamaProvider 创建默认 OllamaProvider
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		BaseURL: "http://localhost:11434",
	}
}

// Name 返回提供商标识
func (p *OllamaProvider) Name() string { return "ollama" }

// GenerateRequestData 将 ModelRequest 转换为 Ollama API 格式
// 请求格式: {"model": "...", "messages": [...], "stream": true}
func (p *OllamaProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	model := p.Model
	if model == "" {
		model = "llama3"
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

	temperature := req.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		Options:     req.Options,
	}, nil
}

// RequestModel 发送 HTTP POST 请求到 /api/chat，支持 SSE 流式
func (p *OllamaProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	url := strings.TrimRight(p.BaseURL, "/") + "/api/chat"

	// 构建请求体
	reqBody := map[string]any{
		"model":    data.Model,
		"messages": data.Messages,
		"stream":   true,
	}
	if data.Temperature > 0 {
		reqBody["temperature"] = data.Temperature
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

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	// 启动 goroutine 解析 Ollama SSE 流
	// Ollama 每条独立 JSON: {"done": false, "message": {"role": "assistant", "content": "..."}}
	stream := make(chan *StreamChunk, 64)
	go func() {
		defer close(stream)
		defer resp.Body.Close()

		var fullContent strings.Builder
		var usage *UsageInfo
		var done bool

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var chunk ollamaChunk
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}

			// 提取内容
			if chunk.Message != nil && chunk.Message.Content != "" {
				fullContent.WriteString(chunk.Message.Content)
				stream <- &StreamChunk{
					Delta:   chunk.Message.Content,
					IsDone:  false,
					Usage:   nil,
					Meta:    nil,
				}
			}

			// 更新 usage
			if chunk.Usage != nil {
				usage = &UsageInfo{
					PromptTokens:     chunk.Usage.PromptEvalCount,
					CompletionTokens: chunk.Usage.EvalCount,
					TotalTokens:      chunk.Usage.PromptEvalCount + chunk.Usage.EvalCount,
				}
			}

			// done=true 表示完成
			if chunk.Done {
				done = true
				stream <- &StreamChunk{
					IsDone:  true,
					Usage:   usage,
					Meta:    nil,
					Delta:   "",
				}
				break
			}
		}

		if err := scanner.Err(); err != nil && !done {
			stream <- &StreamChunk{
				IsDone: true,
				Meta:   map[string]any{"error": err.Error()},
			}
		}
	}()

	return stream, nil
}

// BroadcastResponse 将 SSE 流转换为 ResultEvent 流
func (p *OllamaProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)

		var fullContent strings.Builder

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
						Content: fullContent.String(),
						Usage:   *chunk.Usage,
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

// ollamaChunk Ollama API 的 SSE chunk 结构
// 每条独立 JSON: {"model": "...", "done": false, "message": {...}, "usage": {...}}
type ollamaChunk struct {
	Model   string  `json:"model"`
	Done    bool    `json:"done"`
	Message *struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Usage *struct {
		PromptEvalCount  int `json:"prompt_eval_count"`
		EvalCount        int `json:"eval_count"`
		EvalDuration     int64 `json:"eval_duration"`
		PromptEvalDuration int64 `json:"prompt_eval_duration"`
	} `json:"usage"`
}
