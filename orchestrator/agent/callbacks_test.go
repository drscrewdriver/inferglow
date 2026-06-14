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

package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestCallbacks_LifecycleHooks verifies that all lifecycle hooks are called
// in the correct order during a simple executeLoop run.
func TestCallbacks_LifecycleHooks(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"hello"}`}
			close(ch)
			return ch, nil
		},
	}

	var mu sync.Mutex
	var events []string

	cb := &AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			mu.Lock()
			events = append(events, "RunStart:"+userMessage)
			mu.Unlock()
		},
		OnRunEnd: func(ctx context.Context, response string, err error) {
			mu.Lock()
			events = append(events, "RunEnd:"+response)
			mu.Unlock()
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			mu.Lock()
			events = append(events, "LLMStart")
			mu.Unlock()
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int) {
			mu.Lock()
			events = append(events, "LLMEnd")
			mu.Unlock()
		},
	}

	agent := New(sess, actExt, mockReq, WithCallbacks(cb))
	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello" {
		t.Fatalf("expected %q, got %q", "hello", resp)
	}

	// Expected order: RunStart → LLMStart → LLMEnd → RunEnd
	mu.Lock()
	defer mu.Unlock()
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d: %v", len(events), events)
	}
	if events[0] != "RunStart:hi" {
		t.Errorf("events[0] = %q, want %q", events[0], "RunStart:hi")
	}
	// LLMStart and LLMEnd should be between RunStart and RunEnd
	foundLLMStart, foundLLMEnd, foundRunEnd := false, false, false
	for _, e := range events[1:] {
		switch e {
		case "LLMStart":
			foundLLMStart = true
		case "LLMEnd":
			if foundLLMStart {
				foundLLMEnd = true
			}
		default:
			if e == "RunEnd:hello" || len(e) > 6 && e[:7] == "RunEnd:" {
				foundRunEnd = true
			}
		}
	}
	if !foundLLMStart {
		t.Error("OnLLMCallStart was not called")
	}
	if !foundLLMEnd {
		t.Error("OnLLMCallEnd was not called")
	}
	if !foundRunEnd {
		t.Error("OnRunEnd was not called")
	}
}

// TestCallbacks_NilCallbacks verifies that nil callbacks do not cause panics.
func TestCallbacks_NilCallbacks(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`}
			close(ch)
			return ch, nil
		},
	}

	// WithCallbacks(nil) should be safe.
	agent := New(sess, actExt, mockReq, WithCallbacks(nil))
	resp, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected %q, got %q", "ok", resp)
	}
}

// TestCallbacks_PartialCallbacks verifies that only set callbacks are invoked.
func TestCallbacks_PartialCallbacks(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`}
			close(ch)
			return ch, nil
		},
	}

	runStartCalled := false
	cb := &AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			runStartCalled = true
		},
		// All other fields are nil.
	}

	agent := New(sess, actExt, mockReq, WithCallbacks(cb))
	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !runStartCalled {
		t.Error("OnRunStart should have been called")
	}
}

// TestCallbacks_FireFunctionsNilSafety verifies that fire* functions
// handle nil callbacks and nil fields gracefully.
func TestCallbacks_FireFunctionsNilSafety(t *testing.T) {
	ctx := context.Background()

	// All of these should not panic with nil callbacks.
	fireOnRunStart(nil, ctx, "msg")
	fireOnRunEnd(nil, ctx, "resp", nil)
	fireOnLLMCallStart(nil, ctx, 0)
	fireOnLLMCallEnd(nil, ctx, 0, 0)
	fireOnToolCallStart(nil, ctx, "tool")
	fireOnToolCallEnd(nil, ctx, "tool", nil)

	// Empty callbacks (all fields nil) should also be safe.
	cb := &AgentCallbacks{}
	fireOnRunStart(cb, ctx, "msg")
	fireOnRunEnd(cb, ctx, "resp", nil)
	fireOnLLMCallStart(cb, ctx, 0)
	fireOnLLMCallEnd(cb, ctx, 0, 0)
	fireOnToolCallStart(cb, ctx, "tool")
	fireOnToolCallEnd(cb, ctx, "tool", nil)
}
