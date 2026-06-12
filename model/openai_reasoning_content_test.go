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
	"testing"
)

// TestProcessOpenAILineReasoningContentOnly verifies that an SSE chunk
// containing only reasoning_content (no reasoning) correctly fills
// StreamChunk.Reasoning. Delta should remain empty.
func TestProcessOpenAILineReasoningContentOnly(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning_content":"思考过程"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "思考过程" {
		t.Errorf("Reasoning=%q, want %q", got.Reasoning, "思考过程")
	}
	if got.Delta != "" {
		t.Errorf("Delta=%q, want empty", got.Delta)
	}
}

// TestProcessOpenAILineReasoningContentOverridesReasoning verifies that
// when a chunk contains both reasoning and reasoning_content, the
// reasoning_content value wins (higher priority).
func TestProcessOpenAILineReasoningContentOverridesReasoning(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning":"低优先级","reasoning_content":"高优先级"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "高优先级" {
		t.Errorf("Reasoning=%q, want %q (reasoning_content should override reasoning)", got.Reasoning, "高优先级")
	}
}

// TestProcessOpenAILineReasoningBackwardCompat verifies that a chunk
// containing only reasoning (no reasoning_content) still works — backward
// compatibility with standard OpenAI/DeepSeek/Qwen providers.
func TestProcessOpenAILineReasoningBackwardCompat(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning":"标准推理"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "标准推理" {
		t.Errorf("Reasoning=%q, want %q", got.Reasoning, "标准推理")
	}
}

// TestToStreamChunkReasoningContent verifies that toStreamChunk (the
// backward-compat helper) also handles reasoning_content with the same
// priority override.
func TestToStreamChunkReasoningContent(t *testing.T) {
	reasoningVal := "低优先级"
	reasoningContentVal := "高优先级"

	chunk := openAIChunk{
		Choices: []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string     `json:"role,omitempty"`
				Content          *string    `json:"content,omitempty"`
				Reasoning        *string    `json:"reasoning,omitempty"`
				ReasoningContent *string    `json:"reasoning_content,omitempty"`
				ReasoningDetails *string    `json:"reasoning_details,omitempty"`
				ToolCalls        []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role             string     `json:"role,omitempty"`
					Content          *string    `json:"content,omitempty"`
					Reasoning        *string    `json:"reasoning,omitempty"`
					ReasoningContent *string    `json:"reasoning_content,omitempty"`
					ReasoningDetails *string    `json:"reasoning_details,omitempty"`
					ToolCalls        []toolCall `json:"tool_calls,omitempty"`
				}{
					Reasoning:        &reasoningVal,
					ReasoningContent: &reasoningContentVal,
				},
			},
		},
	}

	result := toStreamChunk(chunk, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Reasoning != "高优先级" {
		t.Errorf("Reasoning=%q, want %q (reasoning_content should override reasoning)", result.Reasoning, "高优先级")
	}
}

// TestToStreamChunkReasoningContentOnly verifies toStreamChunk with only
// reasoning_content (no reasoning field).
func TestToStreamChunkReasoningContentOnly(t *testing.T) {
	reasoningContentVal := "仅 reasoning_content"

	chunk := openAIChunk{
		Choices: []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string     `json:"role,omitempty"`
				Content          *string    `json:"content,omitempty"`
				Reasoning        *string    `json:"reasoning,omitempty"`
				ReasoningContent *string    `json:"reasoning_content,omitempty"`
				ReasoningDetails *string    `json:"reasoning_details,omitempty"`
				ToolCalls        []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role             string     `json:"role,omitempty"`
					Content          *string    `json:"content,omitempty"`
					Reasoning        *string    `json:"reasoning,omitempty"`
					ReasoningContent *string    `json:"reasoning_content,omitempty"`
					ReasoningDetails *string    `json:"reasoning_details,omitempty"`
					ToolCalls        []toolCall `json:"tool_calls,omitempty"`
				}{
					ReasoningContent: &reasoningContentVal,
				},
			},
		},
	}

	result := toStreamChunk(chunk, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Reasoning != "仅 reasoning_content" {
		t.Errorf("Reasoning=%q, want %q", result.Reasoning, "仅 reasoning_content")
	}
}

// --- G1-04: <think> tag normalization tests ---

