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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/inferglow/model/internal/ssestream"
)

// GoogleGenerativeProvider implements the native Google Generative Language
// API (streamGenerateContent) protocol. LLM-provider-port: port of pi-ai's
// google-generative-ai adapter. Handles Gemini 3 / Gemma 4 thinkingLevel
// (uppercase wire values) and reasoning-part streaming.
//
// Endpoint: POST {baseURL}/models/{model}:streamGenerateContent?alt=sse
// Auth: x-goog-api-key header (or Authorization: Bearer).
type GoogleGenerativeProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// ProviderName overrides the value returned by Name().
	ProviderName string
	// EffortFormat / EffortLevels: same semantic-effort translation contract
	// as OpenAICompatibleProvider. Defaults to EffortGoogle when unset.
	EffortFormat EffortWireFormat
	EffortLevels EffortLevelMap
}

// Name returns the provider identifier.
func (p *GoogleGenerativeProvider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return "google"
}

// CacheCapability returns the prefix-cache profile for this provider.
func (p *GoogleGenerativeProvider) CacheCapability() CacheCapability {
	return CacheCapabilityFor(p.Name())
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback.
func (p *GoogleGenerativeProvider) effectiveHTTPClient() *http.Client {
	return ssestream.EffectiveHTTPClient(p.HTTPClient)
}

// effortWireParams mirrors OpenAICompatibleProvider.effortWireParams. With no
// level map the raw value passes through as thinkingConfig (legacy raw).
func (p *GoogleGenerativeProvider) effortWireParams(level any) map[string]any {
	if level == nil {
		return nil
	}
	format := p.EffortFormat
	if format == "" {
		format = EffortGoogle
	}
	if p.EffortLevels == nil {
		if s, ok := level.(string); ok {
			return map[string]any{"thinkingConfig": map[string]any{"thinkingLevel": s}}
		}
		return nil
	}
	s, ok := level.(string)
	if !ok {
		return nil
	}
	return TranslateEffort(format, s, p.EffortLevels)
}

// applyEffortProfile wires registry protocol facts into the provider.
func (p *GoogleGenerativeProvider) applyEffortProfile(mp ModelProfile) {
	if mp.EffortFormat != "" {
		p.EffortFormat = mp.EffortFormat
	}
	if mp.EffortLevels != nil {
		p.EffortLevels = mp.EffortLevels
	}
}

// GenerateRequestData converts a ModelRequest into Google's request shape.
// System → systemInstruction; history + user → contents; tools → Google
// functionDeclaration envelope.
func (p *GoogleGenerativeProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	if req == nil {
		return nil, fmt.Errorf("model request cannot be nil")
	}
	model := p.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	options := make(map[string]any, len(req.Options)+2)
	for k, v := range req.Options {
		options[k] = v
	}
	options["_google_system"] = req.System
	options["_google_developer"] = req.Developer
	// MaxTokens from Options["max_tokens"] (mirrors OpenAICompatibleProvider).
	maxTokens := 0
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
	return &RequestData{
		Model:       model,
		Messages:    append(append([]ChatMessage{}, req.ChatHistory...), ChatMessage{Role: "user", Content: req.Input}),
		Tools:       req.Tools,
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
		Options:     options,
	}, nil
}

// googleRole converts an inferglow role to a Google content role. Google uses
// "user" / "model"; tool results are emitted as user messages.
func googleRole(role string) string {
	switch role {
	case "assistant", "model":
		return "model"
	case "system":
		return "user"
	default: // user, tool, function
		return "user"
	}
}

// googleContents builds the contents array from messages.
func googleContents(msgs []ChatMessage) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		role := googleRole(m.Role)
		parts := []map[string]any{}
		if m.Content != "" {
			parts = append(parts, map[string]any{"text": m.Content})
		}
		// Tool calls / results: Google represents tool results as function
		// response parts in a user message.
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": tc.Name,
					"args": json.RawMessage(args),
				},
			})
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, map[string]any{"role": role, "parts": parts})
	}
	return out
}

// googleTools converts ToolDefinition into Google functionDeclaration.
func googleTools(tools []ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"functionDeclarations": []map[string]any{
				{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			},
		})
	}
	return out
}

