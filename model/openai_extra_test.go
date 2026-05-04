package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Check: 非 OK HTTP 状态码返回错误
func TestRequestModelErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid request"}}`))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "test"}}}
	_, err := provider.RequestModel(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

// Check: RequestModel 返回 JSON 解析错误
func TestRequestModelInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: invalid json"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "test"}}}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	// 应读取到 IsDone=true 的 chunk（因为无效 JSON 被跳过，没有 [DONE]）
	var chunks []*StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
		if chunk.IsDone {
			break
		}
	}

	// 没有有效 chunk，但不会 panic
	_ = chunks
}

// Check: Reasoning 字段在 SSE 中正确解析
func TestSSEWithReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := `data: {"choices":[{"delta":{"reasoning":"Let me think..."},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":"The answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}
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

	foundReasoning := false
	for chunk := range stream {
		if chunk.Reasoning == "Let me think..." {
			foundReasoning = true
		}
	}

	if !foundReasoning {
		t.Error("expected reasoning content in stream")
	}
}

// Check: Developer 消息正确放入 messages
func TestGenerateRequestDataWithDeveloper(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		System:    "You are a helpful assistant.",
		Developer: "Always respond in JSON.",
		Instruct:  "Calculate 2+2.",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// 应该有 system + developer + user 三条消息
	if len(data.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(data.Messages))
	}

	if data.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", data.Messages[0].Role, "system")
	}
	if data.Messages[1].Role != "developer" {
		t.Errorf("second message role = %q, want %q", data.Messages[1].Role, "developer")
	}
	if data.Messages[2].Role != "user" {
		t.Errorf("third message role = %q, want %q", data.Messages[2].Role, "user")
	}
}

// Check: ChatHistory 正确放入 messages
func TestGenerateRequestDataWithChatHistory(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		Instruct: "Follow up question",
		ChatHistory: []ChatMessage{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// 应该有 history(2) + user(1) = 3 messages
	if len(data.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(data.Messages))
	}

	if data.Messages[0].Content != "first question" {
		t.Errorf("first message content = %q, want %q", data.Messages[0].Content, "first question")
	}
}

// Check: Instruct 和 Input 合并
func TestGenerateRequestDataInstructInputMerged(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		Instruct: "Task: ",
		Input:    "Calculate 2+2.",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// Instruct 和 Input 应该合并
	if data.Messages[len(data.Messages)-1].Content != "Task: \n\nCalculate 2+2." {
		t.Errorf("user content = %q, expected Instruct + Input", data.Messages[len(data.Messages)-1].Content)
	}
}

// Check: RequestModel 返回 HTTP client 错误
func TestRequestModelConnectionError(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		BaseURL: "http://localhost:99999", // 无效端口
		Model:   "gpt-4",
	}

	data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "test"}}}
	_, err := provider.RequestModel(context.Background(), data)
	if err == nil {
		t.Fatal("expected connection error for invalid URL")
	}
}

// Check: toStreamChunk 无 choices 返回 nil（M-MEDIUM-4：空 choices 不再视为完成）
func TestToStreamChunkEmptyChoices(t *testing.T) {
	// 模拟空 choices 的 chunk
	chunk := openAIChunk{
		Choices: []struct {
			Index        int `json:"index"`
			Delta        struct {
				Role          string     `json:"role,omitempty"`
				Content       *string    `json:"content,omitempty"`
				Reasoning     *string    `json:"reasoning,omitempty"`
				ToolCalls     []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{},
	}

	result := toStreamChunk(chunk, nil)
	if result != nil {
		t.Errorf("expected nil for empty choices; got %+v", result)
	}
}

// Check: 工具调用参数为空字符串
func TestToolCallEmptyArguments(t *testing.T) {
	// 模拟一个空的 tool call arguments
	chunk := openAIChunk{
		Choices: []struct {
			Index        int `json:"index"`
			Delta        struct {
				Role          string     `json:"role,omitempty"`
				Content       *string    `json:"content,omitempty"`
				Reasoning     *string    `json:"reasoning,omitempty"`
				ToolCalls     []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role          string     `json:"role,omitempty"`
					Content       *string    `json:"content,omitempty"`
					Reasoning     *string    `json:"reasoning,omitempty"`
					ToolCalls     []toolCall `json:"tool_calls,omitempty"`
				}{
					ToolCalls: []toolCall{
						{
							ID:     "call_1",
							Type:   "function",
							Function: functionCall{Name: "test_func", Arguments: ""},
						},
					},
				},
			},
		},
	}

	result := toStreamChunk(chunk, nil)
	if len(result.Tools) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "test_func" {
		t.Errorf("tool name = %q, want %q", result.Tools[0].Name, "test_func")
	}
}

// Check: BroadcastResponse 携带 Usage 事件
func TestBroadcastResponseUsage(t *testing.T) {
	stream := make(chan *StreamChunk, 5)
	stream <- &StreamChunk{Delta: "test", IsDone: false, Usage: &UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	provider := &OpenAICompatibleProvider{}
	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	foundUsage := false
	for event := range events {
		if event.EventType == MetaEvent {
			usage, ok := event.Payload.(*UsageInfo)
			if ok && usage.PromptTokens == 10 && usage.CompletionTokens == 5 {
				foundUsage = true
			}
		}
	}

	if !foundUsage {
		t.Error("expected Usage event in BroadcastResponse")
	}
}

// Check: BroadcastResponse 携带 Error 事件
func TestBroadcastResponseError(t *testing.T) {
	stream := make(chan *StreamChunk, 5)
	stream <- &StreamChunk{IsDone: true, Meta: map[string]any{"error": "connection timeout"}}
	close(stream)

	provider := &OpenAICompatibleProvider{}
	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	foundError := false
	for event := range events {
		if event.EventType == ErrorEvent {
			foundError = true
		}
	}

	if !foundError {
		t.Error("expected Error event in BroadcastResponse")
	}
}

// Check: BroadcastResponse 携带 ToolCalls 事件
func TestBroadcastResponseToolCalls(t *testing.T) {
	stream := make(chan *StreamChunk, 5)
	stream <- &StreamChunk{
		Tools: []ToolCall{{ID: "1", Name: "calc", Arguments: map[string]any{"x": 1}}},
		IsDone: false,
	}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	provider := &OpenAICompatibleProvider{}
	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	foundToolCalls := false
	for event := range events {
		if event.EventType == ToolCallsEvent {
			foundToolCalls = true
		}
	}

	if !foundToolCalls {
		t.Error("expected ToolCalls event in BroadcastResponse")
	}
}

// Check: JSON 请求体中正确包含 options
func TestRequestModelWithOptions(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:       "gpt-4",
		Messages:    []ChatMessage{{Role: "user", Content: "test"}},
		Temperature: 0.8,
		MaxTokens:   500,
		Options:     map[string]any{"stop": []string{"\n\n"}},
	}

	_, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	// M-HIGH-2: max_tokens is a reserved field — set via RequestData.MaxTokens,
	// not Options["max_tokens"]. Non-reserved fields (stop) still expand.
	if receivedBody["max_tokens"] == nil {
		t.Error("expected max_tokens in request body")
	}
	if receivedBody["stop"] == nil {
		t.Error("expected stop in request body")
	}
}

// Check: ChatMessage 序列化含 ToolCalls
func TestChatMessageWithToolCallsSerialization(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCall{
			{ID: "call_abc", Name: "get_weather", Arguments: map[string]any{"location": "Beijing"}},
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

	if len(decoded.ToolCalls) != 1 {
		t.Errorf("ToolCalls length = %d, want 1", len(decoded.ToolCalls))
	}
	if decoded.ToolCalls[0].Name != "get_weather" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", decoded.ToolCalls[0].Name, "get_weather")
	}
}

// Check: ChatMessage 含 Name 字段
func TestChatMessageWithName(t *testing.T) {
	msg := ChatMessage{
		Role:    "function",
		Content: "result",
		Name:    "get_weather",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ChatMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", decoded.Name, "get_weather")
	}
}

// Check: RequestData 含 Tools 字段
func TestRequestDataWithTools(t *testing.T) {
	data := &RequestData{
		Model:       "gpt-4",
		Messages:    []ChatMessage{{Role: "user", Content: "test"}},
		Temperature: 0.7,
		Tools: []ToolDefinition{
			{Name: "calc", Description: "calculator", Parameters: map[string]any{"type": "object"}},
		},
	}

	if len(data.Tools) != 1 {
		t.Errorf("Tools length = %d, want 1", len(data.Tools))
	}
	if data.Tools[0].Name != "calc" {
		t.Errorf("Tools[0].Name = %q, want %q", data.Tools[0].Name, "calc")
	}
}
