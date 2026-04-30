package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Check 1.1: OllamaProvider 结构体定义完整
func TestOllamaProviderName(t *testing.T) {
	p := NewOllamaProvider()
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

// Check 1.1: 默认 BaseURL 正确
func TestOllamaProviderDefaultBaseURL(t *testing.T) {
	p := NewOllamaProvider()
	if p.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "http://localhost:11434")
	}
}

// Check 1.1: GenerateRequestData 正确构建请求
func TestOllamaProviderGenerateRequestData(t *testing.T) {
	p := NewOllamaProvider()
	req := &ModelRequest{
		System:      "You are helpful",
		Instruct:    "Hello",
		Temperature: 0.5,
	}

	data, err := p.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}

	if data.Model != "llama3" {
		t.Errorf("Model = %q, want %q", data.Model, "llama3")
	}
	if data.Temperature != 0.5 {
		t.Errorf("Temperature = %v, want %v", data.Temperature, 0.5)
	}
	if len(data.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(data.Messages))
	}
	if data.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", data.Messages[0].Role, "system")
	}
	if data.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want %q", data.Messages[1].Role, "user")
	}
}

// Check 1.1: GenerateRequestData 无默认 model 时使用 llama3
func TestOllamaProviderGenerateRequestDataDefaultModel(t *testing.T) {
	p := NewOllamaProvider()
	p.Model = "mistral"

	req := &ModelRequest{Instruct: "test"}
	data, err := p.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}
	if data.Model != "mistral" {
		t.Errorf("Model = %q, want %q", data.Model, "mistral")
	}
}