// TestHasThinkingTags is a table-driven test for hasThinkingTags.
func TestHasThinkingTags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"complete block", "<think>x</think>y", true},
		{"plain text", "纯文本", false},
		{"open tag only no close", "<think>无闭合", false},
		{"close tag only no open", "无开始</think>", false},
		{"empty think block", "<think></think>", true},
		{"multiple blocks", "<think>a</think>b<think>c</think>", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasThinkingTags(tt.content)
			if got != tt.want {
				t.Errorf("hasThinkingTags(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestNormalizeThinkingTags is a table-driven test for normalizeThinkingTags.
func TestNormalizeThinkingTags(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantReasoning string
		wantCleaned   string
	}{
		{
			name:          "simple block",
			content:       "<think>推理</think>回答",
			wantReasoning: "推理",
			wantCleaned:   "回答",
		},
		{
			name:          "no tags",
			content:       "无标签",
			wantReasoning: "",
			wantCleaned:   "无标签",
		},
		{
			name:          "multiple blocks joined",
			content:       "<think>第一段</think>中间<think>第二段</think>结尾",
			wantReasoning: "第一段\n第二段",
			wantCleaned:   "中间结尾",
		},
		{
			name:          "whitespace trimmed cleaned empty",
			content:       "  <think>推理</think>  ",
			wantReasoning: "推理",
			wantCleaned:   "",
		},
		{
			name:          "empty think block",
			content:       "<think></think>回答",
			wantReasoning: "",
			wantCleaned:   "回答",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReasoning, gotCleaned := normalizeThinkingTags(tt.content)
			if gotReasoning != tt.wantReasoning {
				t.Errorf("reasoning = %q, want %q", gotReasoning, tt.wantReasoning)
			}
			if gotCleaned != tt.wantCleaned {
				t.Errorf("cleaned = %q, want %q", gotCleaned, tt.wantCleaned)
			}
		})
	}
}

// TestBroadcastResponseThinkTagNormalization is an end-to-end test: feed
// a StreamChunk stream whose Delta accumulates <think>...</think> content,
// and verify BroadcastResponse normalizes it into Reasoning/Content.
func TestBroadcastResponseThinkTagNormalization(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Delta: "<think>推理过程</think>最终回答"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if donePayload.Reasoning != "推理过程" {
		t.Errorf("Reasoning = %q, want %q", donePayload.Reasoning, "推理过程")
	}
	if donePayload.Content != "最终回答" {
		t.Errorf("Content = %q, want %q", donePayload.Content, "最终回答")
	}
}

// TestBroadcastResponseNoNormalizationWhenReasoningPresent verifies that
// when the standard reasoning field is already filled (via reasoning/
// reasoning_content delta), the <think> tag normalization is NOT triggered
// (avoids double processing).
func TestBroadcastResponseNoNormalizationWhenReasoningPresent(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	// Standard reasoning delta fills fullReasoning, so normalization should
	// be skipped even though Content contains <think> tags.
	stream <- &StreamChunk{Reasoning: "标准推理", Delta: "<think>不应提取</think>回答"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	// Reasoning should be the standard field, NOT the <think> tag content.
	if donePayload.Reasoning != "标准推理" {
		t.Errorf("Reasoning = %q, want %q (standard reasoning should not be overwritten)", donePayload.Reasoning, "标准推理")
	}
	// Content should still contain the <think> tags (not normalized).
	if donePayload.Content != "<think>不应提取</think>回答" {
		t.Errorf("Content = %q, want %q (content should not be cleaned when reasoning is present)", donePayload.Content, "<think>不应提取</think>回答")
	}
}

// --- G1-03: thinking / reasoning_effort parameter pass-through tests ---

// TestThinkingParameterPassThrough verifies that Options["thinking"] is
// passed through to the request body (used by MiMo/Stepfun/Sensenova).
// thinking is not a reserved field, so it must survive the Options expansion.
func TestThinkingParameterPassThrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "mimo-v2.5-pro"}
	data := &RequestData{
		Model:    "mimo-v2.5-pro",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options: map[string]any{
			"thinking": map[string]string{"type": "enabled"},
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	thinking, ok := receivedBody["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %v, want map (Options[\"thinking\"] should pass through)", receivedBody["thinking"])
	}
	if tp, ok := thinking["type"].(string); !ok || tp != "enabled" {
		t.Errorf("thinking.type = %v, want \"enabled\"", thinking["type"])
	}
}

// TestReasoningEffortParameterPassThrough verifies that Options["reasoning_effort"]
// is passed through to the request body (used by OpenAI o-series).
func TestReasoningEffortParameterPassThrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "o1"}
	data := &RequestData{
		Model:    "o1",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options: map[string]any{
			"reasoning_effort": "high",
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	effort, ok := receivedBody["reasoning_effort"].(string)
	if !ok || effort != "high" {
		t.Errorf("reasoning_effort = %v, want \"high\" (Options[\"reasoning_effort\"] should pass through)", receivedBody["reasoning_effort"])
	}
}

// TestThinkingAndReasoningEffortNotReserved verifies that the thinking and
// reasoning_effort keys are NOT in reservedFields, so they pass through the
// Options expansion in RequestModel.
func TestThinkingAndReasoningEffortNotReserved(t *testing.T) {
	if reservedFields["thinking"] {
		t.Errorf("reservedFields[\"thinking\"] = true, want false (must pass through for MiMo/Stepfun/Sensenova)")
	}
	if reservedFields["reasoning_effort"] {
		t.Errorf("reservedFields[\"reasoning_effort\"] = true, want false (must pass through for OpenAI o-series)")
	}
}
