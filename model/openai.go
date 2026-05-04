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

// DefaultMaxTokens is the default max_tokens value sent to OpenAI-compatible
// APIs when the caller does not specify one. Chosen to match the OpenAI
// DEFAULT_SETTINGS entry.
const DefaultMaxTokens = 4096

// DefaultTemperature is the default temperature used when the caller does not
// explicitly set Temperature on ModelRequest.
const DefaultTemperature = 0.7

// DefaultOpenAIBaseURL is the default OpenAI-compatible API endpoint.
const DefaultOpenAIBaseURL = "https://api.openai.com/v1"

// reservedFields lists request body fields that must NOT be overridden by
// ModelRequest.Options expansion. These fields are managed explicitly by the
// provider (model/messages/stream/tools/tool_choice/response_format/
// max_tokens/temperature) and allowing Options to overwrite them would
// silently break the request (e.g. Options["model"]="gpt-4o" overriding the
// configured provider model).
//
// M-HIGH-2: whitelist filter prevents reserved field override.
var reservedFields = map[string]bool{
	"model":           true,
	"messages":        true,
	"stream":          true,
	"tools":           true,
	"tool_choice":     true,
	"response_format": true,
	"max_tokens":      true,
	"temperature":     true,
}

// OpenAICompatibleProvider OpenAI 兼容协议的 Provider 实现
// 支持 OpenAI / DeepSeek / Qwen / Ollama 等兼容 API
type OpenAICompatibleProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Model      string
	// ProviderName overrides the value returned by Name(). Defaults to
	// "openai-compatible" when empty so existing behavior is preserved.
	ProviderName string
	// RoleMapping maps request roles (e.g. "developer") to provider-specific
	// roles. If a role is not present in the map, it is used unchanged.
	// Example: DeepSeek/Qwen/GLM/Kimi do not accept "developer"; mapping it
	// to "system" causes the developer content to be merged into the system
	// message instead of being emitted as a separate "developer" message.
	RoleMapping map[string]string
}

// Name 返回 Provider 名称
func (p *OpenAICompatibleProvider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return "openai-compatible"
}

// CacheCapability returns the prefix-cache profile for this provider.
// Looks up ProviderCacheProfiles[p.Name()] and returns the matching profile
// (or the conservative default if the name is unknown). Providers that set
// ProviderName to a known profile name (e.g., "deepseek", "qwen", "openai")
// pick up the corresponding capability automatically.
func (p *OpenAICompatibleProvider) CacheCapability() CacheCapability {
	return CacheCapabilityFor(p.Name())
}

// mapRole applies RoleMapping to the given role. If no mapping exists, the
// role is returned unchanged.
func (p *OpenAICompatibleProvider) mapRole(role string) string {
	if p.RoleMapping == nil {
		return role
	}
	if mapped, ok := p.RoleMapping[role]; ok && mapped != "" {
		return mapped
	}
	return role
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback
// with a 5-minute timeout (long enough for streaming responses).
func (p *OpenAICompatibleProvider) effectiveHTTPClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Minute}
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

	// 开发消息 - apply role mapping. If mapped to "system", merge into the
	// existing system message (or create one if missing) instead of emitting
	// a separate developer message. This is required for DeepSeek/Qwen/GLM/
	// Kimi which reject the "developer" role.
	if req.Developer != "" {
		developerRole := p.mapRole("developer")
		if developerRole == "system" {
			merged := false
			for i, m := range messages {
				if m.Role == "system" {
					messages[i].Content = m.Content + "\n" + req.Developer
					merged = true
					break
				}
			}
			if !merged {
				messages = append(messages, ChatMessage{Role: "system", Content: req.Developer})
			}
		} else {
			messages = append(messages, ChatMessage{Role: developerRole, Content: req.Developer})
		}
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

	// Temperature: respect the caller's value. If TemperatureSet=true, even 0
	// is honored (deterministic). Otherwise the legacy behavior applies: 0 is
	// treated as "unset" and replaced with DefaultTemperature.
	temperature := req.Temperature
	if !req.TemperatureSet {
		if temperature == 0 {
			temperature = DefaultTemperature
		}
	}

	// MaxTokens: prefer caller-provided Options["max_tokens"], else default.
	maxTokens := DefaultMaxTokens
	if req.Options != nil {
		if v, ok := req.Options["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				maxTokens = n
			case int64:
				maxTokens = int(n)
			case float64:
				maxTokens = int(n)
			}
		}
	}

	// O-CRITICAL-1: when the caller requests JSON output (via Options
	// ["force_json"]=true or by setting req.Output), surface that as a
	// response_format hint in the Options map. The actual translation to
	// the OpenAI "response_format" request-body field happens in
	// RequestModel so it stays close to where the body is built.
	opts := req.Options
	if shouldForceJSON(req) {
		// Copy the caller's Options so we don't mutate their map.
		opts = make(map[string]any, len(req.Options)+1)
		for k, v := range req.Options {
			opts[k] = v
		}
		opts["response_format"] = map[string]any{"type": "json_object"}
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Options:     opts,
		// M-MEDIUM-1: pass through tool_choice.
		ToolChoice: req.ToolChoice,
	}, nil
}

