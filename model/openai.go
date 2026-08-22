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
	"unicode/utf8"

	"github.com/inferglow/model/internal/ssestream"
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
	// MaxReasoningTokens 限制推理内容（reasoning/thinking）的最大 token 数。
	// 0 表示不限制（向后兼容）。达到预算时 BroadcastResponse 会截断 fullReasoning
	// 并在 ModelResponse 中设置 ReasoningTruncated=true。G1-05。
	// 注意：token 数按 1 token ≈ 4 bytes 的粗略估算，未接入精确 tokenizer。
	MaxReasoningTokens int
	// FullURL overrides BaseURL + defaultPath ("/chat/completions") when
	// non-empty. Spec: model-parity Phase 1 — full_url 覆盖.
	FullURL string
	// ContentMapping, when non-nil, lets processOpenAILine extract reasoning
	// and delta from non-standard SSE field paths via ExtractByPath. When nil,
	// the provider falls back to its struct-based parsing (legacy behavior).
	// Spec: model-parity Phase 3 — content_mapping 字段路径自定义.
	ContentMapping ContentMapping
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
	return ssestream.MapRole(role, p.RoleMapping)
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback
// with a 5-minute timeout (long enough for streaming responses).
func (p *OpenAICompatibleProvider) effectiveHTTPClient() *http.Client {
	return ssestream.EffectiveHTTPClient(p.HTTPClient)
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
		// L1/L2: When Output.Properties is non-empty and caller hasn't explicitly
		// forced json_object mode, emit full json_schema response_format.
		// vLLM/SGLang XGrammar consumes this for token-level constrained decoding.
		// OpenAI/DeepSeek use server-side structured output.
		//
		// The json_schema upgrade is gated behind an explicit force_json=true so
		// legacy callers that set only req.Output (without force_json) keep
		// receiving {"type":"json_object"} — preserving backward compatibility.
		forceJSONExplicit := false
		if req.Options != nil {
			if b, ok := req.Options["force_json"].(bool); ok && b {
				forceJSONExplicit = true
			}
		}
		if forceJSONExplicit && req.Output != nil && len(req.Output.Properties) > 0 && !forceJSONObject(req) {
			jsonSchema := BuildJSONSchemaFromOutput(req.Output)
			opts["response_format"] = map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "structured_output",
					"strict": true,
					"schema": jsonSchema,
				},
			}
		} else {
			// Degradation: no specific schema or forced json_object mode
			opts["response_format"] = map[string]any{"type": "json_object"}
		}
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

