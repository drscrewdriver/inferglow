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
	"errors"
	"strings"
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// mockRateLimitHook is a test double for RateLimitHook.
type mockRateLimitHook struct {
	acquireFn func(ctx context.Context, providerName string, estimatedTokens int) error
	calls     int
}

func (m *mockRateLimitHook) Acquire(ctx context.Context, providerName string, estimatedTokens int) error {
	m.calls++
	if m.acquireFn != nil {
		return m.acquireFn(ctx, providerName, estimatedTokens)
	}
	return nil
}

// TestEngine_RateLimitHook_Allowed verifies that when the hook allows the
// request, executeLoop proceeds normally and the hook is called.
func TestEngine_RateLimitHook_Allowed(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`}
			close(ch)
			return ch, nil
		},
	}

	hook := &mockRateLimitHook{}
	engine := NewEngine(sess, actExt, mockReq)
	engine.rateLimitHook = hook

	_, err := engine.executeLoop(context.Background(), "hello", 1, "you are a helper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.calls == 0 {
		t.Fatal("expected rateLimitHook.Acquire to be called at least once")
	}
}

// TestEngine_RateLimitHook_Rejected verifies that when the hook returns an
// error, executeLoop aborts with a rate-limit error without calling RequestModel.
func TestEngine_RateLimitHook_Rejected(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	llmCalled := false
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			llmCalled = true
			ch := make(chan *model.StreamChunk, 1)
			close(ch)
			return ch, nil
		},
	}

	rateLimitErr := errors.New("rate limit exceeded: too many requests")
	hook := &mockRateLimitHook{
		acquireFn: func(ctx context.Context, providerName string, estimatedTokens int) error {
			return rateLimitErr
		},
	}
	engine := NewEngine(sess, actExt, mockReq)
	engine.rateLimitHook = hook

	_, err := engine.executeLoop(context.Background(), "hello", 1, "you are a helper")
	if err == nil {
		t.Fatal("expected error from rate limit hook, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("expected rate limit error, got: %v", err)
	}
	if llmCalled {
		t.Fatal("RequestModel should NOT have been called when rate limit hook rejects")
	}
}

// TestEngine_RateLimitHook_NilIsNoop verifies that a nil hook does not
// affect executeLoop behavior.
func TestEngine_RateLimitHook_NilIsNoop(t *testing.T) {
	sess := NewSessionExtension(session.NewSession("test", 10000))
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`}
			close(ch)
			return ch, nil
		},
	}

	engine := NewEngine(sess, actExt, mockReq)
	// rateLimitHook is nil by default

	_, err := engine.executeLoop(context.Background(), "hello", 1, "you are a helper")
	if err != nil {
		t.Fatalf("unexpected error with nil hook: %v", err)
	}
}

// TestAgent_Run_PropagatesRateLimitHook verifies that WithRateLimitHook
// on Agent.Run propagates the hook to the engine.
func TestAgent_Run_PropagatesRateLimitHook(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	hook := &mockRateLimitHook{}
	agent := New(sess, actExt, &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"ok"}`}
			close(ch)
			return ch, nil
		},
	}, WithRateLimitHook(hook))

	_, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.calls == 0 {
		t.Fatal("expected rateLimitHook to be propagated and called")
	}
}