// Check 1.1: GenerateRequestData nil 请求返回 error
func TestOllamaProviderGenerateRequestDataNil(t *testing.T) {
	p := NewOllamaProvider()
	_, err := p.GenerateRequestData(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

// Check 1.2: Ollama SSE 格式解析 - 流式响应
func TestOllamaProviderRequestModelStreaming(t *testing.T) {
	// Mock Ollama SSE 响应
	sseResponses := []string{
		`{"model":"llama3","done":false,"message":{"role":"assistant","content":"Hello"}}`,
		`{"model":"llama3","done":false,"message":{"role":"assistant","content":" world"}}`,
		`{"model":"llama3","done":true,"usage":{"prompt_eval_count":10,"eval_count":20}}`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("URL = %q, want /api/chat", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		for _, resp := range sseResponses {
			_, _ = w.Write([]byte(resp + "\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	provider := &OllamaProvider{
		BaseURL: server.URL,
		Model:   "llama3",
	}

	data := &RequestData{
		Model:       "llama3",
		Messages:    []ChatMessage{{Role: "user", Content: "Hi"}},
		Temperature: 0.7,
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var chunks []*StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// 第一条：Hello
	if chunks[0].Delta != "Hello" {
		t.Errorf("chunk[0].Delta = %q, want %q", chunks[0].Delta, "Hello")
	}
	if chunks[0].IsDone {
		t.Error("chunk[0] should not be done")
	}

	// 第二条：world
	if chunks[1].Delta != " world" {
		t.Errorf("chunk[1].Delta = %q, want %q", chunks[1].Delta, " world")
	}

	// 第三条：done
	if !chunks[2].IsDone {
		t.Error("chunk[2] should be done")
	}
	if chunks[2].Usage == nil {
		t.Error("chunk[2] should have usage")
	} else {
		if chunks[2].Usage.PromptTokens != 10 {
			t.Errorf("PromptTokens = %d, want 10", chunks[2].Usage.PromptTokens)
		}
		if chunks[2].Usage.CompletionTokens != 20 {
			t.Errorf("CompletionTokens = %d, want 20", chunks[2].Usage.CompletionTokens)
		}
	}
}

// Check 1.2: Ollama SSE 格式解析 - 非流式完成标记
func TestOllamaProviderRequestModelDoneOnly(t *testing.T) {
	sseResponses := []string{
		`{"model":"llama3","done":true,"usage":{"prompt_eval_count":5,"eval_count":15}}`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, resp := range sseResponses {
			_, _ = w.Write([]byte(resp + "\n"))
		}
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	data := &RequestData{Model: "llama3", Messages: []ChatMessage{{Role: "user", Content: "Hi"}}}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

	var chunks []*StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !chunks[0].IsDone {
		t.Error("expected IsDone=true")
	}
}

// Check 1.3: BroadcastResponse 正确广播事件
func TestOllamaProviderBroadcastResponse(t *testing.T) {
	provider := NewOllamaProvider()

	stream := make(chan *StreamChunk, 10)
	stream <- &StreamChunk{Delta: "Hello", IsDone: false}
	stream <- &StreamChunk{Delta: " world", IsDone: false}
	stream <- &StreamChunk{
		IsDone:  true,
		Usage:   &UsageInfo{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
	}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var deltas []string
	var done bool
	for event := range events {
		switch event.EventType {
		case EventDelta:
			deltas = append(deltas, event.Payload.(string))
		case EventDone:
			done = true
			resp, ok := event.Payload.(*ModelResponse)
			if !ok {
				t.Fatal("expected *ModelResponse on done")
			}
			if resp.Content != "Hello world" {
				t.Errorf("Content = %q, want %q", resp.Content, "Hello world")
			}
		case MetaEvent:
			usage, ok := event.Payload.(*UsageInfo)
			if !ok {
				t.Fatal("expected *UsageInfo on meta")
			}
			if usage.PromptTokens != 5 {
				t.Errorf("Meta PromptTokens = %d, want 5", usage.PromptTokens)
			}
		}
	}

	if len(deltas) != 2 {
		t.Errorf("expected 2 delta events, got %d", len(deltas))
	}
	if deltas[0] != "Hello" || deltas[1] != " world" {
		t.Errorf("deltas = %v, want [Hello world]", deltas)
	}
	if !done {
		t.Error("expected done event")
	}
}

// Check 1.3: BroadcastResponse 错误事件
func TestOllamaProviderBroadcastResponseError(t *testing.T) {
	provider := NewOllamaProvider()

	stream := make(chan *StreamChunk, 5)
	stream <- &StreamChunk{
		IsDone: true,
		Meta:   map[string]any{"error": "connection refused"},
	}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var errorEvent *ResultEvent
	for event := range events {
		if event.EventType == ErrorEvent {
			errorEvent = event
		}
	}

	if errorEvent == nil {
		t.Fatal("expected error event")
	}
	if errorEvent.Payload.(string) != "connection refused" {
		t.Errorf("error payload = %q, want %q", errorEvent.Payload, "connection refused")
	}
}

// Check 1.3: 错误 HTTP 状态码
func TestOllamaProviderRequestModelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	data := &RequestData{Model: "llama3", Messages: []ChatMessage{{Role: "user", Content: "Hi"}}}

	_, err := provider.RequestModel(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain 500, got %q", err.Error())
	}
}

// Check 1.1: 注册 OllamaProvider 到注册表
func TestOllamaProviderRegister(t *testing.T) {
	reg := NewModelRequesterRegistry()
	provider := NewOllamaProvider()

	if err := reg.Register(provider); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	list := reg.List()
	if len(list) != 1 || list[0] != "ollama" {
		t.Errorf("unexpected list: %v", list)
	}

	found, err := reg.Get("ollama")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found.Name() != "ollama" {
		t.Errorf("wrong provider name: %q", found.Name())
	}
}

// Check: 并发安全
func TestOllamaProviderConcurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`{"model":"llama3","done":true,"usage":{"prompt_eval_count":1,"eval_count":1}}` + "\n"))
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	data := &RequestData{Model: "llama3", Messages: []ChatMessage{{Role: "user", Content: "test"}}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			stream, err := provider.RequestModel(ctx, data)
			if err != nil {
				t.Error(err)
				return
			}
			for chunk := range stream {
				_ = chunk
			}
		}()
		go func() {
			defer func() { done <- true }()
			_ = provider.Name()
		}()
	}

	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("concurrent test timed out")
		}
	}
}
