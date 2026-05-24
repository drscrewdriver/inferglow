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
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAnthropicFullURL verifies AnthropicCompatibleProvider.RequestModel
// honors FullURL override; otherwise falls back to BaseURL + /v1/messages.
func TestAnthropicFullURL(t *testing.T) {
	t.Run("full_url_takes_precedence", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			// Minimal Anthropic-style SSE to terminate the stream cleanly.
			w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
		}))
		defer server.Close()

		provider := &AnthropicCompatibleProvider{
			BaseURL: "https://api.anthropic.com",
			FullURL: server.URL + "/proxy-messages",
			Model:   "claude-3-5-sonnet-20241022",
		}
		data := &RequestData{Model: "claude-3-5-sonnet-20241022", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/proxy-messages" {
			t.Errorf("request hit path %q, want /proxy-messages (FullURL precedence)", hitPath)
		}
	})

	t.Run("empty_full_url_uses_default_path", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
		}))
		defer server.Close()

		provider := &AnthropicCompatibleProvider{
			BaseURL: server.URL,
			Model:   "claude-3-5-sonnet-20241022",
		}
		data := &RequestData{Model: "claude-3-5-sonnet-20241022", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/v1/messages" {
			t.Errorf("request hit path %q, want /v1/messages (legacy behavior)", hitPath)
		}
	})
}
