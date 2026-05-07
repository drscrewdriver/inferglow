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
