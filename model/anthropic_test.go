package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Check: Name() 返回 "anthropic"
func TestAnthropicProviderName(t *testing.T) {
	p := &AnthropicCompatibleProvider{}
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q, want \"anthropic\"", p.Name())
	}
}

// Check: GenerateRequestData 正确转换 ModelRequest → RequestData
// System 存入 Options["_anthropic_system"]；max_tokens 默认 1024
func TestAnthropicGenerateRequestData(t *testing.T) {
	provider := &AnthropicCompatibleProvider{Model: "claude-3-5-sonnet-20241022"}

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

	if data.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("wrong model: %q", data.Model)
	}

	// System 应该存入 Options["_anthropic_system"]
	sysVal, ok := data.Options["_anthropic_system"]
	if !ok {
		t.Fatal("expected _anthropic_system in Options")
	}
	if sysVal != "You are helpful." {
		t.Errorf("_anthropic_system = %v", sysVal)
	}

	// max_tokens 默认 1024
	if mt, ok := data.Options["max_tokens"]; !ok {
		t.Error("expected max_tokens in Options")
	} else if mt != 1024 {
		t.Errorf("max_tokens = %v, want 1024", mt)
	}

	// 不应该有 system role 的 message
	for _, m := range data.Messages {
		if m.Role == "system" {
			t.Error("system message should be in Options, not messages")
		}
	}

	// 最后一条消息应是 user
	if len(data.Messages) == 0 || data.Messages[len(data.Messages)-1].Role != "user" {
		t.Error("last message should be user")
	}

	if len(data.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(data.Tools))
	}
}

