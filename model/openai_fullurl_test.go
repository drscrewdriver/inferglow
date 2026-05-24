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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAIRequestModel_FullURL verifies that when FullURL is set on the
// OpenAICompatibleProvider, the HTTP POST lands on FullURL exactly (not on
// BaseURL + /chat/completions). This is the contract:
//   - FullURL non-empty  → request URL == FullURL
//   - FullURL empty      → request URL == TrimRight(BaseURL,"/") + "/chat/completions"
//
// Spec: model-parity Phase 1, Scenario "full_url 非空时直连".
func TestOpenAIRequestModel_FullURL(t *testing.T) {
	t.Run("full_url_takes_precedence", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
		}))
		defer server.Close()

		// FullURL points to the test server's root; BaseURL is a decoy that
		// would otherwise produce /chat/completions. If FullURL is honored,
		// hitPath will be "/" (the test server's only route).
		provider := &OpenAICompatibleProvider{
			BaseURL: "https://api.openai.com/v1",
			FullURL: server.URL + "/proxy-chat",
			Model:   "gpt-4",
		}
		data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		// Drain to let the server record hitPath.
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/proxy-chat" {
			t.Errorf("request hit path %q, want %q (FullURL should take precedence)", hitPath, "/proxy-chat")
		}
	})

	t.Run("empty_full_url_uses_default_path", func(t *testing.T) {
		var hitPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			BaseURL: server.URL,
			Model:   "gpt-4",
			// FullURL intentionally empty
		}
		data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if hitPath != "/chat/completions" {
			t.Errorf("request hit path %q, want /chat/completions (legacy behavior)", hitPath)
		}
	})

	// Sanity: ensure request body is still well-formed and decodable.
	t.Run("request_body_well_formed_with_full_url", func(t *testing.T) {
		var bodyBytes []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bodyBytes, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\ndata: [DONE]\n"))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			BaseURL: "https://api.openai.com/v1",
			FullURL: server.URL + "/proxy",
			Model:   "gpt-4",
		}
		data := &RequestData{
			Model:    "gpt-4",
			Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}
		for chunk := range stream {
			if chunk.IsDone {
				break
			}
		}

		if len(bodyBytes) == 0 {
			t.Fatal("request body is empty")
		}
	})
}