// RequestModel sends a streamGenerateContent request and returns the stream.
func (p *GoogleGenerativeProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("GoogleGenerativeProvider.BaseURL is empty; configure base_url")
	}

	// Build request body.
	contents := googleContents(data.Messages)
	generationConfig := map[string]any{}
	if data.Temperature > 0 {
		generationConfig["temperature"] = data.Temperature
	}
	if data.MaxTokens > 0 {
		generationConfig["maxOutputTokens"] = data.MaxTokens
	}
	// Apply effort wire (thinkingConfig) from Options["reasoning_effort"].
	if len(data.Options) > 0 {
		if v, ok := data.Options["reasoning_effort"]; ok {
			if wire := p.effortWireParams(v); wire != nil {
				if tc, ok := wire["thinkingConfig"]; ok {
					generationConfig["thinkingConfig"] = tc
				}
			}
		}
	}

	system := ""
	if v, ok := data.Options["_google_system"]; ok {
		if s, ok := v.(string); ok {
			system = s
		}
	}
	body := map[string]any{
		"contents": contents,
	}
	if len(generationConfig) > 0 {
		body["generationConfig"] = generationConfig
	}
	if system != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}
	if tools := googleTools(data.Tools); tools != nil {
		body["tools"] = tools
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	// Endpoint: {baseURL}/models/{model}:streamGenerateContent?alt=sse
	modelID := data.Model
	url := strings.TrimRight(baseURL, "/") + "/models/" + url.PathEscape(modelID) + ":streamGenerateContent?alt=sse"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("x-goog-api-key", p.APIKey)
	}

	client := p.effectiveHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var usage *UsageInfo
	parse := func(line string, emit func(*StreamChunk)) bool {
		if u := p.processGoogleLine(line, usage, emit); u != nil {
			usage = u
		}
		return false
	}
	errorChunk := func(err error) *StreamChunk {
		return &StreamChunk{IsDone: true, Meta: map[string]any{"error": err.Error()}}
	}
	return ssestream.RunLines(ctx, resp.Body, parse, errorChunk), nil
}

// googleStreamPart mirrors a Google GenerateContentResponse part.
type googleStreamPart struct {
	Text        string        `json:"text,omitempty"`
	Thought     bool          `json:"thought,omitempty"`
	FunctionCall *json.RawMessage `json:"functionCall,omitempty"`
}

// googleStreamContent mirrors a Google content block in a streamed chunk.
type googleStreamContent struct {
	Parts []googleStreamPart `json:"parts"`
}

// googleStreamCandidate mirrors a candidate in a streamed chunk.
type googleStreamCandidate struct {
	Content      *googleStreamContent `json:"content"`
	FinishReason string               `json:"finishReason"`
}

// googleUsageMetadata mirrors usage in a streamed chunk.
type googleUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
}

// googleStreamChunk is the top-level SSE data payload.
type googleStreamChunk struct {
	Candidates    []googleStreamCandidate `json:"candidates"`
	UsageMetadata *googleUsageMetadata    `json:"usageMetadata"`
}

// processGoogleLine parses one SSE data line and emits StreamChunks. Returns
// a new usage pointer when the chunk carried usage metadata (else nil).
func (p *GoogleGenerativeProvider) processGoogleLine(line string, usage *UsageInfo, emit func(*StreamChunk)) *UsageInfo {
	payload, ok := ssestream.ParseDataLine(line)
	if !ok || payload == "" {
		return nil
	}
	if payload == "[DONE]" {
		emit(&StreamChunk{IsDone: true})
		return nil
	}
	var chunk googleStreamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil
	}
	if len(chunk.Candidates) > 0 && chunk.Candidates[0].Content != nil {
		for _, part := range chunk.Candidates[0].Content.Parts {
			if part.Text == "" {
				continue
			}
			if part.Thought {
				emit(&StreamChunk{Reasoning: part.Text})
			} else {
				emit(&StreamChunk{Delta: part.Text})
			}
		}
	}
	if chunk.UsageMetadata != nil {
		u := &UsageInfo{
			PromptTokens:     chunk.UsageMetadata.PromptTokenCount - chunk.UsageMetadata.CachedContentTokenCount,
			CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount,
			TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			CompletionTokensDetails: map[string]int{
				"reasoning_tokens": chunk.UsageMetadata.ThoughtsTokenCount,
				"cached_tokens":    chunk.UsageMetadata.CachedContentTokenCount,
			},
		}
		return u
	}
	return nil
}

// BroadcastResponse fans a StreamChunk stream into ResultEvents (same
// contract as the other providers).
func (p *GoogleGenerativeProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 64)
	go func() {
		defer close(events)
		var fullContent strings.Builder
		var fullReasoning strings.Builder
		var lastUsage *UsageInfo
		for chunk := range stream {
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if chunk.IsDone {
				if meta, ok := chunk.Meta["error"]; ok {
					events <- &ResultEvent{EventType: ErrorEvent, Payload: meta}
					continue
				}
				resp := &ModelResponse{Content: fullContent.String(), Reasoning: fullReasoning.String()}
				if lastUsage != nil {
					resp.Usage = *lastUsage
					resp.ReasoningTokens = lastUsage.ReasoningTokens()
				}
				// G1-04: defensive <think> tag normalization.
				if resp.Reasoning == "" && hasThinkingTags(resp.Content) {
					reasoning, cleaned := normalizeThinkingTags(resp.Content)
					resp.Reasoning = reasoning
					resp.Content = cleaned
				}
				events <- &ResultEvent{EventType: EventDone, Payload: resp}
				continue
			}
			if chunk.Reasoning != "" {
				fullReasoning.WriteString(chunk.Reasoning)
				events <- &ResultEvent{EventType: ReasoningDelta, Payload: chunk.Reasoning}
			}
			if chunk.Delta != "" {
				fullContent.WriteString(chunk.Delta)
				events <- &ResultEvent{EventType: EventDelta, Payload: chunk.Delta}
			}
		}
	}()
	return events, nil
}
