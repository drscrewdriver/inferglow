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
	"strings"
	"testing"
)

// TestOpenAIStreamThinkNormalization verifies that BroadcastResponse routes
// <think>-wrapped deltas through the LeadingThinkNormalizer so that:
//  1. ReasoningDelta events are emitted during streaming (not just at Done).
//  2. The final ModelResponse separates reasoning from answer content.
//
// Without the normalizer integration the stream emits raw EventDelta events
// containing the <think> tags and no ReasoningDelta events at all.
func TestOpenAIStreamThinkNormalization(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Delta: "<think>reasoning"}
	stream <- &StreamChunk{Delta: "</think>answer"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var sawReasoningDelta bool
	var reasoningDeltaContent strings.Builder
	var deltaContent strings.Builder
	var donePayload *ModelResponse
	for ev := range events {
		switch ev.EventType {
		case ReasoningDelta:
			sawReasoningDelta = true
			if s, ok := ev.Payload.(string); ok {
				reasoningDeltaContent.WriteString(s)
			}
		case EventDelta:
			if s, ok := ev.Payload.(string); ok {
				deltaContent.WriteString(s)
			}
		case EventDone:
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if !sawReasoningDelta {
		t.Error("expected at least one ReasoningDelta event during streaming, got none")
	}
	if reasoningDeltaContent.String() != "reasoning" {
		t.Errorf("streaming reasoning delta = %q, want %q", reasoningDeltaContent.String(), "reasoning")
	}
	if deltaContent.String() != "answer" {
		t.Errorf("streaming delta = %q, want %q (think tags must not leak into deltas)", deltaContent.String(), "answer")
	}
	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if donePayload.Reasoning != "reasoning" {
		t.Errorf("ModelResponse.Reasoning = %q, want %q", donePayload.Reasoning, "reasoning")
	}
	if donePayload.Content != "answer" {
		t.Errorf("ModelResponse.Content = %q, want %q", donePayload.Content, "answer")
	}
}

// TestOpenAIStreamThinkNoTags verifies that deltas without <think> tags
// pass through unchanged — no spurious ReasoningDelta events and content
// accumulates normally.
func TestOpenAIStreamThinkNoTags(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Delta: "hello world"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var sawReasoningDelta bool
	var donePayload *ModelResponse
	for ev := range events {
		switch ev.EventType {
		case ReasoningDelta:
			sawReasoningDelta = true
		case EventDone:
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if sawReasoningDelta {
		t.Error("did not expect ReasoningDelta events for plain text deltas")
	}
	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if donePayload.Reasoning != "" {
		t.Errorf("ModelResponse.Reasoning = %q, want empty", donePayload.Reasoning)
	}
	if donePayload.Content != "hello world" {
		t.Errorf("ModelResponse.Content = %q, want %q", donePayload.Content, "hello world")
	}
}
