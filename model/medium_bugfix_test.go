package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// M-MEDIUM-2: anthropic-version header should be "2024-10-22" (or newer),
// not the outdated "2023-06-01".
func TestAnthropicVersionHeaderUpdated(t *testing.T) {
	var receivedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, APIKey: "k", Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	if receivedVersion == "" {
		t.Fatal("anthropic-version header not set")
	}
	if receivedVersion == "2023-06-01" {
		t.Errorf("anthropic-version = %q (outdated); expected 2024-10-22 or newer", receivedVersion)
	}
	if receivedVersion < "2024-10-22" {
		t.Errorf("anthropic-version = %q; expected >= 2024-10-22", receivedVersion)
	}
}

// M-MEDIUM-4: toStreamChunk must return nil when Choices is empty, instead
// of marking the chunk as IsDone. An empty-choices chunk carries usage or
// keepalive data, not a finish signal.
func TestToStreamChunk_EmptyChoicesNotDone(t *testing.T) {
	chunk := openAIChunk{
		ID:      "chatcmpl-1",
		Choices: nil,
		Usage:   &UsageInfo{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}

	result := toStreamChunk(chunk, nil)

	if result != nil && result.IsDone {
		t.Errorf("empty-choices chunk must not be IsDone; got %+v", result)
	}
}

// M-MEDIUM-5: FinishReason="length" indicates the response was truncated by
// max_tokens and is therefore incomplete. It must NOT mark the chunk as
// IsDone — only genuine completion signals ("stop", "tool_calls") should.
func TestToStreamChunk_FinishReasonLengthNotDone(t *testing.T) {
	chunk := openAIChunk{
		ID: "chatcmpl-1",
		Choices: []struct {
			Index int `json:"index"`
			Delta struct {
				Role      string     `json:"role,omitempty"`
				Content   *string    `json:"content,omitempty"`
				Reasoning *string    `json:"reasoning,omitempty"`
				ToolCalls []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{FinishReason: "length"},
		},
	}

	result := toStreamChunk(chunk, nil)
	if result == nil {
		t.Fatal("expected non-nil result for finish_reason=length")
	}
	if result.IsDone {
		t.Errorf("finish_reason=\"length\" must not set IsDone; got %+v", result)
	}
}

// M-MEDIUM-9: Ollama provider must handle ModelRequest.Output by setting the
// Ollama-native "format":"json" field in the request body (structured output).
func TestOllamaHandlesOutputSchema(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"llama3","done":true,"message":{"role":"assistant","content":"{\"x\":1}"}}` + "\n"))
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	req := &ModelRequest{
		Input: "test",
		Output: &OutputSchema{
			Type:       "object",
			Properties: map[string]any{"x": map[string]any{"type": "integer"}},
			Required:   []string{"x"},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	// M-MEDIUM-9: Ollama format=json triggers JSON mode
	if format, ok := receivedBody["format"].(string); !ok || format != "json" {
		t.Errorf("format = %v, want \"json\" when Output schema is set", receivedBody["format"])
	}
}

// M-MEDIUM-9: Ollama must NOT set format=json when no Output schema is set.
func TestOllamaNoOutputNoFormat(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"llama3","done":true,"message":{"role":"assistant","content":"hi"}}` + "\n"))
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	req := &ModelRequest{Input: "test"}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	if _, exists := receivedBody["format"]; exists {
		t.Errorf("format must not be set when no Output schema; got %v", receivedBody["format"])
	}
}

// M-MEDIUM-9: Anthropic provider must handle ModelRequest.Output by injecting
// a JSON instruction into the system message (Anthropic has no native
// response_format).
func TestAnthropicHandlesOutputSchema(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, APIKey: "k", Model: "claude"}
	req := &ModelRequest{
		System: "You are helpful.",
		Input:  "test",
		Output: &OutputSchema{
			Type:       "object",
			Properties: map[string]any{"x": map[string]any{"type": "integer"}},
			Required:   []string{"x"},
		},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	// System message must contain a JSON instruction
	sysRaw, ok := receivedBody["system"]
	if !ok {
		t.Fatal("expected system field in request body")
	}
	sysStr, ok := sysRaw.(string)
	if !ok {
		t.Fatalf("system should be a string, got %T", sysRaw)
	}
	if !strings.Contains(strings.ToLower(sysStr), "json") {
		t.Errorf("system message must contain JSON instruction; got %q", sysStr)
	}
}

