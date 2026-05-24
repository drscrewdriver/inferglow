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

// TestOllamaReasoningFieldExtraction verifies that processOllamaLine
// extracts the reasoning field from Ollama's message object into
// StreamChunk.Reasoning alongside the content delta.
func TestOllamaReasoningFieldExtraction(t *testing.T) {
	provider := &OllamaProvider{}
	var emitted []*StreamChunk
	emit := func(c *StreamChunk) { emitted = append(emitted, c) }
	var usage *UsageInfo

	line := `{"done":false,"message":{"role":"assistant","content":"answer","reasoning":"hmm"}}`
	provider.processOllamaLine(line, &usage, emit)

	if len(emitted) != 1 {
		t.Fatalf("expected 1 StreamChunk, got %d", len(emitted))
	}
	chunk := emitted[0]
	if chunk.Delta != "answer" {
		t.Errorf("Delta = %q, want %q", chunk.Delta, "answer")
	}
	if chunk.Reasoning != "hmm" {
		t.Errorf("Reasoning = %q, want %q", chunk.Reasoning, "hmm")
	}
}

// TestOllamaStreamThinkNormalization verifies that BroadcastResponse
// routes <think>-wrapped content deltas through the LeadingThinkNormalizer
// so that reasoning and answer are properly separated.
func TestOllamaStreamThinkNormalization(t *testing.T) {
	provider := &OllamaProvider{}
	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Delta: "<think>step</think>result"}
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
	if donePayload.Reasoning != "step" {
		t.Errorf("ModelResponse.Reasoning = %q, want %q", donePayload.Reasoning, "step")
	}
	if donePayload.Content != "result" {
		t.Errorf("ModelResponse.Content = %q, want %q", donePayload.Content, "result")
	}
}

// TestOllamaContentMappingExtraction verifies that when ContentMapping
// is set, processOllamaLine extracts reasoning from a non-standard JSON
// path (e.g. "data.thinking") instead of the default message.reasoning.
func TestOllamaContentMappingExtraction(t *testing.T) {
	provider := &OllamaProvider{
		ContentMapping: ContentMapping{
			"reasoning": "data.thinking",
			"delta":     "message.content",
		},
	}
	var emitted []*StreamChunk
	emit := func(c *StreamChunk) { emitted = append(emitted, c) }
	var usage *UsageInfo

	// Non-standard structure: reasoning is under data.thinking, not
	// message.reasoning.
	line := `{"done":false,"data":{"thinking":"custom reasoning"},"message":{"role":"assistant","content":"answer"}}`
	provider.processOllamaLine(line, &usage, emit)

	if len(emitted) != 1 {
		t.Fatalf("expected 1 StreamChunk, got %d", len(emitted))
	}
	chunk := emitted[0]
	if chunk.Delta != "answer" {
		t.Errorf("Delta = %q, want %q", chunk.Delta, "answer")
	}
	if chunk.Reasoning != "custom reasoning" {
		t.Errorf("Reasoning = %q, want %q", chunk.Reasoning, "custom reasoning")
	}
}
