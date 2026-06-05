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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIResponses_GenerateRequestData covers the request-body construction
// contract for the OpenAI Responses API:
//  - messages (non-system) are rewritten into `input` array
//  - system message becomes `instructions` field
//  - Tools are passed through in OpenAI envelope format
func TestOpenAIResponses_GenerateRequestData(t *testing.T) {
	t.Run("rewrites_messages_into_input_with_instructions", func(t *testing.T) {
		p := &OpenAIResponsesProvider{Model: "gpt-4o"}
		req := &ModelRequest{
			System:    "You are a helpful assistant.",
			Instruct:  "Calculate 2+2.",
			ChatHistory: []ChatMessage{
				{Role: "user", Content: "previous question"},
				{Role: "assistant", Content: "previous answer"},
			},
		}

		data, err := p.GenerateRequestData(context.Background(), req)
		if err != nil {
			t.Fatalf("GenerateRequestData failed: %v", err)
		}
		if data.Model != "gpt-4o" {
			t.Errorf("Model = %q, want gpt-4o", data.Model)
		}
		// Instructions carries the system prompt.
		if got, _ := data.Options["instructions"].(string); got != "You are a helpful assistant." {
			t.Errorf("Options[instructions] = %q, want system prompt", got)
		}
		// Messages should carry the rewritten input (user+assistant+user).
		// We don't pin the exact shape — the RequestModel body builder consumes
		// data.Messages and emits the `input` field. The contract here is that
		// the system message is NOT in Messages (it's in instructions) and
		// the user/assistant turns are preserved.
		if len(data.Messages) < 2 {
			t.Errorf("expected at least 2 messages (history + current user), got %d", len(data.Messages))
		}
		for _, m := range data.Messages {
			if m.Role == "system" {
				t.Errorf("system message must be moved to instructions, found role=%q content=%q", m.Role, m.Content)
			}
		}
	})

	t.Run("tools_passed_through_in_openai_envelope", func(t *testing.T) {
		p := &OpenAIResponsesProvider{Model: "gpt-4o"}
		req := &ModelRequest{
			Instruct: "hi",
			Tools: []ToolDefinition{
				{Name: "calc", Description: "calc", Parameters: map[string]any{"type": "object"}},
			},
		}
		data, err := p.GenerateRequestData(context.Background(), req)
		if err != nil {
			t.Fatalf("GenerateRequestData failed: %v", err)
		}
		if len(data.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(data.Tools))
		}
		if data.Tools[0].Name != "calc" {
			t.Errorf("tool name = %q, want calc", data.Tools[0].Name)
		}
	})

	t.Run("nil_request_returns_error", func(t *testing.T) {
		p := &OpenAIResponsesProvider{Model: "gpt-4o"}
		_, err := p.GenerateRequestData(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil request")
		}
	})
}

// TestOpenAIResponses_RequestModel_URL covers URL resolution:
//   - default: BaseURL + /responses
//   - FullURL override: hits FullURL exactly
func TestOpenAIResponses_RequestModel_URL(t *testing.T) {
	t.Run("default_url_concatenation", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"type\":\"response.completed\"}\n\ndata: [DONE]\n\n"))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/responses" {
			t.Errorf("request hit path %q, want /responses", hitPath)
		}
	})

	t.Run("full_url_override", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"type\":\"response.completed\"}\n\ndata: [DONE]\n\n"))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{
			BaseURL: "https://api.openai.com/v1",
			FullURL: server.URL + "/proxy-responses",
			Model:   "gpt-4o",
		}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/proxy-responses" {
			t.Errorf("request hit path %q, want /proxy-responses (FullURL override)", hitPath)
		}
	})

	t.Run("request_body_has_input_and_instructions", func(t *testing.T) {
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"type\":\"response.completed\"}\n\ndata: [DONE]\n\n"))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{
			Model:    "gpt-4o",
			Messages: []ChatMessage{{Role: "user", Content: "hi"}},
			Options: map[string]any{
				"instructions": "You are a helpful assistant.",
			},
		}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if body["model"] != "gpt-4o" {
			t.Errorf("body[model] = %v, want gpt-4o", body["model"])
		}
		if body["stream"] != true {
			t.Errorf("body[stream] = %v, want true", body["stream"])
		}
		if body["input"] == nil {
			t.Error("body[input] is missing")
		}
		if body["instructions"] != "You are a helpful assistant." {
			t.Errorf("body[instructions] = %v, want system prompt", body["instructions"])
		}
	})
}