// M-MEDIUM-10: BroadcastResponse must pass Usage into the final ModelResponse
// (EventDone payload). Without this, consumers reading the Done event lose
// token usage information.
func TestBroadcastResponsePassesUsageToDone(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Delta: "hello"}
	stream <- &StreamChunk{Usage: &UsageInfo{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	var sawUsageMetaEvent bool
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
		if ev.EventType == MetaEvent {
			if _, ok := ev.Payload.(*UsageInfo); ok {
				sawUsageMetaEvent = true
			}
		}
	}

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if donePayload.Usage.TotalTokens != 8 {
		t.Errorf("ModelResponse.Usage.TotalTokens = %d, want 8 (Usage must propagate to Done payload)", donePayload.Usage.TotalTokens)
	}
	if donePayload.Usage.PromptTokens != 5 {
		t.Errorf("ModelResponse.Usage.PromptTokens = %d, want 5", donePayload.Usage.PromptTokens)
	}
	if donePayload.Usage.CompletionTokens != 3 {
		t.Errorf("ModelResponse.Usage.CompletionTokens = %d, want 3", donePayload.Usage.CompletionTokens)
	}
	if !sawUsageMetaEvent {
		t.Error("expected a MetaEvent with UsageInfo payload")
	}
}

// M-MEDIUM-10: Anthropic BroadcastResponse also passes Usage to ModelResponse.
func TestAnthropicBroadcastResponsePassesUsageToDone(t *testing.T) {
	provider := &AnthropicCompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Delta: "hi"}
	stream <- &StreamChunk{Usage: &UsageInfo{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}}
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
	if donePayload.Usage.TotalTokens != 12 {
		t.Errorf("ModelResponse.Usage.TotalTokens = %d, want 12", donePayload.Usage.TotalTokens)
	}
}

// M-MEDIUM-11: Anthropic message_start event carries the initial usage
// (input_tokens). The provider must capture and emit it so consumers see
// prompt token counts even when message_delta is missing or partial.
func TestAnthropicMessageStartUsageCaptured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 1. message_start with input_tokens=42
		w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":42,\"output_tokens\":1}}}\n\n"))
		// 2. content_block_start
		w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		// 3. content_block_delta with text
		w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
		// 4. content_block_stop
		w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		// 5. message_delta with output_tokens (but no input_tokens!)
		w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n"))
		// 6. message_stop
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, APIKey: "k", Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"max_tokens": 100},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var sawUsageWithInputTokens bool
	for chunk := range stream {
		if chunk.Usage != nil && chunk.Usage.PromptTokens == 42 {
			sawUsageWithInputTokens = true
		}
	}

	if !sawUsageWithInputTokens {
		t.Errorf("expected usage chunk with PromptTokens=42 from message_start; never seen")
	}
}

// M-MEDIUM-8: OpenAI / Anthropic tool_calls[].function.arguments must be
// serialized as a JSON STRING (not a JSON object) on the wire. The Go
// struct keeps Arguments as map[string]any for type safety; a custom
// MarshalJSON converts it to a string. Round-trip must preserve data.
func TestToolCallArgumentsSerializedAsJSONString(t *testing.T) {
	msg := ChatMessage{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"location": "Beijing"}},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Wire format must show arguments as a JSON string, not object.
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal to wire map: %v", err)
	}
	tcs, ok := wire["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Fatalf("tool_calls not a slice of len 1: %v", wire["tool_calls"])
	}
	tc, ok := tcs[0].(map[string]any)
	if !ok {
		t.Fatalf("tool_calls[0] not a map: %T", tcs[0])
	}
	args, ok := tc["arguments"]
	if !ok {
		t.Fatal("arguments field missing")
	}
	argsStr, ok := args.(string)
	if !ok {
		t.Fatalf("arguments should be a JSON string on the wire, got %T: %v", args, args)
	}
	// The string must itself be valid JSON object.
	var inner map[string]any
	if err := json.Unmarshal([]byte(argsStr), &inner); err != nil {
		t.Fatalf("arguments string is not valid JSON: %v (value=%q)", err, argsStr)
	}
	if loc, ok := inner["location"].(string); !ok || loc != "Beijing" {
		t.Errorf("arguments.location = %v, want \"Beijing\"", inner["location"])
	}

	// Round-trip: unmarshal back to ChatMessage preserves Arguments map.
	var decoded ChatMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(decoded.ToolCalls))
	}
	if loc, ok := decoded.ToolCalls[0].Arguments["location"].(string); !ok || loc != "Beijing" {
		t.Errorf("round-trip Arguments.location = %v, want \"Beijing\"", decoded.ToolCalls[0].Arguments["location"])
	}
}

// M-MEDIUM-1: tool_choice must be passed through to OpenAI-compatible API
// in the request body. Values can be "auto", "none", "required", or a
// structured {"type":"function","function":{"name":"..."}} object.
func TestOpenAIToolChoicePassedThrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:      "gpt-4",
		Messages:   []ChatMessage{{Role: "user", Content: "test"}},
		ToolChoice: "required",
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	if tc, ok := receivedBody["tool_choice"].(string); !ok || tc != "required" {
		t.Errorf("tool_choice = %v, want \"required\"", receivedBody["tool_choice"])
	}
}

