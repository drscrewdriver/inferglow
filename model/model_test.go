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
	"encoding/json"
	"testing"
)

// Check 1.1.1: ChatMessage 类型定义完整
func TestChatMessageSerialization(t *testing.T) {
	msg := ChatMessage{
		Role:    "user",
		Content: "hello world",
		Name:    "test",
		ToolCalls: []ToolCall{
			{ID: "1", Name: "test_tool", Arguments: map[string]any{"key": "value"}},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ChatMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Role != msg.Role || decoded.Content != msg.Content || decoded.Name != msg.Name {
		t.Errorf("mismatch: got %+v, want %+v", decoded, msg)
	}

	if len(decoded.ToolCalls) != 1 || decoded.ToolCalls[0].Name != "test_tool" {
		t.Errorf("unexpected tool calls: %+v", decoded.ToolCalls)
	}
}

// Check 1.1.2: ToolCall/ToolDefinition 类型定义完整
func TestToolCallAndDefinitionSerialization(t *testing.T) {
	def := ToolDefinition{
		Name:        "get_weather",
		Description: "Get the current weather",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{"type": "string"},
			},
		},
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal ToolDefinition failed: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal ToolDefinition failed: %v", err)
	}

	if decoded.Name != def.Name || decoded.Description != def.Description {
		t.Errorf("mismatch: got %+v, want %+v", decoded, def)
	}
}

// Check 1.1.3: ModelRequest 包含所有必要字段
func TestModelRequestFields(t *testing.T) {
	req := ModelRequest{
		System:       "You are a helpful assistant.",
		Instruct:     "What is 2+2?",
		OutputFormat: "json",
		ChatHistory: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
		Info:        map[string]any{"user_id": "123"},
		Tools:       []ToolDefinition{{Name: "calc", Description: "calculator", Parameters: map[string]any{}}},
		EnsureAll:   true,
		Options:     map[string]any{"timeout": 30},
		Model:       "gpt-4",
		Temperature: 0.7,
	}

	if req.System == "" || req.Instruct == "" || req.OutputFormat == "" {
		t.Errorf("missing required fields: %+v", req)
	}

	if len(req.ChatHistory) != 1 {
		t.Errorf("expected 1 chat history, got %d", len(req.ChatHistory))
	}
}

// Check 1.1.4: ModelResponse 包含 Content/Reasoning/Tools/Usage 字段
func TestModelResponseFields(t *testing.T) {
	resp := ModelResponse{
		Content:   "The answer is 4.",
		Reasoning: "I calculated 2+2.",
		Tools:     []ToolCall{{ID: "1", Name: "calc", Arguments: map[string]any{"expression": "2+2"}}},
		Usage: UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Meta: map[string]any{"latency_ms": 100},
	}

	if resp.Content == "" || resp.Reasoning == "" || len(resp.Tools) == 0 {
		t.Errorf("missing response fields: %+v", resp)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal ModelResponse failed: %v", err)
	}

	var decoded ModelResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal ModelResponse failed: %v", err)
	}

	if decoded.Content != resp.Content {
		t.Errorf("content mismatch: got %q, want %q", decoded.Content, resp.Content)
	}
}

// Check 1.1.5: StreamChunk 包含 Delta/IsDone/Usage 字段
func TestStreamChunkFields(t *testing.T) {
	usage := &UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	chunk := StreamChunk{
		Delta:     "hello",
		Reasoning: "thinking...",
		IsDone:    false,
		Usage:     usage,
		Meta:      map[string]any{"chunk_id": 1},
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal StreamChunk failed: %v", err)
	}

	var decoded StreamChunk
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal StreamChunk failed: %v", err)
	}

	if decoded.Delta != chunk.Delta || decoded.IsDone != chunk.IsDone {
		t.Errorf("mismatch: got %+v, want %+v", decoded, chunk)
	}
}

// Check 1.1.6: ResultEventType 常量定义完整
func TestResultEventTypeConstants(t *testing.T) {
	expected := []ResultEventType{
		EventDelta, EventDone, ErrorEvent, MetaEvent, StatusEvent,
		ToolCallsEvent, ReasoningDelta, ReasoningDone,
		OriginalDelta, OriginalDone,
	}

	for _, evt := range expected {
		if string(evt) == "" {
			t.Errorf("empty event type: %v", evt)
		}
	}
}

// Check 1.1.7: 类型可正确序列化/反序列化（JSON）
func TestAllTypesJSONRoundTrip(t *testing.T) {
	// 测试所有核心类型的 JSON round-trip
	tests := []struct {
		name string
		data any
	}{
		{"ChatMessage", ChatMessage{Role: "assistant", Content: "hi"}},
		{"ToolCall", ToolCall{ID: "1", Name: "test", Arguments: map[string]any{"a": 1}}},
		{"ToolDefinition", ToolDefinition{Name: "test", Description: "desc", Parameters: map[string]any{}}},
		{"UsageInfo", UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{"ModelResponse", ModelResponse{Content: "test", Usage: UsageInfo{TotalTokens: 15}}},
		{"StreamChunk", StreamChunk{Delta: "test", IsDone: true}},
		{"ResultEvent", ResultEvent{EventType: EventDelta, Payload: "test"}},
		{"OutputSchema", OutputSchema{Type: "object", Properties: map[string]any{}, Required: []string{"name"}}},
		{"ActionResult", ActionResult{Name: "test", Success: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			// 验证 JSON 格式有效
			var js map[string]any
			if err := json.Unmarshal(data, &js); err != nil {
				t.Fatalf("json not valid: %v", err)
			}
		})
	}
}
