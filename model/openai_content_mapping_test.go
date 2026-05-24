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

// TestOpenAIContentMapping verifies that OpenAICompatibleProvider uses
// ContentMapping to extract reasoning/delta from non-standard SSE field
// paths, and falls back to the legacy struct-based parsing when ContentMapping
// is nil.
func TestOpenAIContentMapping(t *testing.T) {
	t.Run("custom_reasoning_path_via_content_mapping", func(t *testing.T) {
		// Vendor returns reasoning under data.thinking instead of the
		// standard choices[0].delta.reasoning_content path.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			// The SSE payload deliberately puts reasoning in a non-standard
			// location. ContentMapping must override the struct path.
			sse := "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}],\"data\":{\"thinking\":\"hmm\"}}\n"
			sse += "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n"
			sse += "data: [DONE]\n"
			w.Write([]byte(sse))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			BaseURL: server.URL,
			Model:   "gpt-4",
			ContentMapping: ContentMapping{
				"reasoning": "data.thinking",
				"delta":     "choices[0].delta.content",
			},
		}
		data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}

		var sawReasoning string
		var sawDelta string
		for chunk := range stream {
			if chunk.Reasoning != "" {
				sawReasoning = chunk.Reasoning
			}
			if chunk.Delta != "" {
				sawDelta = chunk.Delta
			}
			if chunk.IsDone {
				break
			}
		}

		if sawReasoning != "hmm" {
			t.Errorf("reasoning = %q, want %q (via ContentMapping)", sawReasoning, "hmm")
		}
		if sawDelta != "answer" {
			t.Errorf("delta = %q, want %q", sawDelta, "answer")
		}
	})

	t.Run("nil_content_mapping_preserves_legacy_behavior", func(t *testing.T) {
		// Standard OpenAI reasoning_content path must still work when
		// ContentMapping is nil. This guards the backward-compatibility
		// contract: the field is opt-in.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			sse := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"legacy reasoning\",\"content\":\"answer\"}}]}\n"
			sse += "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n"
			sse += "data: [DONE]\n"
			w.Write([]byte(sse))
		}))
		defer server.Close()

		provider := &OpenAICompatibleProvider{
			BaseURL: server.URL,
			Model:   "gpt-4",
			// ContentMapping intentionally nil.
		}
		data := &RequestData{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

		stream, err := provider.RequestModel(context.Background(), data)
		if err != nil {
			t.Fatalf("RequestModel failed: %v", err)
		}

		var sawReasoning string
		for chunk := range stream {
			if chunk.Reasoning != "" {
				sawReasoning = chunk.Reasoning
			}
			if chunk.IsDone {
				break
			}
		}

		if sawReasoning != "legacy reasoning" {
			t.Errorf("reasoning = %q, want %q (legacy reasoning_content path)", sawReasoning, "legacy reasoning")
		}
	})
}
