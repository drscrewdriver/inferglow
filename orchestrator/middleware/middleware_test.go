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

package middleware

import (
	"context"
	"errors"
	"testing"
)

func TestChain_NoMiddlewares_PassThrough(t *testing.T) {
	called := false
	handler := func(ctx context.Context, input *Input) (*Output, error) {
		called = true
		return &Output{Messages: []Message{{Role: "assistant", Content: "hello"}}}, nil
	}

	chained := Chain()(handler)
	out, err := chained(context.Background(), &Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if len(out.Messages) != 1 || out.Messages[0].Content != "hello" {
		t.Errorf("unexpected output: %+v", out)
	}
}

func TestChain_SingleMiddleware_WrapsHandler(t *testing.T) {
	var order []string

	mw := func(next Handler) Handler {
		return func(ctx context.Context, input *Input) (*Output, error) {
			order = append(order, "before")
			out, err := next(ctx, input)
			order = append(order, "after")
			return out, err
		}
	}

	handler := func(ctx context.Context, input *Input) (*Output, error) {
		order = append(order, "handler")
		return &Output{}, nil
	}

	chained := Chain(mw)(handler)
	_, err := chained(context.Background(), &Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"before", "handler", "after"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], expected[i])
		}
	}
}

func TestChain_MultipleMiddlewares_OutermostFirst(t *testing.T) {
	var order []string

	mw1 := func(next Handler) Handler {
		return func(ctx context.Context, input *Input) (*Output, error) {
			order = append(order, "mw1-before")
			out, err := next(ctx, input)
			order = append(order, "mw1-after")
			return out, err
		}
	}
	mw2 := func(next Handler) Handler {
		return func(ctx context.Context, input *Input) (*Output, error) {
			order = append(order, "mw2-before")
			out, err := next(ctx, input)
			order = append(order, "mw2-after")
			return out, err
		}
	}

	handler := func(ctx context.Context, input *Input) (*Output, error) {
		order = append(order, "handler")
		return &Output{}, nil
	}

	chained := Chain(mw1, mw2)(handler)
	_, err := chained(context.Background(), &Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First middleware in the list should be outermost (executed first on entry).
	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], expected[i])
		}
	}
}

func TestChain_Middleware_PropagatesError(t *testing.T) {
	wantErr := errors.New("middleware error")
	mw := func(next Handler) Handler {
		return func(ctx context.Context, input *Input) (*Output, error) {
			return nil, wantErr
		}
	}

	handler := func(ctx context.Context, input *Input) (*Output, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	chained := Chain(mw)(handler)
	_, err := chained(context.Background(), &Input{})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestChain_Middleware_ModifiesOutput(t *testing.T) {
	mw := func(next Handler) Handler {
		return func(ctx context.Context, input *Input) (*Output, error) {
			out, err := next(ctx, input)
			if err != nil {
				return nil, err
			}
			out.Messages = append(out.Messages, Message{Role: "system", Content: "appended"})
			return out, nil
		}
	}

	handler := func(ctx context.Context, input *Input) (*Output, error) {
		return &Output{Messages: []Message{{Role: "assistant", Content: "original"}}}, nil
	}

	chained := Chain(mw)(handler)
	out, err := chained(context.Background(), &Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out.Messages))
	}
	if out.Messages[1].Content != "appended" {
		t.Errorf("second message = %q, want %q", out.Messages[1].Content, "appended")
	}
}
