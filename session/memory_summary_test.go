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

package session

import (
	"strings"
	"testing"
)

// mockSummarizer is a test double that returns a fixed summary.
type mockSummarizer struct {
	summary string
	err     error
}

func (m *mockSummarizer) Summarize(input string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.summary, nil
}

// TestSummaryMemory_NoSummarizer verifies that without a summarizer,
// SummaryMemory acts as a simple message store.
func TestSummaryMemory_NoSummarizer(t *testing.T) {
	mem := NewSummaryMemory(WithTokenThreshold(10))
	msgs := []ChatMessage{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	}
	mem.Save(msgs)
	loaded := mem.Load()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}
}

// TestSummaryMemory_AutoSummarize verifies that when token count exceeds
// the threshold, old messages are summarized.
func TestSummaryMemory_AutoSummarize(t *testing.T) {
	summarizer := &mockSummarizer{summary: "old conversation summary"}
	// Use a custom estimate func: each message = 10 tokens.
	mem := NewSummaryMemory(
		WithTokenThreshold(25),
		WithSummarizer(summarizer),
		WithEstimateFunc(func(s string) int { return 10 }),
	)

	// 4 messages × 10 tokens = 40 > 25 threshold → triggers summarization.
	msgs := []ChatMessage{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "msg2"},
		{Role: "user", Content: "msg3"},
		{Role: "assistant", Content: "msg4"},
	}
	mem.Save(msgs)

	loaded := mem.Load()
	// After summarization: 1 summary + 2 recent messages = 3.
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages (1 summary + 2 recent), got %d", len(loaded))
	}
	// First message should be the summary.
	if loaded[0].Role != "system" {
		t.Errorf("expected summary role 'system', got %q", loaded[0].Role)
	}
	content := ContentToString(loaded[0].Content)
	if !strings.HasPrefix(content, "[summary:") {
		t.Errorf("expected summary content to start with '[summary:', got %q", content)
	}
	if !strings.Contains(content, "old conversation summary") {
		t.Errorf("expected summary to contain %q, got %q", "old conversation summary", content)
	}
}

// TestSummaryMemory_BelowThreshold verifies no summarization when under threshold.
func TestSummaryMemory_BelowThreshold(t *testing.T) {
	summarizer := &mockSummarizer{summary: "should not appear"}
	mem := NewSummaryMemory(
		WithTokenThreshold(1000),
		WithSummarizer(summarizer),
		WithEstimateFunc(func(s string) int { return 5 }),
	)

	msgs := []ChatMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	}
	mem.Save(msgs)

	loaded := mem.Load()
	// 2 messages × 5 tokens = 10 < 1000 → no summarization.
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages (no summarization), got %d", len(loaded))
	}
}

// TestSummaryMemory_TooFewMessages verifies no summarization when ≤2 messages.
func TestSummaryMemory_TooFewMessages(t *testing.T) {
	summarizer := &mockSummarizer{summary: "should not appear"}
	mem := NewSummaryMemory(
		WithTokenThreshold(1),
		WithSummarizer(summarizer),
		WithEstimateFunc(func(s string) int { return 100 }),
	)

	// Only 2 messages → skip summarization even if over threshold.
	msgs := []ChatMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	}
	mem.Save(msgs)

	loaded := mem.Load()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages (too few to summarize), got %d", len(loaded))
	}
}

// TestSummaryMemory_Clear verifies Clear resets all state.
func TestSummaryMemory_Clear(t *testing.T) {
	mem := NewSummaryMemory()
	mem.Save([]ChatMessage{{Role: "user", Content: "test"}})
	mem.Clear()
	if got := mem.Load(); len(got) != 0 {
		t.Fatalf("expected 0 messages after Clear, got %d", len(got))
	}
}

// TestSummaryMemory_AsResizeHandler verifies the resize handler integration.
func TestSummaryMemory_AsResizeHandler(t *testing.T) {
	mem := NewSummaryMemory(WithTokenThreshold(1000))
	handler := mem.AsResizeHandler()

	window := []ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	result, err := handler(nil, window)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
}
