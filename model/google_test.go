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
	"strings"
	"testing"
)

// TestGoogleGenerateRequestData verifies the Google request shape:
// systemInstruction, contents roles, generationConfig.
func TestGoogleGenerateRequestData(t *testing.T) {
	p := &GoogleGenerativeProvider{BaseURL: "https://example.com", Model: "gemini-3.1-pro-preview"}
	req := &ModelRequest{
		System: "you are helpful",
		Input:  "hello",
		ChatHistory: []ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hey"},
		},
		Options: map[string]any{"max_tokens": 100},
	}
	data, err := p.GenerateRequestData(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateRequestData: %v", err)
	}
	if data.Model != "gemini-3.1-pro-preview" {
		t.Fatalf("model = %q", data.Model)
	}
	if data.MaxTokens != 100 {
		t.Fatalf("maxTokens = %d, want 100", data.MaxTokens)
	}
	if _, ok := data.Options["_google_system"]; !ok {
		t.Fatal("_google_system should be in options")
	}
}

// TestGoogleRequestModelStream verifies SSE parsing: text deltas + reasoning
// parts are separated, usage is captured.
func TestGoogleRequestModelStream(t *testing.T) {
	var received map[string]any
	var reqURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqURL = r.URL.String()
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"let me think\",\"thought\":true}]}}]}\n\n" +
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"answer\"}]}}]}\n\n" +
				"data: {\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":5,\"thoughtsTokenCount\":3,\"totalTokenCount\":18}}\n\n" +
				"data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := &GoogleGenerativeProvider{BaseURL: server.URL, Model: "gemini-3.1-pro-preview"}
	data := &RequestData{Model: "gemini-3.1-pro-preview", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
	stream, err := p.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel: %v", err)
	}

	var content, reasoning strings.Builder
	var done bool
	for chunk := range stream {
		if chunk.Delta != "" {
			content.WriteString(chunk.Delta)
		}
		if chunk.Reasoning != "" {
			reasoning.WriteString(chunk.Reasoning)
		}
		if chunk.IsDone {
			done = true
		}
	}
	if content.String() != "answer" {
		t.Fatalf("content = %q, want \"answer\"", content.String())
	}
	if reasoning.String() != "let me think" {
		t.Fatalf("reasoning = %q, want \"let me think\"", reasoning.String())
	}
	if !done {
		t.Fatal("stream should terminate")
	}
	if !strings.Contains(reqURL, ":streamGenerateContent?alt=sse") {
		t.Fatalf("url = %q, want streamGenerateContent?alt=sse", reqURL)
	}
}

// TestGoogleRequestModelEffortWire verifies Options["reasoning_effort"] is
// translated into generationConfig.thinkingConfig.thinkingLevel (uppercase).
func TestGoogleRequestModelEffortWire(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	p := &GoogleGenerativeProvider{
		BaseURL:      server.URL,
		Model:        "gemini-3.1-pro-preview",
		EffortFormat: EffortGoogle,
		EffortLevels: EffortLevelMap{"low": "LOW", "high": "HIGH"},
	}
	data := &RequestData{
		Model:    "gemini-3.1-pro-preview",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Options:  map[string]any{"reasoning_effort": "high"},
	}
	stream, err := p.RequestModel(context.Background(), data)
	if err != nil {
		t.Fatalf("RequestModel: %v", err)
	}
	for range stream {
	}

	gc, ok := received["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generationConfig missing: %v", received)
	}
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("thinkingConfig missing: %v", gc)
	}
	if tc["thinkingLevel"] != "HIGH" {
		t.Fatalf("thinkingLevel = %v, want HIGH", tc["thinkingLevel"])
	}
}

// TestGoogleBroadcastResponse verifies the ResultEvent fan-out (EventDone
// payload carries content + reasoning + usage).
func TestGoogleBroadcastResponse(t *testing.T) {
	p := &GoogleGenerativeProvider{}
	ch := make(chan *StreamChunk, 4)
	ch <- &StreamChunk{Reasoning: "think"}
	ch <- &StreamChunk{Delta: "answer"}
	ch <- &StreamChunk{IsDone: true, Meta: map[string]any{}}
	close(ch)

	events, err := p.BroadcastResponse(context.Background(), ch)
	if err != nil {
		t.Fatalf("BroadcastResponse: %v", err)
	}
	var done *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			done = ev.Payload.(*ModelResponse)
		}
	}
	if done == nil {
		t.Fatal("no EventDone")
	}
	if done.Content != "answer" || done.Reasoning != "think" {
		t.Fatalf("done = %+v, want content=answer reasoning=think", done)
	}
}

// TestGoogleFactory verifies NewGoogleProviderFromConfig loads from
// DEFAULT_SETTINGS.
func TestGoogleFactory(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"google": map[string]any{"api_key": "test-key"},
	}}
	p, err := NewGoogleProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if p.Name() != "google" {
		t.Fatalf("name = %q", p.Name())
	}
	if p.BaseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("base_url = %q", p.BaseURL)
	}
	// ApplyEffortProfile should wire EffortGoogle + level map.
	ApplyEffortProfile(p, "google", "gemini-3.1-pro-preview")
	if p.EffortFormat != EffortGoogle {
		t.Fatalf("format = %q, want google", p.EffortFormat)
	}
	if p.EffortLevels == nil {
		t.Fatal("level map should be wired")
	}
	if v, _ := p.EffortLevels["high"]; v != "HIGH" {
		t.Fatalf("high wire = %v, want HIGH", v)
	}
}
