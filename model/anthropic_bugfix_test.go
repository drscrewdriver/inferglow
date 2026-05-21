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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// M-CRITICAL-3 + M-CRITICAL-4: Anthropic SSE goroutine must exit promptly
// when ctx is cancelled, even if the server is still streaming slowly.
func TestAnthropicRequestModelRespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 20; i++ {
			fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"%d\"}}\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintln(w, "data: {\"type\":\"message_stop\"}")
	}))
	defer server.Close()

	provider := &AnthropicCompatibleProvider{BaseURL: server.URL, Model: "claude"}
	data := &RequestData{
		Model:    "claude",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"max_tokens": 100},
	}

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
		t.Fatal("anthropic stream goroutine did not exit after ctx cancellation (goroutine leak)")
	}
}

// M-HIGH-12: fallback http.Client should have a non-zero Timeout.
func TestAnthropicFallbackHTTPClientHasTimeout(t *testing.T) {
	provider := &AnthropicCompatibleProvider{Model: "claude"}
	client := provider.effectiveHTTPClient()
	if client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.Timeout <= 0 {
		t.Errorf("expected non-zero Timeout on fallback http.Client, got %v", client.Timeout)
	}
}
