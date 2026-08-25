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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestModelEffortWireDeepSeek verifies that an OpenAICompatibleProvider
// with EffortFormat=deepseek + EffortLevels translates Options["reasoning_effort"]
// into the DeepSeek wire shape (thinking.enabled + reasoning_effort).
func TestRequestModelEffortWireDeepSeek(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL:      server.URL,
		Model:        "deepseek-v4-pro",
		EffortFormat: EffortDeepSeek,
		EffortLevels: EffortLevelMap{"low": "low", "high": "high", "max": "max"},
	}
	data := &RequestData{
		Model:    "deepseek-v4-pro",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"reasoning_effort": "max"},
	}
	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	thinking, ok := receivedBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking = %v, want {type:enabled}", receivedBody["thinking"])
	}
	if v, ok := receivedBody["reasoning_effort"].(string); !ok || v != "max" {
		t.Fatalf("reasoning_effort = %v, want \"max\"", receivedBody["reasoning_effort"])
	}
}

// TestRequestModelEffortWireOpenRouter verifies the reasoning:{effort} shape.
func TestRequestModelEffortWireOpenRouter(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL:      server.URL,
		Model:        "anthropic/claude-opus-4-7",
		EffortFormat: EffortOpenRouter,
		EffortLevels: EffortLevelMap{"xhigh": "xhigh", "max": "max"},
	}
	data := &RequestData{
		Model:    "anthropic/claude-opus-4-7",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"reasoning_effort": "max"},
	}
	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	reasoning, ok := receivedBody["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "max" {
		t.Fatalf("reasoning = %v, want {effort:max}", receivedBody["reasoning"])
	}
}

// TestRequestModelEffortWireGoogle verifies uppercase wire values (LOW/HIGH).
func TestRequestModelEffortWireGoogle(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL:      server.URL,
		Model:        "gemini-3.1-pro",
		EffortFormat: EffortGoogle,
		EffortLevels: EffortLevelMap{"low": "LOW", "high": "HIGH"},
	}
	data := &RequestData{
		Model:    "gemini-3.1-pro",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"reasoning_effort": "high"},
	}
	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	tc, ok := receivedBody["thinkingConfig"].(map[string]any)
	if !ok || tc["thinkingLevel"] != "HIGH" {
		t.Fatalf("thinkingConfig = %v, want {thinkingLevel:HIGH}", receivedBody["thinkingConfig"])
	}
}

// TestRequestModelEffortWireNotOffered verifies that a level the model does not
// offer (nil in the level map) is dropped entirely.
func TestRequestModelEffortWireNotOffered(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	provider := &OpenAICompatibleProvider{
		BaseURL:      server.URL,
		Model:        "deepseek-v4-pro",
		EffortFormat: EffortDeepSeek,
		// medium explicitly not offered (nil)
		EffortLevels: EffortLevelMap{"low": "low", "medium": nil, "high": "high", "max": "max"},
	}
	data := &RequestData{
		Model:    "deepseek-v4-pro",
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
		Options:  map[string]any{"reasoning_effort": "medium"},
	}
	stream, err := provider.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	for range stream {
	}

	if _, ok := receivedBody["thinking"]; ok {
		t.Fatalf("thinking should be absent for a not-offered level, got %v", receivedBody["thinking"])
	}
	if _, ok := receivedBody["reasoning_effort"]; ok {
		t.Fatalf("reasoning_effort should be absent for a not-offered level, got %v", receivedBody["reasoning_effort"])
	}
}
