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

// OpenAIResponsesProvider implements ModelRequester against the OpenAI
// Responses API (`/responses` endpoint), the recommended interface for
// o-series reasoning models.
//
// Differences from OpenAICompatibleProvider (Chat Completions API):
//   - Request body uses `input` (array of {role, content}) instead of `messages`.
//   - System prompt is moved out of messages into top-level `instructions`.
//   - SSE events are typed differently:
//     `response.output_text.delta` → answer text delta
//     `response.completed`         → final event; carries `response.reasoning.summary`
//     `[DONE]`                     → stream terminator
//   - Reasoning content is NOT streamed token-by-token; it is delivered as a
//     summary at `response.completed`. This provider extracts both string and
//     list summary formats.
//   - Tool use is not yet supported. GenerateRequestData returns an explicit
//     error when req.Tools is set rather than silently dropping it.
//
// Spec: model-parity Phase 2, P0 — OpenAI Responses API Provider.
type OpenAIResponsesProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Model      string
	// ProviderName overrides the value returned by Name(). Defaults to
	// "openai-responses" when empty.
	ProviderName string
	// FullURL overrides BaseURL + "/responses" when non-empty.
	// Spec: model-parity Phase 1 — full_url 覆盖.
	FullURL string
	// RoleMapping is honored for parity with OpenAICompatibleProvider
	// (e.g. developers that map "developer" → "system"). When a role maps
	// to "system", its content is appended to `instructions` instead of
	// being emitted in the `input` array.
	RoleMapping map[string]string
}

// Name returns the provider identifier. Defaults to "openai-responses".
func (p *OpenAIResponsesProvider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return "openai-responses"
}

// effectiveHTTPClient returns the configured HTTPClient or a sane fallback
// with a 5-minute timeout (long enough for streaming responses).
func (p *OpenAIResponsesProvider) effectiveHTTPClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

// mapRole applies RoleMapping to the given role. If no mapping exists, the
// role is returned unchanged. Mirrors OpenAICompatibleProvider.mapRole so
// Responses-API callers get consistent role handling.
func (p *OpenAIResponsesProvider) mapRole(role string) string {
	if p.RoleMapping == nil {
		return role
	}
	if mapped, ok := p.RoleMapping[role]; ok && mapped != "" {
		return mapped
	}
	return role
}

// GenerateRequestData converts a ModelRequest into the Responses API shape.
//
// The transformation:
//   - System message → Options["instructions"]
//   - developer role (when mapped to "system") → merged into instructions
//   - all other messages (chat history + current user) → Messages (unchanged)
//   - Tools → rejected with explicit error
//
// The actual `input`/`instructions` body fields are emitted by RequestModel
// from this RequestData so the conversion stays close to where the body is
// built (mirrors the OpenAICompatibleProvider pattern).
func (p *OpenAIResponsesProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	model := p.Model
	if model == "" {
		model = "gpt-4o"
	}

	if req == nil {
		return nil, fmt.Errorf("model request cannot be nil")
	}

	// Reject tool use explicitly. The Responses API has a different tool-use
	// event model; full support is deferred to a later spec.
	if len(req.Tools) > 0 {
		return nil, fmt.Errorf("tool use not yet supported by OpenAIResponsesProvider")
	}

	// Build the instructions (system prompt) and the input messages.
	instructions := req.System
	if req.Developer != "" {
		developerRole := p.mapRole("developer")
		if developerRole == "system" {
			if instructions == "" {
				instructions = req.Developer
			} else {
				instructions = instructions + "\n" + req.Developer
			}
		}
		// Non-system-mapped developer role: drop it. The Responses API
		// `input` array accepts role+content pairs but "developer" is not
		// a documented role; emitting it would risk a 422.
	}

	messages := make([]ChatMessage, 0, len(req.ChatHistory)+1)
	messages = append(messages, req.ChatHistory...)

	// Build the current user message (Instruct + Input merged).
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

	// Temperature: respect caller's value. When TemperatureSet=false and
	// Temperature==0, fall back to DefaultTemperature to match the
	// OpenAICompatibleProvider convention.
	temperature := req.Temperature
	if !req.TemperatureSet && temperature == 0 {
		temperature = DefaultTemperature
	}

	options := make(map[string]any, len(req.Options)+1)
	for k, v := range req.Options {
		options[k] = v
	}
	if instructions != "" {
		options["instructions"] = instructions
	}

	return &RequestData{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		Options:     options,
	}, nil
}