// shouldForceJSON reports whether the caller wants the OpenAI-compatible API
// to enforce JSON-object output via response_format. Triggered by either:
//   - req.Options["force_json"] == true, or
//   - req.Output != nil (an OutputSchema signals a structured-output request)
//
// Both conditions are checked here so GenerateRequestData can compute the
// flag once and the RequestModel builder can consult the resulting Options
// map without re-parsing the original ModelRequest.
func shouldForceJSON(req *ModelRequest) bool {
	if req == nil {
		return false
	}
	if req.Output != nil {
		return true
	}
	if req.Options != nil {
		if v, ok := req.Options["force_json"]; ok {
			if b, ok := v.(bool); ok && b {
				return true
			}
		}
	}
	return false
}

// openAITool is the OpenAI API tool envelope: {"type":"function","function":{...}}
// ToolDefinition is the inner "function" spec.
type openAITool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

// openAIToolState accumulates streaming tool_call deltas (id, name, and
// partial JSON arguments) across SSE chunks per tool index.
type openAIToolState struct {
	ID   string
	Name string
	Args strings.Builder
}

// RequestModel 发送 HTTP POST 请求，支持 SSE 流式
func (p *OpenAICompatibleProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	// M-HIGH-4: validate BaseURL — without it the URL becomes relative
	// ("/chat/completions") and the request silently fails or hits an
	// unexpected host.
	baseURL := p.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("OpenAICompatibleProvider.BaseURL is empty; configure base_url or use a factory function")
	}

	client := p.effectiveHTTPClient()

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	// 构建请求体
	reqBody := map[string]any{
		"model":    data.Model,
		"messages": data.Messages,
		"stream":   true,
	}
	if data.Temperature > 0 {
		reqBody["temperature"] = data.Temperature
	}
	// M-CRITICAL-1: wrap each tool in the OpenAI envelope.
	if len(data.Tools) > 0 {
		tools := make([]openAITool, len(data.Tools))
		for i, t := range data.Tools {
			tools[i] = openAITool{Type: "function", Function: t}
		}
		reqBody["tools"] = tools
	}
	// M-HIGH-10: explicit max_tokens takes precedence over Options.
	if data.MaxTokens > 0 {
		reqBody["max_tokens"] = data.MaxTokens
	}
	if len(data.Options) > 0 {
		for k, v := range data.Options {
			// M-HIGH-2: skip reserved fields so Options cannot override
			// model/messages/stream/tools/etc. managed by the provider.
			if reservedFields[k] {
				continue
			}
			reqBody[k] = v
		}
	}
	// Re-apply max_tokens after Options loop so it wins over Options["max_tokens"].
	if data.MaxTokens > 0 {
		reqBody["max_tokens"] = data.MaxTokens
	}
	// O-CRITICAL-1: response_format is in reservedFields (so Options can't
	// override it arbitrarily), but GenerateRequestData intentionally sets
	// Options["response_format"] when the caller requests force_json or sets
	// req.Output. Re-apply it here so the OpenAI-compatible API enforces
	// JSON-object output. Caller-supplied Options["response_format"] (when
	// not via force_json/Output) is also honored since GenerateRequestData
	// passes opts through unchanged.
	if rf, ok := data.Options["response_format"]; ok {
		reqBody["response_format"] = rf
	}
	// M-MEDIUM-1: tool_choice support — pass through to request body when set.
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

		// Use bufio.Reader so we can poll ctx.Done() between reads.
		// M-MEDIUM-6: increase buffer to 1MB so large SSE lines (e.g. tool_call
		// arguments with big JSON payloads) don't trigger excessive refills.
		reader := bufio.NewReaderSize(resp.Body, 1024*1024)
		var usage *UsageInfo

		// M-CRITICAL-2: accumulate streaming tool_call arguments per index.
		toolStates := make(map[int]*openAIToolState)

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
						if processed := p.processOpenAILine(line, usage, toolStates, emit); processed != nil {
							usage = processed
						}
					}
					return
				}
				emit(&StreamChunk{
					IsDone: true,
					Meta:   map[string]any{"error": err.Error()},
				})
				return
			}

			if u := p.processOpenAILine(line, usage, toolStates, emit); u != nil {
				usage = u
			}
		}
	}()

	return stream, nil
}