// fixEmptyReasoningContent returns a copy of msgs where assistant messages
// with empty ReasoningContent have it replaced with " " (single space).
// This works around a DeepSeek V4 behavior where empty string
// reasoning_content in multi-turn requests triggers a 400 error.
// Following Hermes practice: send " " instead of "".
// NOTE: This is intentionally a no-op. The actual empty→space fix is applied
// at the session layer (PreparePrompt) where Meta context is available to
// distinguish "reasoning was empty" from "no reasoning field". This function
// is retained as a hook for provider-specific fixes if needed.
func fixEmptyReasoningContent(msgs []ChatMessage) []ChatMessage {
	return msgs
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

	// Spec: model-parity Phase 1 — FullURL overrides BaseURL + defaultPath.
	// When FullURL is empty, ResolveURL degrades to the legacy
	// TrimRight(baseURL,"/") + "/chat/completions" behavior (preserved for
	// backward compatibility — all existing tests rely on this).
	url := ResolveURL(baseURL, "/chat/completions", p.FullURL)

	// 构建请求体
	// G1-02: fix empty reasoning_content for providers that reject it (DeepSeek V4).
	// When an assistant message has reasoning_content set to "" (empty string),
	// some providers return 400. Replace with " " (space) following Hermes practice.
	msgs := fixEmptyReasoningContent(data.Messages)
	reqBody := map[string]any{
		"model":    data.Model,
		"messages": msgs,
		"stream":   true,
		// Request usage info in streaming mode (OpenAI-compatible APIs).
		"stream_options": map[string]any{"include_usage": true},
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

	// 启动 goroutine 解析 SSE 流。The goroutine skeleton, emit closure,
	// ctx.Done() polling, and EOF/error handling are provided by
	// ssestream.RunLines (D7-4 deduplication).
	//
	// M-CRITICAL-2: accumulate streaming tool_call arguments per index.
	toolStates := make(map[int]*openAIToolState)
	var usage *UsageInfo

	parse := func(line string, emit func(*StreamChunk)) bool {
		if u := p.processOpenAILine(line, usage, toolStates, emit); u != nil {
			usage = u
		}
		return false
	}

	errorChunk := func(err error) *StreamChunk {
		return &StreamChunk{
			IsDone: true,
			Meta:   map[string]any{"error": err.Error()},
		}
	}

	return ssestream.RunLines(ctx, resp.Body, parse, errorChunk), nil
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
	data, ok := ssestream.ParseDataLine(line)
	if !ok {
		return nil
	}
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

	// Spec: model-parity Phase 3 — when ContentMapping is set, extract
	// reasoning/delta from non-standard SSE field paths via ExtractByPath.
	// This bypasses the struct-based delta parsing below so vendors that
	// put reasoning under data.thinking (or any non-standard location) work
	// without forking the provider. Falls back to legacy parsing when nil.
	if p.ContentMapping != nil {
		var raw map[string]any
		// Re-unmarshal into a generic map for path extraction. The cost is
		// acceptable: ContentMapping is an opt-in feature for non-standard
		// vendors, not the default OpenAI path.
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			return usage
		}
		result := &StreamChunk{Usage: usage}
		if path, ok := p.ContentMapping["delta"]; ok && path != "" {
			if v, ok := ExtractByPath(raw, path); ok {
				if s, ok := v.(string); ok {
					result.Delta = s
				}
			}
		}
		if path, ok := p.ContentMapping["reasoning"]; ok && path != "" {
			if v, ok := ExtractByPath(raw, path); ok {
				if s, ok := v.(string); ok {
					result.Reasoning = s
				}
			}
		}
		// Accumulate tool_call deltas in ContentMapping path as well.
		// Some vendors use standard tool_calls delta structure even when
		// content/reasoning paths are non-standard.
		if len(chunk.Choices) > 0 {
			c := chunk.Choices[0]
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
				indices := make([]int, 0, len(toolStates))
				for idx := range toolStates {
					indices = append(indices, idx)
				}
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
				for k := range toolStates {
					delete(toolStates, k)
				}
			} else if c.FinishReason != "" && c.FinishReason != "length" {
				result.IsDone = true
			}
		}
		emit(result)
		return usage
	}

	// M-MEDIUM-4: empty-choices chunks carry usage or keepalive data, not a
	// finish signal. Emit a chunk with usage so the engine can capture it.
	if len(chunk.Choices) == 0 {
		if usage != nil {
			emit(&StreamChunk{Usage: usage})
		}
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
	if c.Delta.ReasoningContent != nil {
		// reasoning_content 优先级高于 reasoning（MiMo/Spark/SenseNova 用此字段）
		result.Reasoning = *c.Delta.ReasoningContent
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

		// G1-05: 推理预算控制。MaxReasoningTokens > 0 时启用。
		// 粗略估算 1 token ≈ 4 bytes（未接入精确 tokenizer）。
		reasoningBudgetBytes := p.MaxReasoningTokens * 4
		reasoningBytes := 0
		reasoningTruncated := false

		// P1: streaming <think> tag state machine. Providers that embed
		// reasoning inside the content delta (rather than a separate
		// reasoning field) are normalized here so that downstream
		// consumers receive ReasoningDelta / EventDelta events with the
		// think tags already stripped.
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
				// (e.g. trailing answer text glued to a </think> close tag)
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
					Content:            fullContent.String(),
					Reasoning:          fullReasoning.String(),
					ReasoningTruncated: reasoningTruncated,
				}
				if lastUsage != nil {
					resp.Usage = *lastUsage
					// G1-06: 提取 Provider 报告的 reasoning_tokens 计数。
					resp.ReasoningTokens = lastUsage.ReasoningTokens()
				}
				// G1-04: 防御性 <think> 标签归一化。仅当 reasoning 字段为空
				// 且 content 含 <think>...</think> 标签时触发，避免双重处理。
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
					// P1: reasoning field already separated by the
					// provider — do NOT route through the normalizer
					// (avoids double processing when <think> tags are
					// present in content alongside a dedicated reasoning
					// field).
					fullContent.WriteString(chunk.Delta)
					events <- &ResultEvent{EventType: EventDelta, Payload: chunk.Delta}
				} else {
					et, payload := normalizer.FeedDelta(chunk.Delta)
					switch et {
					case "delta":
						fullContent.WriteString(payload)
						events <- &ResultEvent{EventType: EventDelta, Payload: payload}
					case "reasoning_delta":
						if reasoningBudgetBytes > 0 && !reasoningTruncated {
							chunkBytes := len(payload)
							if reasoningBytes+chunkBytes > reasoningBudgetBytes {
								remaining := reasoningBudgetBytes - reasoningBytes
								if remaining > 0 {
									truncated := truncateRunes(payload, remaining)
									fullReasoning.WriteString(truncated)
									reasoningBytes += len(truncated)
								}
								reasoningTruncated = true
							} else {
								fullReasoning.WriteString(payload)
								reasoningBytes += len(payload)
							}
						} else if reasoningBudgetBytes == 0 {
							fullReasoning.WriteString(payload)
						}
						events <- &ResultEvent{EventType: ReasoningDelta, Payload: payload}
					case "reasoning_done":
						events <- &ResultEvent{EventType: ReasoningDone, Payload: ""}
					}
				}
			}

			if chunk.Reasoning != "" {
				// G1-05: 推理预算检查。一旦超预算，停止累积到 fullReasoning
				// 但仍向下游传播 ReasoningDelta 事件（流式消费者可自行决定
				// 是否展示被截断的推理）。最终在 ModelResponse 中通过
				// ReasoningTruncated=true 标记。
				if reasoningBudgetBytes > 0 && !reasoningTruncated {
					chunkBytes := len(chunk.Reasoning)
					if reasoningBytes+chunkBytes > reasoningBudgetBytes {
						// 仅写入预算内剩余部分。
						remaining := reasoningBudgetBytes - reasoningBytes
						if remaining > 0 {
							// 按 rune 截断以避免切断多字节字符。
							truncated := truncateRunes(chunk.Reasoning, remaining)
							fullReasoning.WriteString(truncated)
							reasoningBytes += len(truncated)
						}
						reasoningTruncated = true
					} else {
						fullReasoning.WriteString(chunk.Reasoning)
						reasoningBytes += chunkBytes
					}
				} else if reasoningBudgetBytes == 0 {
					// 无预算限制：原样累积。
					fullReasoning.WriteString(chunk.Reasoning)
				}
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
				// G1-06: 当 Provider 报告 reasoning_tokens 时，额外发一条
				// 专用 MetaEvent，便于上层无需解析 UsageInfo 即可拿到计费信号。
				if rt := chunk.Usage.ReasoningTokens(); rt > 0 {
					events <- &ResultEvent{
						EventType: MetaEvent,
						Payload:   &ReasoningTokenMeta{Count: rt},
					}
				}
			}
		}
	}()

	return events, nil
}

// truncateRunes 从 s 中截取不超过 maxBytes 字节的子串，按 rune 边界对齐，
// 避免切断多字节 UTF-8 字符。用于 G1-05 推理预算截断。
func truncateRunes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		if maxBytes <= 0 {
			return ""
		}
		return s
	}
	// 向前回退至完整 rune 边界。
	end := maxBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

// openAIChunk OpenAI API 的 SSE chunk 结构
type openAIChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string     `json:"role,omitempty"`
			Content          *string    `json:"content,omitempty"`
			Reasoning        *string    `json:"reasoning,omitempty"`
			ReasoningContent *string    `json:"reasoning_content,omitempty"`
			ReasoningDetails *string    `json:"reasoning_details,omitempty"` // OpenRouter
			ToolCalls        []toolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *UsageInfo `json:"usage"`
}

type toolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
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
	if c.Delta.ReasoningContent != nil {
		// reasoning_content 优先级高于 reasoning（MiMo/Spark/SenseNova 用此字段）
		result.Reasoning = *c.Delta.ReasoningContent
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


