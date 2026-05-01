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
