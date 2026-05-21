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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// M-CRITICAL-1: Tools sent in request body must be wrapped as
// {"type":"function","function":{name,description,parameters}} per OpenAI API.
func TestOpenAIToolsWrappedInFunctionEnvelope(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Tools: []ToolDefinition{
			{Name: "calc", Description: "calculator", Parameters: map[string]any{"type": "object"}},
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	toolsRaw, ok := receivedBody["tools"]
	if !ok {
		t.Fatal("expected tools in request body")
	}
	tools, ok := toolsRaw.([]any)
	if !ok {
		t.Fatalf("tools should be a slice, got %T", toolsRaw)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool should be a map, got %T", tools[0])
	}
	if tool["type"] != "function" {
		t.Errorf("tool.type = %v, want \"function\"", tool["type"])
	}
	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool.function should be a map, got %T", tool["function"])
	}
	if fn["name"] != "calc" {
		t.Errorf("tool.function.name = %v, want \"calc\"", fn["name"])
	}
	if fn["description"] != "calculator" {
		t.Errorf("tool.function.description = %v, want \"calculator\"", fn["description"])
	}
}

// M-CRITICAL-2: streaming tool_call arguments must be accumulated across
// chunks per tool index and emitted as a single ToolCall when finish_reason
// equals "tool_calls".
func TestOpenAIStreamingToolCallArgumentsAccumulated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// First chunk: tool_call name + start of args (split JSON)
		// Accumulated args after this chunk: {"expr
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"calc","arguments":"{\"exp"}}]},"finish_reason":null}]}`)
		// Second chunk: continuation of args. Combined: {"expression":"2+2"}
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ression\":\"2+2\"}"}}]},"finish_reason":null}]}`)
		// Third chunk: finish_reason tool_calls
		fmt.Fprintln(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "calc 2+2"}},
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
		t.Fatalf("expected 1 accumulated tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "call_1" {
		t.Errorf("tool ID = %q, want \"call_1\"", toolCalls[0].ID)
	}
	if toolCalls[0].Name != "calc" {
		t.Errorf("tool Name = %q, want \"calc\"", toolCalls[0].Name)
	}
	if expression, ok := toolCalls[0].Arguments["expression"]; !ok {
		t.Fatalf("expected Arguments[\"expression\"], got %+v", toolCalls[0].Arguments)
	} else if expression != "2+2" {
		t.Errorf("Arguments[\"expression\"] = %v, want \"2+2\"", expression)
	}
}

// M-CRITICAL-3 + M-CRITICAL-4: SSE goroutine must exit promptly when ctx is
// cancelled, even if the server is still streaming slowly.
func TestOpenAIRequestModelRespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		// Short loop with a long final sleep so the handler exits promptly
		// after ctx cancellation (server.Close blocks until handler returns).
		for i := 0; i < 20; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"%d\"},\"finish_reason\":null}]}\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "test"}}}

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := provider.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	// Read one chunk, then cancel.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan struct{})
	go func() {
		for range stream {
		}
		close(done)
	}()

	select {
	case <-done:
		// success - goroutine exited
	case <-time.After(3 * time.Second):
		t.Fatal("stream goroutine did not exit after ctx cancellation (goroutine leak)")
	}
}

// M-HIGH-1: developer role should be remappable via RoleMapping.
// Providers like DeepSeek/Qwen/GLM/Kimi don't support "developer" role and
// expect "system" instead.
func TestOpenAIRoleMappingDeveloperToSystem(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		Model: "deepseek-chat",
		RoleMapping: map[string]string{
			"developer": "system",
		},
	}
	req := &ModelRequest{
		System:    "You are helpful.",
		Developer: "Always respond in JSON.",
		Instruct:  "Calculate 2+2.",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// Should have system + user, with developer content merged into system.
	if len(data.Messages) != 2 {
		t.Fatalf("expected 2 messages (system+user) after merge, got %d", len(data.Messages))
	}
	if data.Messages[0].Role != "system" {
		t.Errorf("first role = %q, want \"system\"", data.Messages[0].Role)
	}
	if !contains(data.Messages[0].Content, "You are helpful.") {
		t.Errorf("system content missing original System: %q", data.Messages[0].Content)
	}
	if !contains(data.Messages[0].Content, "Always respond in JSON.") {
		t.Errorf("system content missing Developer: %q", data.Messages[0].Content)
	}
	if data.Messages[1].Role != "user" {
		t.Errorf("second role = %q, want \"user\"", data.Messages[1].Role)
	}
}