// Check: max_tokens 可被 req.Options["max_tokens"] 覆盖
func TestAnthropicGenerateRequestDataMaxTokensOverride(t *testing.T) {
	provider := &AnthropicCompatibleProvider{Model: "claude"}
	req := &ModelRequest{
		System:  "test",
		Options: map[string]any{"max_tokens": 500},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if mt, ok := data.Options["max_tokens"]; !ok || mt != 500 {
		t.Errorf("max_tokens = %v, want 500", data.Options["max_tokens"])
	}
}

// Check: nil req 返回 error
func TestAnthropicGenerateRequestDataNilReq(t *testing.T) {
	provider := &AnthropicCompatibleProvider{Model: "claude"}
	_, err := provider.GenerateRequestData(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

// Check: 请求体结构 - system 在顶级，max_tokens 必填，stream=true，headers 正确
func TestAnthropicRequestBodyStructure(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 Header
		if r.Header.Get("x-api-key") != "anthropic-key" {
			t.Errorf("x-api-key = %q, want \"anthropic-key\"", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want \"2023-06-01\"", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}

		if r.URL.Path != "/v1/messages" {
			t.Errorf("URL path = %q, want \"/v1/messages\"", r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&receivedBody)

		// 返回最小 SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{
		BaseURL: server.URL,
		APIKey:  "anthropic-key",
		Model:   "claude-3-5-sonnet-20241022",
	}

	data := &RequestData{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options: map[string]any{
			"_anthropic_system": "You are a helpful assistant.",
			"max_tokens":        1024,
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	// drain stream
	for range stream {
	}

	// 验证 body
	if receivedBody["stream"] != true {
		t.Error("expected stream=true")
	}
	if receivedBody["model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("model = %v", receivedBody["model"])
	}
	// system 应在顶级
	if sys, ok := receivedBody["system"]; !ok {
		t.Error("expected system at top level")
	} else if sys != "You are a helpful assistant." {
		t.Errorf("system = %v", sys)
	}
	// max_tokens 必填
	mt, ok := receivedBody["max_tokens"]
	if !ok {
		t.Fatal("expected max_tokens in body")
	}
	// JSON 解码后数字为 float64
	if mtFloat, ok := mt.(float64); !ok || mtFloat != 1024 {
		t.Errorf("max_tokens = %v, want 1024", mt)
	}
}

// Check: 流式 SSE - text_delta → StreamChunk.Delta
func TestAnthropicSSETextDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var content strings.Builder
	var doneCount int
	for chunk := range stream {
		if chunk.IsDone {
			doneCount++
		}
		content.WriteString(chunk.Delta)
	}

	if content.String() != "Hello world" {
		t.Errorf("content = %q, want \"Hello world\"", content.String())
	}
	if doneCount != 1 {
		t.Errorf("doneCount = %d, want 1", doneCount)
	}
}

// Check: 流式 SSE - thinking_delta → StreamChunk.Reasoning
func TestAnthropicSSEThinkingDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var reasoning strings.Builder
	for chunk := range stream {
		reasoning.WriteString(chunk.Reasoning)
	}

	if reasoning.String() != "Let me think..." {
		t.Errorf("reasoning = %q, want \"Let me think...\"", reasoning.String())
	}
}

// Check: 流式 SSE - tool_use 解析为 ToolCall
func TestAnthropicSSEToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"location\":\"Beijing\"}"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "weather?"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var toolCalls []ToolCall
	for chunk := range stream {
		toolCalls = append(toolCalls, chunk.Tools...)
	}

	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "toolu_01" {
		t.Errorf("ID = %q", toolCalls[0].ID)
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("Name = %q", toolCalls[0].Name)
	}
	if loc, ok := toolCalls[0].Arguments["location"]; !ok || loc != "Beijing" {
		t.Errorf("Arguments.location = %v", toolCalls[0].Arguments["location"])
	}
}

// Check: tool_use 多次 input_json_delta 分片累积
func TestAnthropicSSEToolUsePartialJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_02","name":"calc","input":{}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"expression\":"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"2+2\"}"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "calc 2+2"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var toolCalls []ToolCall
	for chunk := range stream {
		toolCalls = append(toolCalls, chunk.Tools...)
	}

	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if exp, ok := toolCalls[0].Arguments["expression"]; !ok || exp != "2+2" {
		t.Errorf("Arguments.expression = %v", toolCalls[0].Arguments["expression"])
	}
}

// Check: message_delta 携带 stop_reason / usage
func TestAnthropicMessageDeltaUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var usage *UsageInfo
	var stopReason any
	for chunk := range stream {
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Meta != nil {
			if sr, ok := chunk.Meta["stop_reason"]; ok {
				stopReason = sr
			}
		}
	}

	if usage == nil {
		t.Fatal("expected usage in stream")
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
	if usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", usage.TotalTokens)
	}
	if stopReason != "end_turn" {
		t.Errorf("stop_reason = %v", stopReason)
	}
}

// Check: BroadcastResponse 正确解析事件流
func TestAnthropicBroadcastResponse(t *testing.T) {
	stream := make(chan *StreamChunk, 10)
	stream <- &StreamChunk{Delta: "Hello", IsDone: false}
	stream <- &StreamChunk{Delta: " world", IsDone: true}
	close(stream)

	provider := &AnthropicCompatibleProvider{}
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

// Check: BroadcastResponse 携带 ToolCalls 事件
func TestAnthropicBroadcastResponseToolCalls(t *testing.T) {
	stream := make(chan *StreamChunk, 5)
	stream <- &StreamChunk{
		Tools: []ToolCall{{ID: "1", Name: "calc", Arguments: map[string]any{"x": 1}}},
	}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	provider := &AnthropicCompatibleProvider{}
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

// Check: 端到端 - GenerateRequestData → RequestModel → BroadcastResponse
func TestAnthropicEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true {
			t.Error("expected stream=true")
		}
		if _, ok := body["system"]; !ok {
			t.Error("expected system at top level")
		}
		if _, ok := body["max_tokens"]; !ok {
			t.Error("expected max_tokens in body")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi there"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "claude-3-5-sonnet-20241022",
	}

	req := &ModelRequest{
		System:   "You are helpful.",
		Instruct: "Say hello",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var deltas []string
	var donePayload *ModelResponse
	for event := range events {
		switch event.EventType {
		case EventDelta:
			if s, ok := event.Payload.(string); ok {
				deltas = append(deltas, s)
			}
		case EventDone:
			if m, ok := event.Payload.(*ModelResponse); ok {
				donePayload = m
			}
		}
	}

	if len(deltas) == 0 {
		t.Error("expected delta events")
	}
	if donePayload == nil {
		t.Fatal("expected done event with ModelResponse")
	}
	if donePayload.Content != "Hi there" {
		t.Errorf("Content = %q, want \"Hi there\"", donePayload.Content)
	}
}

// Check: 非 OK HTTP 状态码返回错误
func TestAnthropicRequestModelErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid request"}}`))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"max_tokens": 100},
	}
	_, err := provider.RequestModel(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

// Check: nil RequestData 返回错误
func TestAnthropicRequestDataNilError(t *testing.T) {
	provider := &AnthropicCompatibleProvider{Model: "claude"}
	_, err := provider.RequestModel(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request data")
	}
}

// Check: 非流式响应（mock server 返回完整 message 后用 stream:true 解析）
// 此场景模拟 Anthropic 真实流式响应被一次性返回的情况
func TestAnthropicNonStreamingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 stream=true 在请求体中
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true {
			t.Error("expected stream=true in request body")
		}

		// 一次性返回完整 message 的 SSE 流
		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Complete response in one shot"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":8}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		w.Write([]byte(sse))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "claude-3",
	}

	req := &ModelRequest{
		System:   "Be helpful.",
		Instruct: "Say something",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var content strings.Builder
	var doneCount int
	for chunk := range stream {
		if chunk.IsDone {
			doneCount++
		}
		content.WriteString(chunk.Delta)
	}

	if content.String() != "Complete response in one shot" {
		t.Errorf("content = %q", content.String())
	}
	if doneCount != 1 {
		t.Errorf("doneCount = %d, want 1", doneCount)
	}
}
