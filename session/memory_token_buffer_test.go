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
	"testing"
)

// TestTokenBufferMemory_BasicOperations verifies Load, Save, and Clear.
func TestTokenBufferMemory_BasicOperations(t *testing.T) {
	mem := NewTokenBufferMemory(WithTokenBudget(1000))

	if got := mem.Load(); len(got) != 0 {
		t.Fatalf("expected empty Load, got %d messages", len(got))
	}

	msgs := []ChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	mem.Save(msgs)

	loaded := mem.Load()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages after Save, got %d", len(loaded))
	}
	if loaded[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", loaded[0].Role)
	}

	mem.Clear()
	if got := mem.Load(); len(got) != 0 {
		t.Fatalf("expected empty Load after Clear, got %d messages", len(got))
	}
}

// TestTokenBufferMemory_TokenBudget verifies that messages are trimmed
// when the budget is exceeded.
func TestTokenBufferMemory_TokenBudget(t *testing.T) {
	// Each message = 10 tokens. Budget = 25 → can hold 2 messages.
	mem := NewTokenBufferMemory(
		WithTokenBudget(25),
		WithTokenEstimateFunc(func(s string) int { return 10 }),
	)

	msgs := []ChatMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	}
	mem.Save(msgs)

	loaded := mem.Load()
	// 3 × 10 = 30 > 25 → trim oldest → 2 messages remain.
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages after trimming, got %d", len(loaded))
	}
	// The first remaining message should be "b" (the second one).
	if ContentToString(loaded[0].Content) != "b" {
		t.Errorf("expected first remaining message to be 'b', got %q", ContentToString(loaded[0].Content))
	}
}

// TestTokenBufferMemory_AddMessage verifies AddMessage appends and trims.
func TestTokenBufferMemory_AddMessage(t *testing.T) {
	mem := NewTokenBufferMemory(
		WithTokenBudget(20),
		WithTokenEstimateFunc(func(s string) int { return 10 }),
	)

	mem.AddMessage(ChatMessage{Role: "user", Content: "a"})
	mem.AddMessage(ChatMessage{Role: "assistant", Content: "b"})
	// 2 × 10 = 20, exactly at budget.
	loaded := mem.Load()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages at budget, got %d", len(loaded))
	}

	mem.AddMessage(ChatMessage{Role: "user", Content: "c"})
	// 3 × 10 = 30 > 20 → trim → 2 remain.
	loaded = mem.Load()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages after AddMessage trim, got %d", len(loaded))
	}
}

// TestTokenBufferMemory_TokenCount verifies the token counting.
func TestTokenBufferMemory_TokenCount(t *testing.T) {
	mem := NewTokenBufferMemory(
		WithTokenBudget(1000),
		WithTokenEstimateFunc(func(s string) int { return len(s) }),
	)

	mem.Save([]ChatMessage{
		{Role: "user", Content: "hello"},     // 5
		{Role: "assistant", Content: "world"}, // 5
	})

	if tc := mem.TokenCount(); tc != 10 {
		t.Errorf("expected TokenCount=10, got %d", tc)
	}
}

// TestTokenBufferMemory_PreserveLastMessage verifies that even when a
// single message exceeds the budget, it is preserved.
func TestTokenBufferMemory_PreserveLastMessage(t *testing.T) {
	mem := NewTokenBufferMemory(
		WithTokenBudget(5),
		WithTokenEstimateFunc(func(s string) int { return 100 }),
	)

	mem.Save([]ChatMessage{
		{Role: "user", Content: "huge message that exceeds budget"},
	})

	loaded := mem.Load()
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message (preserved despite exceeding budget), got %d", len(loaded))
	}
}

// TestTokenBufferMemory_AsResizeHandler verifies the handler integration.
func TestTokenBufferMemory_AsResizeHandler(t *testing.T) {
	mem := NewTokenBufferMemory(WithTokenBudget(100))
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