// TestOpenAIResponses_StreamParsing covers SSE event handling:
//   - response.output_text.delta → StreamChunk.Delta
//   - response.completed → StreamChunk.IsDone = true
//   - [DONE] → stream terminates
func TestOpenAIResponses_StreamParsing(t *testing.T) {
	t.Run("output_text_delta_emits_stream_chunk_delta", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			sse := strings.Join([]string{
				`data: {"type":"response.output_text.delta","delta":"Hello"}`,
				``,
				`data: {"type":"response.output_text.delta","delta":" world"}`,
				``,
				`data: {"type":"response.completed"}`,
				``,
				`data: [DONE]`,
				``,
				``,
			}, "\n")
			w.Write([]byte(sse))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}

		var deltas []string
		var sawDone bool
		for chunk := range stream {
			if chunk.Delta != "" {
				deltas = append(deltas, chunk.Delta)
			}
			if chunk.IsDone {
				sawDone = true
			}
		}

		if got, want := strings.Join(deltas, ""), "Hello world"; got != want {
			t.Errorf("delta stream = %q, want %q", got, want)
		}
		if !sawDone {
			t.Error("expected IsDone chunk on response.completed")
		}
	})
}

// TestOpenAIResponses_ReasoningSummary covers the reasoning summary extraction
// at response.completed time:
//  - string format: reasoning.summary == "I thought about this"
//  - list format:   reasoning.summary == [{"type":"summary_text","text":"step1"}, ...]
//  - missing:       Reasoning stays empty
func TestOpenAIResponses_ReasoningSummary(t *testing.T) {
	t.Run("string_summary", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			sse := strings.Join([]string{
				`data: {"type":"response.output_text.delta","delta":"answer"}`,
				``,
				`data: {"type":"response.completed","response":{"reasoning":{"summary":"I thought about this"}}}`,
				``,
				`data: [DONE]`,
				``,
				``,
			}, "\n")
			w.Write([]byte(sse))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}

		events, err := p.BroadcastResponse(context.Background(), stream)
		if err != nil {
			t.Fatalf("BroadcastResponse failed: %v", err)
		}

		var resp *ModelResponse
		for ev := range events {
			if ev.EventType == EventDone {
				if mr, ok := ev.Payload.(*ModelResponse); ok {
					resp = mr
				}
			}
		}
		if resp == nil {
			t.Fatal("expected EventDone with ModelResponse payload")
		}
		if resp.Reasoning != "I thought about this" {
			t.Errorf("Reasoning = %q, want %q", resp.Reasoning, "I thought about this")
		}
		if resp.Content != "answer" {
			t.Errorf("Content = %q, want %q", resp.Content, "answer")
		}
	})

	t.Run("list_summary_joined_by_newline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			sse := strings.Join([]string{
				`data: {"type":"response.output_text.delta","delta":"answer"}`,
				``,
				`data: {"type":"response.completed","response":{"reasoning":{"summary":[{"type":"summary_text","text":"step1"},{"type":"summary_text","text":"step2"}]}}}`,
				``,
				`data: [DONE]`,
				``,
				``,
			}, "\n")
			w.Write([]byte(sse))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		events, err := p.BroadcastResponse(context.Background(), stream)
		if err != nil {
			t.Fatalf("BroadcastResponse failed: %v", err)
		}

		var resp *ModelResponse
		for ev := range events {
			if ev.EventType == EventDone {
				if mr, ok := ev.Payload.(*ModelResponse); ok {
					resp = mr
				}
			}
		}
		if resp == nil {
			t.Fatal("expected EventDone with ModelResponse payload")
		}
		if resp.Reasoning != "step1\nstep2" {
			t.Errorf("Reasoning = %q, want %q", resp.Reasoning, "step1\nstep2")
		}
	})

	t.Run("missing_reasoning_yields_empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			sse := strings.Join([]string{
				`data: {"type":"response.output_text.delta","delta":"answer"}`,
				``,
				`data: {"type":"response.completed","response":{}}`,
				``,
				`data: [DONE]`,
				``,
				``,
			}, "\n")
			w.Write([]byte(sse))
		}))
		defer server.Close()

		p := &OpenAIResponsesProvider{BaseURL: server.URL, Model: "gpt-4o"}
		data := &RequestData{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := p.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		events, err := p.BroadcastResponse(context.Background(), stream)
		if err != nil {
			t.Fatalf("BroadcastResponse failed: %v", err)
		}

		var resp *ModelResponse
		for ev := range events {
			if ev.EventType == EventDone {
				if mr, ok := ev.Payload.(*ModelResponse); ok {
					resp = mr
				}
			}
		}
		if resp == nil {
			t.Fatal("expected EventDone with ModelResponse payload")
		}
		if resp.Reasoning != "" {
			t.Errorf("Reasoning = %q, want empty", resp.Reasoning)
		}
	})
}

// TestOpenAIResponses_Name confirms the default provider name.
func TestOpenAIResponses_Name(t *testing.T) {
	p := &OpenAIResponsesProvider{}
	if got, want := p.Name(), "openai-responses"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}

	p = &OpenAIResponsesProvider{ProviderName: "custom"}
	if got, want := p.Name(), "custom"; got != want {
		t.Errorf("Name() = %q, want %q (ProviderName override)", got, want)
	}
}
