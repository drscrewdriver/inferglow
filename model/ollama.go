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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider 支持 Ollama 本地 LLM 模型接入
// 默认地址: http://localhost:11434，无需 API Key
type OllamaProvider struct {
	BaseURL    string
	APIKey     string // Ollama 不需要 API Key，保留字段以兼容接口
	HTTPClient *http.Client
	Model      string
	// FullURL overrides BaseURL + defaultPath ("/api/chat") when non-empty.
	// Spec: model-parity Phase 1 — full_url 覆盖.
	FullURL string
	// ContentMapping allows extracting delta/reasoning from non-standard
	// JSON paths in Ollama-compatible SSE chunks. When nil, the default
	// struct-based parsing (message.content / message.reasoning) is used.
	// Spec: model-parity Phase 5 — content_mapping for Ollama.
	ContentMapping ContentMapping
}

// NewOllamaProvider 创建默认 OllamaProvider
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		BaseURL: "http://localhost:11434",
	}
}

// Name 返回提供商标识
func (p *OllamaProvider) Name() string { return "ollama" }

// CacheCapability returns the prefix-cache profile for this provider.
// Ollama's Name() returns "ollama" which maps to the conservative (no-cache)
// profile in ProviderCacheProfiles.
func (p *OllamaProvider) CacheCapability() CacheCapability {
	return CacheCapabilityFor(p.Name())
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback
// with a 5-minute timeout (long enough for streaming responses).
func (p *OllamaProvider) effectiveHTTPClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

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

	// Copy tools so RequestModel can serialize them into the Ollama request body.
	var tools []ToolDefinition
	if len(req.Tools) > 0 {
		tools = make([]ToolDefinition, len(req.Tools))
		copy(tools, req.Tools)
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Temperature: temperature,
		Options:     req.Options,
		ToolChoice:  req.ToolChoice,
		// M-MEDIUM-9: pass through Output schema so RequestModel can enable
		// Ollama JSON mode (format: "json").
		Output: req.Output,
	}, nil
}

// RequestModel 发送 HTTP POST 请求到 /api/chat，支持 SSE 流式
func (p *OllamaProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	// M-HIGH-12: use effectiveHTTPClient (5min timeout fallback) instead
	// of a bare &http.Client{} which has no timeout and can hang forever.
	client := p.effectiveHTTPClient()

	// Spec: model-parity Phase 1 — FullURL overrides BaseURL + defaultPath.
	// When FullURL is empty, ResolveURL degrades to the legacy
	// TrimRight(p.BaseURL,"/") + "/api/chat" behavior.
	url := ResolveURL(p.BaseURL, "/api/chat", p.FullURL)

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
		// M-MEDIUM-3: Ollama /api/chat expects Options nested under an
		// "options" sub-object (e.g. {"options":{"top_p":0.9,"seed":42}}).
		// Top-level expansion is silently ignored by the server.
		opts := make(map[string]any, len(data.Options))
		for k, v := range data.Options {
			opts[k] = v
		}
		reqBody["options"] = opts
	}
	// M-MEDIUM-9: Ollama JSON mode — when caller specifies an Output schema,
	// enable structured JSON output via format:"json". Ollama has no native
	// response_format; format:"json" is the documented way to request JSON.
	if data.Output != nil {
		reqBody["format"] = "json"
	}
	// Tool use: Ollama /api/chat supports tools in the same envelope format
	// as OpenAI: {"type":"function","function":{name,description,parameters}}.
	// When tools are present, JSON mode (format:"json") is NOT set — the model
	// uses native function calling instead.
	if len(data.Tools) > 0 {
		type ollamaTool struct {
			Type     string         `json:"type"`
			Function ToolDefinition `json:"function"`
		}
		ollamaTools := make([]ollamaTool, len(data.Tools))
		for i, t := range data.Tools {
			ollamaTools[i] = ollamaTool{Type: "function", Function: t}
		}
		reqBody["tools"] = ollamaTools
		// Remove format:"json" if it was set — tools and format are mutually exclusive.
		delete(reqBody, "format")
	}
	if data.ToolChoice != nil {
		reqBody["tool_choice"] = data.ToolChoice
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

		// M-CRITICAL-3: use bufio.Reader so we can poll ctx.Done() between reads.
		// M-MEDIUM-6: 1MB buffer for large SSE lines.
		reader := bufio.NewReaderSize(resp.Body, 1024*1024)
		var usage *UsageInfo

		// M-CRITICAL-4: emit selects on ctx.Done() so the goroutine exits
		// even if the channel buffer is full and the consumer has stopped.
		emit := func(schunk *StreamChunk) {
			select {
			case stream <- schunk:
			case <-ctx.Done():
			}
		}

		for {
			// M-CRITICAL-3: poll context before each read so a cancelled
			// consumer doesn't leak this goroutine.
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if strings.TrimSpace(line) != "" {
						p.processOllamaLine(line, &usage, emit)
					}
					return
				}
				emit(&StreamChunk{
					IsDone: true,
					Meta:   map[string]any{"error": err.Error()},
				})
				return
			}

			stop := p.processOllamaLine(line, &usage, emit)
			if stop {
				return
			}
		}
	}()

	return stream, nil
}

