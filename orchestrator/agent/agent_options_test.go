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

package agent

import (
	"context"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestWithModelOptions_MergedIntoRequest verifies that WithModelOptions keys
// are merged into the ModelRequest.Options and override engine-built keys.
func TestWithModelOptions_MergedIntoRequest(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var captured *model.ModelRequest
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			captured = req
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	// Empty ActionExtension => no tools => engine sets force_json=true.
	agent := New(sess, actExt, mockReq)
	_, err := agent.Run(context.Background(), "hi",
		WithModelOptions(map[string]any{"reasoning_effort": "high"}))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if captured == nil {
		t.Fatal("no ModelRequest captured")
	}
	if captured.Options["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, want high", captured.Options["reasoning_effort"])
	}
	// Engine-built key must still be present.
	if captured.Options["force_json"] != true {
		t.Errorf("force_json = %v, want true", captured.Options["force_json"])
	}
}

// TestWithModelOptions_OverrideEngineKey verifies caller keys win over the
// engine-built keys. With tools the engine sets max_tokens=16384.
func TestWithModelOptions_OverrideEngineKey(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	var captured *model.ModelRequest
	mockReq := &mockModelRequester{
		requestFn: func(ctx context.Context, req *model.ModelRequest) {
			captured = req
		},
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	_, err := agent.Run(context.Background(), "hi",
		WithModelOptions(map[string]any{"max_tokens": 999}))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if captured.Options["max_tokens"] != 999 {
		t.Errorf("max_tokens = %v, want 999 (caller wins)", captured.Options["max_tokens"])
	}
}

// TestWithModelRequester_OverrideResponse verifies that WithModelRequester
// switches the engine to the new requester for this run.
func TestWithModelRequester_OverrideResponse(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	// Default requester returns "default-model".
	defaultReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"default-model"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}
	// Override requester returns "override-model".
	overrideReq := &mockModelRequester{
		nameFn: func() string { return "override" },
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"override-model"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, defaultReq)
	result, err := agent.Run(context.Background(), "hi", WithModelRequester(overrideReq))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result != "override-model" {
		t.Errorf("result = %q, want override-model", result)
	}
}

// TestWithModelRequester_DefaultWhenNil verifies that without the option the
// Agent's construction-time requester is used (zero behavior change).
func TestWithModelRequester_DefaultWhenNil(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	defaultReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 1)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"default-model"}`, IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, defaultReq)
	result, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result != "default-model" {
		t.Errorf("result = %q, want default-model", result)
	}
}
