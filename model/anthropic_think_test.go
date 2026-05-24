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
	"testing"
)

// TestAnthropicThinkNormalization verifies that BroadcastResponse routes
// <think>-wrapped text deltas through the LeadingThinkNormalizer so that
// reasoning and answer content are properly separated in the final
// ModelResponse.
func TestAnthropicThinkNormalization(t *testing.T) {
	provider := &AnthropicCompatibleProvider{}
	stream := make(chan *StreamChunk, 3)
	// Anthropic text_delta arriving as a single chunk containing the
	// complete <think>...</think> block plus answer.
	stream <- &StreamChunk{Delta: "<think>r</think>ans"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	var sawReasoningDelta bool
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

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if !sawReasoningDelta {
		t.Error("expected at least one ReasoningDelta event during streaming")
	}
	if donePayload.Reasoning != "r" {
		t.Errorf("ModelResponse.Reasoning = %q, want %q", donePayload.Reasoning, "r")
	}
	if donePayload.Content != "ans" {
		t.Errorf("ModelResponse.Content = %q, want %q", donePayload.Content, "ans")
	}
}

// TestAnthropicThinkNoTags verifies that plain text deltas (without
// <think> tags) pass through unchanged — no spurious ReasoningDelta
// events and content accumulates normally.
func TestAnthropicThinkNoTags(t *testing.T) {
	provider := &AnthropicCompatibleProvider{}
	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Delta: "hello from claude"}
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
	if donePayload.Content != "hello from claude" {
		t.Errorf("ModelResponse.Content = %q, want %q", donePayload.Content, "hello from claude")
	}
}
