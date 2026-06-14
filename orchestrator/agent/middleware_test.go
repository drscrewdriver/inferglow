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

// loggingMiddleware records each call's userMessage and delegates to next.
func loggingMiddleware(log *[]string) Middleware {
	return func(next AgentHandler) AgentHandler {
		return func(ctx context.Context, userMessage string) (string, error) {
			*log = append(*log, "before:"+userMessage)
			resp, err := next(ctx, userMessage)
			*log = append(*log, "after:"+resp)
			return resp, err
		}
	}
}

// authBlockingMiddleware rejects messages containing "forbidden".
func authBlockingMiddleware() Middleware {
	return func(next AgentHandler) AgentHandler {
		return func(ctx context.Context, userMessage string) (string, error) {
			if strings.Contains(userMessage, "forbidden") {
				return "", errors.New("auth: message blocked by middleware")
			}
			return next(ctx, userMessage)
		}
	}
}

func TestMiddleware_LoggingMiddleware(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()
	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: `{"next_action":"response","final_response":"hello back"}`}
			close(ch)
			return ch, nil
		},
	}

	var log []string
	agent := New(sess, actExt, mockReq, WithMiddleware(loggingMiddleware(&log)))

	resp, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello back" {
		t.Fatalf("expected %q, got %q", "hello back", resp)
	}
	if len(log) != 2 {
		t.Fatalf("expected 2 log entries, got %d: %v", len(log), log)
	}
	if log[0] != "before:hi" {
		t.Errorf("expected log[0]=%q, got %q", "before:hi", log[0])
	}
	if log[1] != "after:hello back" {
		t.Errorf("expected log[1]=%q, got %q", "after:hello back", log[1])
	}
}

func TestMiddleware_AuthBlocking(t *testing.T) {
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

	agent := New(sess, actExt, mockReq, WithMiddleware(authBlockingMiddleware()))

	// Normal message should pass through.
	_, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("normal message should pass: %v", err)
	}

	// Forbidden message should be blocked.
	_, err = agent.Run(context.Background(), "this is forbidden content")
	if err == nil {
		t.Fatal("expected auth error for forbidden message")
	}
	if !strings.Contains(err.Error(), "auth: message blocked") {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestMiddleware_ChainOrder(t *testing.T) {
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

	var order []string
	mw1 := func(next AgentHandler) AgentHandler {
		return func(ctx context.Context, msg string) (string, error) {
			order = append(order, "mw1-before")
			resp, err := next(ctx, msg)
			order = append(order, "mw1-after")
			return resp, err
		}
	}
	mw2 := func(next AgentHandler) AgentHandler {
		return func(ctx context.Context, msg string) (string, error) {
			order = append(order, "mw2-before")
			resp, err := next(ctx, msg)
			order = append(order, "mw2-after")
			return resp, err
		}
	}

	agent := New(sess, actExt, mockReq, WithMiddleware(mw1, mw2))
	_, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mw1 is outermost, mw2 is innermost.
	expected := []string{"mw1-before", "mw2-before", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], expected[i])
		}
	}
}

func TestMiddleware_NoMiddlewareZeroOverhead(t *testing.T) {
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

	// No WithMiddleware — should work identically to before.
	agent := New(sess, actExt, mockReq)
	resp, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected %q, got %q", "ok", resp)
	}
}