// M-MEDIUM-1: tool_choice structured form (function-call pinning).
func TestOpenAIToolChoiceStructuredPassedThrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "get_weather",
			},
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	tc, ok := receivedBody["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice should be a map, got %T", receivedBody["tool_choice"])
	}
	if tc["type"] != "function" {
		t.Errorf("tool_choice.type = %v, want \"function\"", tc["type"])
	}
	fn, ok := tc["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice.function should be a map, got %T", tc["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("tool_choice.function.name = %v, want \"get_weather\"", fn["name"])
	}
}

// M-MEDIUM-1: Anthropic tool_choice uses {type:"auto"|"any"|"tool",name?}.
func TestAnthropicToolChoicePassedThrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, APIKey: "k", Model: "claude"}
	data := &RequestData{
		Model:      "claude",
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
		Options:    map[string]any{"max_tokens": 100},
		ToolChoice: map[string]any{"type": "any"},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	tc, ok := receivedBody["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice should be a map, got %T", receivedBody["tool_choice"])
	}
	if tc["type"] != "any" {
		t.Errorf("tool_choice.type = %v, want \"any\"", tc["type"])
	}
}

// M-MEDIUM-1: tool_choice is copied through GenerateRequestData.
func TestOpenAIGenerateRequestData_ToolChoice(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		Input:      "test",
		ToolChoice: "auto",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}
	if data.ToolChoice != "auto" {
		t.Errorf("data.ToolChoice = %v, want \"auto\"", data.ToolChoice)
	}
}

// M-MEDIUM-3: Ollama chat API requires Options to be nested under an
// "options" sub-object (per Ollama /api/chat spec). Top-level expansion is
// silently ignored by the server and silently breaks caller intent.
func TestOllamaOptionsNestedInSubObject(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"llama3","done":true,"message":{"role":"assistant","content":"ok"}}` + "\n"))
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	data := &RequestData{
		Model:    "llama3",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"top_p": 0.9, "seed": 42},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	// "options" must be a sub-object
	optsRaw, ok := receivedBody["options"]
	if !ok {
		t.Fatal("expected \"options\" sub-object in request body")
	}
	opts, ok := optsRaw.(map[string]any)
	if !ok {
		t.Fatalf("options should be a map, got %T", optsRaw)
	}
	if topP, ok := opts["top_p"].(float64); !ok || topP != 0.9 {
		t.Errorf("options.top_p = %v, want 0.9", opts["top_p"])
	}
	if seed, ok := opts["seed"].(float64); !ok || seed != 42 {
		t.Errorf("options.seed = %v, want 42", opts["seed"])
	}

	// Top-level must NOT contain top_p / seed
	if _, exists := receivedBody["top_p"]; exists {
		t.Errorf("top_p must not be at top level (should be inside options)")
	}
	if _, exists := receivedBody["seed"]; exists {
		t.Errorf("seed must not be at top level (should be inside options)")
	}
}

// M-MEDIUM-6: bufio.Reader buffer must handle large SSE lines (>= 1MB) without
// truncation. Lines larger than the default 4KB buffer must be read in full.
// This serves as a regression guard: if anyone switches back to bufio.Scanner
// (default 64KB token limit), this test will fail.
func TestOpenAILargeSSELineHandled(t *testing.T) {
	// Build an SSE line whose payload is ~200KB (well above the 64KB Scanner
	// limit and the 4KB default bufio.Reader buffer).
	largeContent := strings.Repeat("x", 200*1024)
	line := "data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" +
		largeContent + "\"},\"finish_reason\":null}]}\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(line))
		w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var collected strings.Builder
	for chunk := range stream {
		if chunk.Delta != "" {
			collected.WriteString(chunk.Delta)
		}
	}

	if collected.Len() != len(largeContent) {
		t.Errorf("collected %d bytes, want %d (large SSE line truncated)", collected.Len(), len(largeContent))
	}
}

// M-MEDIUM-5: processOpenAILine must not emit IsDone when finish_reason=length.
// The stream continues until [DONE] / EOF so usage is preserved and downstream
// consumers don't mistake a truncated response for a successful completion.
func TestProcessOpenAILine_FinishReasonLengthNotDone(t *testing.T) {
	var emitted []*StreamChunk
	emit := func(c *StreamChunk) { emitted = append(emitted, c) }

	line := `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`

	(&OpenAICompatibleProvider{}).processOpenAILine(line, nil, map[int]*openAIToolState{}, emit)

	for _, c := range emitted {
		if c.IsDone {
			t.Errorf("finish_reason=\"length\" must not emit IsDone chunk; got %+v", c)
		}
	}
}
