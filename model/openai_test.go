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

// Check 1.5.1: GenerateRequestData 正确转换 ModelRequest → RequestData
func TestGenerateRequestData(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}

	req := &ModelRequest{
		System:   "You are helpful.",
		Instruct: "Answer this question.",
		Input:    "What is 2+2?",
		Tools: []ToolDefinition{
			{Name: "calc", Description: "calculator", Parameters: map[string]any{}},
		},
		Temperature: 0.8,
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if data.Model != "gpt-4" {
		t.Errorf("wrong model: got %q", data.Model)
	}

	// 检查 messages 顺序
	if len(data.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(data.Messages))
	}

	// 第一条是 system
	if data.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %q", data.Messages[0].Role)
	}

	// 最后一条是 user
	if data.Messages[len(data.Messages)-1].Role != "user" {
		t.Errorf("last message should be user, got %q", data.Messages[len(data.Messages)-1].Role)
	}

	// 检查 tools
	if len(data.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(data.Tools))
	}

	// 检查 temperature
	if data.Temperature != 0.8 {
		t.Errorf("expected temperature 0.8, got %f", data.Temperature)
	}
}

// Check 1.5.2: 支持 OpenAI API 格式
func TestGenerateRequestDataDefaultTemperature(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}

	req := &ModelRequest{
		System: "test",
		Input:  "test input",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// 默认温度应为 0.7
	if data.Temperature != 0.7 {
		t.Errorf("expected default temperature 0.7, got %f", data.Temperature)
	}
}

// Check 1.5.3: RequestModel 发送 HTTP POST 请求（mock server）
func TestRequestModelMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", auth)
		}

		// 读取请求体
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}

		if body["stream"] != true {
			t.Error("expected stream=true")
		}

		// 返回 mock SSE 响应
		w.Header().Set("Content-Type", "text/event-stream")
		sse := `data: {"choices":[{"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}
data: [DONE]
`
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4",
	}

	data := &RequestData{
		Model:       "gpt-4",
		Messages:    []ChatMessage{{Role: "user", Content: "hi"}},
		Temperature: 0.7,
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	// 读取流
	var chunks []*StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// 合并内容
	var fullContent strings.Builder
	for _, c := range chunks {
		fullContent.WriteString(c.Delta)
	}

	if !strings.Contains(fullContent.String(), "Hello") {
		t.Errorf("expected 'Hello' in response, got %q", fullContent.String())
	}
}

// Check 1.5.4: SSE 流式响应
func TestSSEStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := `data: {"choices":[{"delta":{"content":"part1"},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":"part2"},"finish_reason":"stop"}]}
data: [DONE]
`
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "test"}}}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	chunkCount := 0
	for range stream {
		chunkCount++
	}

	if chunkCount == 0 {
		t.Error("expected at least one chunk")
	}
}

// Check 1.5.5: BroadcastResponse 正确解析 SSE 事件
func TestBroadcastResponse(t *testing.T) {
	stream := make(chan *StreamChunk, 10)
	stream <- &StreamChunk{Delta: "Hello", IsDone: false}
	stream <- &StreamChunk{Delta: " world", IsDone: true}
	close(stream)

	provider := &OpenAICompatibleProvider{}
	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var deltas, doneEvent int
	for event := range events {
		switch event.EventType {
		case EventDelta:
			deltas++
		case EventDone:
			doneEvent++
		}
	}

	if deltas == 0 {
		t.Error("expected at least one delta event")
	}
	if doneEvent == 0 {
		t.Error("expected at least one done event")
	}
}

// Check 1.5.6: Function Calling 支持
func TestFunctionCalling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 读取请求体，验证 tools 被正确传递
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if _, hasTools := body["tools"]; !hasTools {
			t.Error("expected tools in request body")
		}

		// 返回带有 tool_calls 的响应
		w.Header().Set("Content-Type", "text/event-stream")
		sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"calc","arguments":"{\"expression\":\"2+2\"}"}}]},"finish_reason":null}]}
data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}
data: [DONE]
`
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	req := &ModelRequest{
		Instruct: "Calculate 2+2",
		Tools: []ToolDefinition{
			{Name: "calc", Description: "calculator", Parameters: map[string]any{}},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if len(data.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(data.Tools))
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var toolCalls []ToolCall
	for chunk := range stream {
		toolCalls = append(toolCalls, chunk.Tools...)
	}

	if len(toolCalls) == 0 {
		t.Error("expected at least one tool call")
	}
}

// Check 1.5.7: 单元测试使用 mock HTTP 响应
func TestModelRequestNilError(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	_, err := provider.GenerateRequestData(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestRequestDataNilError(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	_, err := provider.RequestModel(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request data")
	}
}

// 测试 OutputSchema 序列化
func TestOutputSchemaSerialization(t *testing.T) {
	schema := OutputSchema{
		Type: "object",
		Properties: map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
		Required: []string{"name", "age"},
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded OutputSchema
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Type != schema.Type || len(decoded.Required) != 2 {
		t.Errorf("mismatch: got %+v, want %+v", decoded, schema)
	}
}
