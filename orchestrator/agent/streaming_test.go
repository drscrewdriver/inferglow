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
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

func TestStreamRunReturnsChannel(t *testing.T) {
	sess := session.NewSession("test", 10000)
	actExt := NewActionExtension()

	mockReq := &mockModelRequester{
		responseFn: func(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
			// T-1: buffer must be ≥ number of chunks sent synchronously
			// before the consumer starts reading, otherwise the second
			// send blocks forever (deadlock). StreamRun returns the
			// channel only after RequestModel returns, so all chunks sent
			// inside responseFn must fit in the buffer.
			ch := make(chan *model.StreamChunk, 2)
			ch <- &model.StreamChunk{Delta: "Hello", IsDone: false}
			ch <- &model.StreamChunk{Delta: " World", IsDone: true}
			close(ch)
			return ch, nil
		},
	}

	agent := New(sess, actExt, mockReq)
	stream, err := agent.StreamRun(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("StreamRun returned error: %v", err)
	}
	if stream == nil {
		t.Fatal("Stream should not be nil")
	}

	// Read all chunks
	var fullText string
	for chunk := range stream {
		fullText += chunk.Delta
	}
	if fullText != "Hello World" {
		t.Errorf("Full text: got %q, want %q", fullText, "Hello World")
	}
}