// processOpenAILine parses one SSE line and emits any resulting StreamChunk
// via emit. Returns the updated usage pointer (nil if unchanged). On
// FinishReason=="tool_calls" it emits the accumulated tool calls.
func (p *OpenAICompatibleProvider) processOpenAILine(
	line string,
	usage *UsageInfo,
	toolStates map[int]*openAIToolState,
	emit func(*StreamChunk),
) *UsageInfo {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	data = strings.TrimSpace(data)
	if data == "[DONE]" || data == "" {
		return nil
	}

	var chunk openAIChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil
	}

	if chunk.Usage != nil {
		usage = chunk.Usage
	}

	// M-MEDIUM-4: empty-choices chunks carry usage or keepalive data, not a
	// finish signal. Update usage but do not emit an IsDone chunk.
	if len(chunk.Choices) == 0 {
		return usage
	}

	c := chunk.Choices[0]
	result := &StreamChunk{
		Usage: usage,
	}

	if c.Delta.Content != nil {
		result.Delta = *c.Delta.Content
	}

	if c.Delta.Reasoning != nil {
		result.Reasoning = *c.Delta.Reasoning
	}

	// Accumulate tool_call deltas per index. The name and id arrive in the
	// first chunk; arguments arrive in pieces across subsequent chunks.
	for _, tc := range c.Delta.ToolCalls {
		st, ok := toolStates[tc.Index]
		if !ok {
			st = &openAIToolState{}
			toolStates[tc.Index] = st
		}
		if tc.ID != "" {
			st.ID = tc.ID
		}
		if tc.Function.Name != "" {
			st.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			st.Args.WriteString(tc.Function.Arguments)
		}
	}

	if c.FinishReason == "tool_calls" {
		// Emit accumulated tool calls.
		// Iterate by sorted index for deterministic order.
		indices := make([]int, 0, len(toolStates))
		for idx := range toolStates {
			indices = append(indices, idx)
		}
		// Simple insertion sort for small slices.
		for i := 1; i < len(indices); i++ {
			for j := i; j > 0 && indices[j] < indices[j-1]; j-- {
				indices[j], indices[j-1] = indices[j-1], indices[j]
			}
		}
		tools := make([]ToolCall, 0, len(indices))
		for _, idx := range indices {
			st := toolStates[idx]
			args := map[string]any{}
			if st.Args.Len() > 0 {
				if err := json.Unmarshal([]byte(st.Args.String()), &args); err != nil {
					args = map[string]any{"_raw": st.Args.String()}
				}
			}
			tools = append(tools, ToolCall{
				ID:        st.ID,
				Name:      st.Name,
				Arguments: args,
			})
		}
		result.Tools = tools
		result.IsDone = true
		// Clear toolStates so a subsequent stream doesn't reuse them.
		for k := range toolStates {
			delete(toolStates, k)
		}
	} else if c.FinishReason != "" && c.FinishReason != "length" {
		// M-MEDIUM-5: "length" indicates truncation by max_tokens — the
		// response is incomplete. Don't mark IsDone; let the stream
		// terminate naturally via [DONE] / EOF.
		result.IsDone = true
	}

	emit(result)
	return usage
}

// BroadcastResponse 将 SSE 流转换为 ResultEvent 流
func (p *OpenAICompatibleProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)

		var fullContent strings.Builder
		var fullReasoning strings.Builder
		// M-MEDIUM-10: track the last seen Usage so it can be propagated to
		// the final ModelResponse (EventDone payload).
		var lastUsage *UsageInfo

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
				resp := &ModelResponse{
					Content:   fullContent.String(),
					Reasoning: fullReasoning.String(),
				}
				if lastUsage != nil {
					resp.Usage = *lastUsage
				}
				events <- &ResultEvent{
					EventType: EventDone,
					Payload:   resp,
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

// toStreamChunk is retained for backward compatibility with existing tests
// that construct openAIChunk values directly. New streaming code uses
// processOpenAILine above which properly accumulates tool_call arguments.
func toStreamChunk(chunk openAIChunk, usage *UsageInfo) *StreamChunk {
	// M-MEDIUM-4: an empty-choices chunk carries usage or keepalive data,
	// not a finish signal. Return nil so callers can skip emission.
	if len(chunk.Choices) == 0 {
		return nil
	}

	c := chunk.Choices[0]
	result := &StreamChunk{
		// M-MEDIUM-5: "length" indicates the response was truncated by
		// max_tokens — not a successful completion. Only genuine completion
		// signals mark IsDone.
		IsDone: c.FinishReason != "" && c.FinishReason != "length",
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