// RequestModel POSTs to {BaseURL}/responses (or FullURL when set) and returns
// a StreamChunk channel. The SSE parser handles the Responses-API event types.
func (p *OpenAIResponsesProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if data == nil {
		return nil, fmt.Errorf("request data cannot be nil")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = DefaultOpenAIBaseURL
	}

	client := p.effectiveHTTPClient()

	// Spec: model-parity Phase 1 — FullURL overrides BaseURL + defaultPath.
	url := ResolveURL(baseURL, "/responses", p.FullURL)

	// Build the Responses API request body:
	//   {model, input: [{role, content}], stream: true, instructions?, ...}
	input := make([]map[string]any, 0, len(data.Messages))
	for _, m := range data.Messages {
		input = append(input, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	reqBody := map[string]any{
		"model":  data.Model,
		"input":  input,
		"stream": true,
	}
	if v, ok := data.Options["instructions"]; ok {
		if s, ok := v.(string); ok && s != "" {
			reqBody["instructions"] = s
		}
	}
	if data.Temperature > 0 {
		reqBody["temperature"] = data.Temperature
	}
	// Pass through remaining non-reserved options (excluding `instructions`
	// which we already promoted to a top-level field).
	for k, v := range data.Options {
		if k == "instructions" {
			continue
		}
		reqBody[k] = v
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
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Responses API error (status %d): %s", resp.StatusCode, string(body))
	}

	stream := make(chan *StreamChunk, 64)
	go func() {
		defer close(stream)
		defer resp.Body.Close()

		reader := bufio.NewReaderSize(resp.Body, 1024*1024)

		emit := func(schunk *StreamChunk) {
			select {
			case stream <- schunk:
			case <-ctx.Done():
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if strings.TrimSpace(line) != "" {
						p.processResponsesLine(line, emit)
					}
					return
				}
				emit(&StreamChunk{
					IsDone: true,
					Meta:   map[string]any{"error": err.Error()},
				})
				return
			}

			stop := p.processResponsesLine(line, emit)
			if stop {
				return
			}
		}
	}()

	return stream, nil
}

// processResponsesLine parses one SSE line and emits any resulting StreamChunk.
// Returns true if the stream should terminate (after `[DONE]`).
//
// Event types handled:
//   - response.output_text.delta  → StreamChunk{Delta: event.delta}
//   - response.output_text.done   → no-op (delta stream ended)
//   - response.reasoning_summary_text.delta → StreamChunk{Reasoning: event.delta}
//   - response.reasoning_summary_text.done  → no-op
//   - response.completed         → extract reasoning summary → StreamChunk{IsDone: true, Reasoning: ...}
//   - response.failed            → StreamChunk{IsDone: true, Meta: {error: ...}}
//   - [DONE]                     → stream terminator (return true)
func (p *OpenAIResponsesProvider) processResponsesLine(line string, emit func(*StreamChunk)) bool {
	if !strings.HasPrefix(line, "data: ") {
		return false
	}
	data := strings.TrimPrefix(line, "data: ")
	data = strings.TrimSpace(data)
	if data == "" {
		return false
	}
	if data == "[DONE]" {
		// Stream terminator. The final `response.completed` chunk (if any)
		// has already been emitted. Terminate without emitting another IsDone.
		return true
	}

	// Parse as generic map so we can handle arbitrary event shapes without
	// declaring a struct for every variant.
	var event map[string]any
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return false
	}

	eventType, _ := event["type"].(string)
	switch eventType {
	case "response.output_text.delta":
		delta, _ := event["delta"].(string)
		if delta != "" {
			emit(&StreamChunk{Delta: delta})
		}
	case "response.output_text.done":
		// No payload to emit; the accumulated text is already streamed.
	case "response.reasoning_summary_text.delta":
		delta, _ := event["delta"].(string)
		if delta != "" {
			emit(&StreamChunk{Reasoning: delta})
		}
	case "response.reasoning_summary_text.done":
		// No-op; summary text was streamed via the delta events above.
	case "response.completed":
		// Final event. Extract reasoning summary from the nested `response`
		// object if present.
		reasoning := ""
		if respObj, ok := event["response"].(map[string]any); ok {
			reasoning = extractReasoningSummary(respObj)
		}
		emit(&StreamChunk{IsDone: true, Reasoning: reasoning})
	case "response.failed":
		errMsg := "response failed"
		if errObj, ok := event["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				errMsg = msg
			}
		}
		emit(&StreamChunk{IsDone: true, Meta: map[string]any{"error": errMsg}})
		return true
	}
	return false
}

// extractReasoningSummary extracts the reasoning summary from a `response`
// object emitted by `response.completed`. The summary field may be:
//   - a string (returned as-is)
//   - a list of {"type":"summary_text","text":"..."} objects (joined by "\n")
//
// Returns an empty string when reasoning or summary is absent.
func extractReasoningSummary(resp map[string]any) string {
	reasoningObj, ok := resp["reasoning"].(map[string]any)
	if !ok {
		return ""
	}
	summary, ok := reasoningObj["summary"]
	if !ok {
		return ""
	}
	switch s := summary.(type) {
	case string:
		return s
	case []any:
		var parts []string
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// BroadcastResponse converts the StreamChunk channel into a ResultEvent
// channel, accumulating delta text into Content and reasoning deltas into
// Reasoning.
//
// For the Responses API:
//   - output_text.delta chunks accumulate into Content
//   - reasoning_summary_text.delta chunks (if any) accumulate into Reasoning
//   - response.completed's reasoning summary (if any) is appended to Reasoning
//     when the streamed reasoning is empty (the summary is the canonical source)
//
// Unlike the Chat Completions provider, BroadcastResponse does NOT invoke
// normalizeThinkingTags on completion: Responses-API reasoning is delivered
// via the summary field, never embedded inside the answer content, so there
// is no `<think>` tag to strip.
func (p *OpenAIResponsesProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
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
					events <- &ResultEvent{
						EventType: ErrorEvent,
						Payload:   meta,
					}
					continue
				}
				// response.completed may carry a reasoning summary. If we
				// already streamed reasoning via reasoning_summary_text.delta,
				// prefer the streamed content (more granular). Otherwise use
				// the summary from the completed event.
				if chunk.Reasoning != "" && fullReasoning.Len() == 0 {
					fullReasoning.WriteString(chunk.Reasoning)
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
