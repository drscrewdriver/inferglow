package model

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// M-CRITICAL-3 + M-CRITICAL-4: Ollama SSE goroutine must exit promptly
// when ctx is cancelled, even if the server is still streaming slowly.
func TestOllamaRequestModelRespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 20; i++ {
			fmt.Fprintf(w, "{\"model\":\"llama3\",\"done\":false,\"message\":{\"role\":\"assistant\",\"content\":\"%d\"}}\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintln(w, `{"model":"llama3","done":true,"usage":{"prompt_eval_count":1,"eval_count":1}}`)
	}))
	defer server.Close()

	provider := &OllamaProvider{BaseURL: server.URL, Model: "llama3"}
	data := &RequestData{Model: "llama3", Messages: []ChatMessage{{Role: "user", Content: "test"}}}

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := provider.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}

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
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("ollama stream goroutine did not exit after ctx cancellation (goroutine leak)")
	}
}

// M-HIGH-12: fallback http.Client should have a non-zero Timeout.
func TestOllamaFallbackHTTPClientHasTimeout(t *testing.T) {
	provider := &OllamaProvider{}
	client := provider.effectiveHTTPClient()
	if client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.Timeout <= 0 {
		t.Errorf("expected non-zero Timeout on fallback http.Client, got %v", client.Timeout)
	}
}

// BUG-NEW-1: BroadcastResponse must not panic when chunk.Usage is nil on done.
// Mirrors OpenAI/Anthropic provider behavior — see medium_bugfix_test.go
// TestBroadcastResponsePassesUsageToDone.
func TestOllamaBroadcastResponse_DoneWithNilUsage_NoPanic(t *testing.T) {
	provider := NewOllamaProvider()
	stream := make(chan *StreamChunk, 1)
	stream <- &StreamChunk{IsDone: true, Usage: nil}
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
	// Zero-value UsageInfo check — no panic, no garbage.
	if donePayload.Usage != (UsageInfo{}) {
		t.Errorf("ModelResponse.Usage = %+v, want zero-value UsageInfo{}", donePayload.Usage)
	}
}

// BUG-NEW-1: BroadcastResponse passes through non-nil Usage on done.
func TestOllamaBroadcastResponse_DoneWithUsage_PassesThrough(t *testing.T) {
	provider := NewOllamaProvider()
	sent := &UsageInfo{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	stream := make(chan *StreamChunk, 1)
	stream <- &StreamChunk{IsDone: true, Usage: sent}
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
	if donePayload.Usage != *sent {
		t.Errorf("ModelResponse.Usage = %+v, want %+v", donePayload.Usage, *sent)
	}
}

// BUG-NEW-1: BroadcastResponse tracks lastUsage from earlier chunks (e.g. Meta
// events) and uses it when the done chunk has nil Usage — same pattern as
// OpenAI TestBroadcastResponsePassesUsageToDone.
func TestOllamaBroadcastResponse_LastUsageTrackedFromMeta(t *testing.T) {
	provider := NewOllamaProvider()
	metaUsage := &UsageInfo{PromptTokens: 7, CompletionTokens: 14, TotalTokens: 21}
	stream := make(chan *StreamChunk, 2)
	// First: a Meta event chunk with non-nil Usage
	stream <- &StreamChunk{IsDone: false, Usage: metaUsage}
	// Then: done chunk with nil Usage — lastUsage should be the Meta event's
	stream <- &StreamChunk{IsDone: true, Usage: nil}
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
	if donePayload.Usage != *metaUsage {
		t.Errorf("ModelResponse.Usage = %+v, want %+v (last tracked usage from Meta event)", donePayload.Usage, *metaUsage)
	}
}