// processOllamaLine parses one Ollama SSE line (a standalone JSON object per
// line) and emits any resulting StreamChunk via emit. Returns true if the
// goroutine should stop (chunk.Done == true).
func (p *OllamaProvider) processOllamaLine(
	line string,
	usage **UsageInfo,
	emit func(*StreamChunk),
) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	// P2: when ContentMapping is set, extract delta/reasoning from
	// non-standard JSON paths via ExtractByPath. Fall back to struct
	// parsing for usage and done fields.
	if p.ContentMapping != nil {
		return p.processOllamaLineWithMapping(line, usage, emit)
	}

	var chunk ollamaChunk
	if err := json.Unmarshal([]byte(line), &chunk); err != nil {
		return false
	}

	// 提取内容、推理和工具调用
	if chunk.Message != nil {
		// Emit tool calls when present (Ollama returns tool_calls in the
		// done message or in intermediate messages).
		if len(chunk.Message.ToolCalls) > 0 {
			tools := make([]ToolCall, 0, len(chunk.Message.ToolCalls))
			for i, tc := range chunk.Message.ToolCalls {
				tcID := tc.ID
				if tcID == "" {
					tcID = fmt.Sprintf("ollama_tc_%d", i)
				}
				tools = append(tools, ToolCall{
					ID:        tcID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
			emit(&StreamChunk{Tools: tools, IsDone: true})
			return true
		}

		if chunk.Message.Content != "" || chunk.Message.Reasoning != "" || chunk.Message.ReasoningContent != "" {
			sc := &StreamChunk{IsDone: false}
			if chunk.Message.Content != "" {
				sc.Delta = chunk.Message.Content
			}
			// P2: extract reasoning from message.reasoning or
			// message.reasoning_content (vendor-dependent field name).
			if chunk.Message.Reasoning != "" {
				sc.Reasoning = chunk.Message.Reasoning
			} else if chunk.Message.ReasoningContent != "" {
				sc.Reasoning = chunk.Message.ReasoningContent
			}
			emit(sc)
		}
	}

	// 更新 usage
	if chunk.Usage != nil {
		*usage = &UsageInfo{
			PromptTokens:     chunk.Usage.PromptEvalCount,
			CompletionTokens: chunk.Usage.EvalCount,
			TotalTokens:      chunk.Usage.PromptEvalCount + chunk.Usage.EvalCount,
		}
	}

	// done=true 表示完成
	if chunk.Done {
		emit(&StreamChunk{
			IsDone: true,
			Usage:  *usage,
		})
		return true
	}
	return false
}

// processOllamaLineWithMapping parses an Ollama SSE line using
// ContentMapping paths to extract delta and reasoning from
// non-standard JSON structures. Usage and done fields still use
// the standard struct for compatibility.
func (p *OllamaProvider) processOllamaLineWithMapping(
	line string,
	usage **UsageInfo,
	emit func(*StreamChunk),
) bool {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return false
	}

	sc := &StreamChunk{IsDone: false}

	if deltaPath, ok := p.ContentMapping["delta"]; ok {
		if v, found := ExtractByPath(raw, deltaPath); found {
			if s, ok := v.(string); ok {
				sc.Delta = s
			}
		}
	} else {
		// Default: message.content
		if v, found := ExtractByPath(raw, "message.content"); found {
			if s, ok := v.(string); ok {
				sc.Delta = s
			}
		}
	}

	if reasoningPath, ok := p.ContentMapping["reasoning"]; ok {
		if v, found := ExtractByPath(raw, reasoningPath); found {
			if s, ok := v.(string); ok {
				sc.Reasoning = s
			}
		}
	}

	if sc.Delta != "" || sc.Reasoning != "" {
		emit(sc)
	}

	// 更新 usage (struct-based for compatibility)
	var chunk ollamaChunk
	_ = json.Unmarshal([]byte(line), &chunk)

	// Extract tool calls from the struct (tool_calls field is always parsed
	// from the standard Ollama location regardless of ContentMapping).
	if chunk.Message != nil && len(chunk.Message.ToolCalls) > 0 {
		tools := make([]ToolCall, 0, len(chunk.Message.ToolCalls))
		for i, tc := range chunk.Message.ToolCalls {
			tcID := tc.ID
			if tcID == "" {
				tcID = fmt.Sprintf("ollama_tc_%d", i)
			}
			tools = append(tools, ToolCall{
				ID:        tcID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		emit(&StreamChunk{Tools: tools, IsDone: true})
		return true
	}

	if chunk.Usage != nil {
		*usage = &UsageInfo{
			PromptTokens:     chunk.Usage.PromptEvalCount,
			CompletionTokens: chunk.Usage.EvalCount,
			TotalTokens:      chunk.Usage.PromptEvalCount + chunk.Usage.EvalCount,
		}
	}

	if chunk.Done {
		emit(&StreamChunk{
			IsDone: true,
			Usage:  *usage,
		})
		return true
	}
	return false
}

// BroadcastResponse 将 SSE 流转换为 ResultEvent 流
func (p *OllamaProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)

		var fullContent strings.Builder
		var fullReasoning strings.Builder
		// BUG-NEW-1: track the last seen Usage so it can be propagated to the
		// final ModelResponse (EventDone payload) — same pattern as OpenAI
		// (openai.go) and Anthropic (anthropic.go) providers. Avoids nil
		// dereference when the done chunk arrives without a Usage field.
		var lastUsage *UsageInfo

		// P2: streaming <think> tag state machine.
		var normalizer LeadingThinkNormalizer

		for chunk := range stream {
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			// Emit tool calls event when present.
			if len(chunk.Tools) > 0 {
				events <- &ResultEvent{
					EventType: ToolCallsEvent,
					Payload:   chunk.Tools,
				}
			}
			if chunk.IsDone {
				if meta, ok := chunk.Meta["error"]; ok {
					events <- &ResultEvent{
						EventType: ErrorEvent,
						Payload:   meta,
					}
					continue
				}
				// P2: flush any content buffered inside the normalizer.
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
				// P2: defensive <think> tag normalization fallback.
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
					// P2: reasoning field already separated — do NOT
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
	Model   string `json:"model"`
	Done    bool   `json:"done"`
	Message *struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		Reasoning        string `json:"reasoning"`
		ReasoningContent string `json:"reasoning_content"`
		ToolCalls        []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Usage *struct {
		PromptEvalCount    int   `json:"prompt_eval_count"`
		EvalCount          int   `json:"eval_count"`
		EvalDuration       int64 `json:"eval_duration"`
		PromptEvalDuration int64 `json:"prompt_eval_duration"`
	} `json:"usage"`
}
