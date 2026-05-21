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
	"net/http"
	"net/http/httptest"
	"testing"
)

// M-HIGH-2: Options must not override reserved fields when expanded.
func TestGenerateRequestData_OptionsWhitelist(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{BaseURL: server.URL, Model: "gpt-4o-mini"}
	data := &RequestData{
		Model:    "gpt-4o-mini",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options: map[string]any{
			"model":    "gpt-4o",                              // should be filtered out
			"messages": []any{map[string]any{"role": "evil"}}, // should be filtered
			"stream":   false,                                 // should be filtered
			"top_p":    0.9,                                   // should be passed through (non-reserved)
		},
	}

	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	// Verify reserved fields are NOT overridden by Options
	if m, ok := receivedBody["model"].(string); !ok || m != "gpt-4o-mini" {
		t.Errorf("model = %v, want \"gpt-4o-mini\" (Options[\"model\"] must be filtered)", receivedBody["model"])
	}
	if s, ok := receivedBody["stream"].(bool); !ok || s != true {
		t.Errorf("stream = %v, want true (Options[\"stream\"] must be filtered)", receivedBody["stream"])
	}
	msgs, ok := receivedBody["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Errorf("messages = %v, want exactly 1 message (Options[\"messages\"] must be filtered)", receivedBody["messages"])
	}

	// Verify non-reserved fields ARE passed through
	if topP, ok := receivedBody["top_p"].(float64); !ok || topP != 0.9 {
		t.Errorf("top_p = %v, want 0.9 (non-reserved Options should pass through)", receivedBody["top_p"])
	}
}

// M-HIGH-2: helper verifying the reservedFields map covers all critical fields.
func TestReservedFields_CoversCriticalFields(t *testing.T) {
	critical := []string{
		"model", "messages", "stream", "tools",
		"tool_choice", "response_format", "max_tokens", "temperature",
	}
	for _, k := range critical {
		if !reservedFields[k] {
			t.Errorf("reservedFields[%q] = false, want true", k)
		}
	}
}