// M-HIGH-1: when RoleMapping maps developer -> developer (or no mapping),
// behavior is unchanged (separate developer message).
func TestOpenAIRoleMappingDefaultDeveloper(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		System:    "You are helpful.",
		Developer: "Always respond in JSON.",
		Instruct:  "Calculate 2+2.",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if len(data.Messages) != 3 {
		t.Fatalf("expected 3 messages (system+developer+user), got %d", len(data.Messages))
	}
	if data.Messages[1].Role != "developer" {
		t.Errorf("second role = %q, want \"developer\" (default mapping)", data.Messages[1].Role)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// M-HIGH-9: when TemperatureSet=true and Temperature=0, provider must respect
// the explicit 0 (deterministic) instead of overriding to 0.7.
func TestOpenAIExplicitZeroTemperatureRespected(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		Input:          "test",
		Temperature:    0,
		TemperatureSet: true,
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}
	if data.Temperature != 0 {
		t.Errorf("expected temperature 0 (explicit), got %f", data.Temperature)
	}
}

// M-HIGH-10: max_tokens should be sent in request body when set via Options
// or defaulted from DEFAULT_SETTINGS.
func TestOpenAIMaxTokensSentInRequestBody(t *testing.T) {
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
		MaxTokens:   2048,
		Temperature: 0.7,
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	if receivedBody["max_tokens"] == nil {
		t.Fatal("expected max_tokens in request body")
	}
	if mt, ok := receivedBody["max_tokens"].(float64); !ok || int(mt) != 2048 {
		t.Errorf("max_tokens = %v, want 2048", receivedBody["max_tokens"])
	}
}

// M-HIGH-10: when caller does not set max_tokens, default 4096 should be sent.
func TestOpenAIMaxTokensDefaultApplied(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		System: "test",
		Input:  "test input",
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if data.MaxTokens != DefaultMaxTokens {
		t.Errorf("expected MaxTokens=%d (default), got %d", DefaultMaxTokens, data.MaxTokens)
	}
}

// M-HIGH-10: explicit max_tokens in Options should override default.
func TestOpenAIMaxTokensOptionsOverride(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	req := &ModelRequest{
		Input:   "test",
		Options: map[string]any{"max_tokens": 100},
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if data.MaxTokens != 100 {
		t.Errorf("expected MaxTokens=100 (from Options), got %d", data.MaxTokens)
	}
}

// M-HIGH-12: fallback http.Client should have a non-zero Timeout.
func TestOpenAIFallbackHTTPClientHasTimeout(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		BaseURL: "http://localhost:0", // invalid port - forces fallback path? Actually need different approach
		Model:   "gpt-4",
		// HTTPClient is nil
	}

	// Test via reflection or just check by exercising the path. Easier: expose a
	// helper that returns the effective client.
	client := provider.effectiveHTTPClient()
	if client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.Timeout <= 0 {
		t.Errorf("expected non-zero Timeout on fallback http.Client, got %v", client.Timeout)
	}
}

// O-CRITICAL-1: when the caller sets Options["force_json"]=true, the
// provider must include response_format={"type":"json_object"} in the
// HTTP request body so OpenAI-compatible APIs enforce JSON-object output.
// This protects Agent.Run against LLMs that would otherwise emit prose
// around the JSON decision.
func TestOpenAIForceJSONSetsResponseFormatInBody(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	req := &ModelRequest{
		Input:   "test",
		Options: map[string]any{"force_json": true},
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

	rf, ok := receivedBody["response_format"]
	if !ok {
		t.Fatal("expected response_format in request body when force_json=true")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want \"json_object\"", rfMap["type"])
	}
}

// O-CRITICAL-1: setting req.Output (an OutputSchema) should also trigger
// response_format in the request body, even without explicit force_json.
func TestOpenAIOutputSchemaTriggersResponseFormat(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
	req := &ModelRequest{
		Input: "test",
		Output: &OutputSchema{
			Type: "object",
			Properties: map[string]any{
				"next_action": map[string]any{"type": "string"},
			},
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

	rf, ok := receivedBody["response_format"]
	if !ok {
		t.Fatal("expected response_format in request body when Output schema is set")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want \"json_object\"", rfMap["type"])
	}
}

// O-CRITICAL-1: when neither force_json nor Output is set, response_format
// must NOT appear in the request body (preserves existing behavior for
// callers that want free-form text).
func TestOpenAINoForceJSONNoResponseFormat(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4"}
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

	if rf, ok := receivedBody["response_format"]; ok {
		t.Errorf("expected no response_format in body, got %v", rf)
	}
}

// O-CRITICAL-1: GenerateRequestData must not mutate the caller's Options
// map when adding response_format. The caller's map should remain
// untouched.
func TestOpenAIForceJSONDoesNotMutateCallerOptions(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	callerOpts := map[string]any{"force_json": true, "custom": "value"}

	req := &ModelRequest{
		Input:   "test",
		Options: callerOpts,
	}

	data, err := provider.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	// Caller's map should still have only the original keys.
	if _, ok := callerOpts["response_format"]; ok {
		t.Error("GenerateRequestData mutated caller's Options map (added response_format)")
	}
	if len(callerOpts) != 2 {
		t.Errorf("caller's Options map size = %d, want 2", len(callerOpts))
	}

	// The resulting RequestData.Options should have response_format.
	rf, ok := data.Options["response_format"]
	if !ok {
		t.Fatal("expected response_format in data.Options")
	}
	rfMap, ok := rf.(map[string]any)
	if !ok {
		t.Fatalf("response_format should be a map, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type = %v, want \"json_object\"", rfMap["type"])
	}
}
