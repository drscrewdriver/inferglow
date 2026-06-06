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
	"reflect"
	"testing"
)

// Check 4.3.1: DefaultAnalysisHandler correctly detects overflow
func TestDefaultAnalysisHandlerOverflow(t *testing.T) {
	window := []ChatMessage{
		{Role: "user", Content: "This is a very long message that should exceed the limit"},
		{Role: "assistant", Content: "And another long response from the assistant side of the conversation"},
	}
	full := []ChatMessage{
		{Role: "system", Content: "You are helpful"},
	}

	// Window bytes > 50, should return true
	if !DefaultAnalysisHandler(full, window, 50) {
		t.Error("DefaultAnalysisHandler should return true when window exceeds maxLength")
	}

	// Window bytes <= 50, should return false
	if DefaultAnalysisHandler(full, window, 500) {
		t.Error("DefaultAnalysisHandler should return false when window is within maxLength")
	}
}

// Check 4.3.1: DefaultAnalysisHandler empty window
func TestDefaultAnalysisHandlerEmpty(t *testing.T) {
	if DefaultAnalysisHandler(nil, nil, 100) {
		t.Error("DefaultAnalysisHandler should return false for empty window")
	}
}

// Check 4.3.2: SimpleCutResizeHandler trims oldest messages
func TestSimpleCutResizeHandler(t *testing.T) {
	window := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
	}
	full := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
	}

	resized, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler failed: %v", err)
	}

	// Should keep only messages that fit within the remaining budget
	// After removing "You are helpful." (16 bytes) and "Hello!" (6 bytes),
	// "Hi there!" (10 bytes) should remain
	if len(resized) == 0 {
		t.Error("SimpleCutResizeHandler should keep at least the most recent message")
	}
	if len(resized) > 0 && resized[0].Role != "assistant" {
		t.Errorf("Should keep assistant message, got role=%q", resized[0].Role)
	}
}

// Check 4.3.3: SimpleCutResizeHandler fits within limit
func TestSimpleCutResizeHandlerFitsLimit(t *testing.T) {
	full := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello!"},
	}
	window := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
	}

	resized, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler failed: %v", err)
	}

	// Calculate bytes
	totalBytes := 0
	for _, m := range resized {
		totalBytes += len(ContentToString(m.Content))
	}

	// After trimming from front, remaining should fit in reasonable limit
	// The last message "Hi there!" is 10 bytes, which should definitely fit
	if totalBytes > 100 {
		t.Errorf("totalBytes = %d, should be small (kept only most recent messages)", totalBytes)
	}
}

// Check 4.3.4: SimpleCutResizeHandler preserves FullContext
func TestSimpleCutPreservesFullContext(t *testing.T) {
	full := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	window := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
	}

	originalFull := make([]ChatMessage, len(full))
	copy(originalFull, full)

	_, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler failed: %v", err)
	}

	if !reflect.DeepEqual(full, originalFull) {
		t.Error("FullContext was modified by SimpleCutResizeHandler")
	}
}

// Check 4.3.5: SimpleCutResizeHandler truncates single message overflow
func TestSimpleCutSingleMessageOverflow(t *testing.T) {
	full := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
	}
	window := []ChatMessage{
		{Role: "user", Content: "This is a very long message that definitely exceeds any reasonable length limit we might set for the context window in our application"},
	}

	resized, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler failed: %v", err)
	}

	// Should keep at least the most recent message
	if len(resized) != 1 {
		t.Fatalf("len(resized) = %d, want 1 (kept most recent message even if oversized)", len(resized))
	}

	// Message content may be trimmed but message should not be dropped entirely
	if resized[0].Content == "" {
		t.Error("Single overflowing message was dropped instead of kept with truncated content")
	}
}

// Check 4.3.6: AutoResize integration
func TestAutoResizeIntegration(t *testing.T) {
	s := NewSession("auto-resize-test", 50)
	s.AutoResize = true

	trigged := false
	s.ResizeHandler = func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		trigged = true
		return SimpleCutResizeHandler(fullContext, contextWindow)
	}

	// Add short messages that fit
	s.AddMessage("system", "You are helpful.", "")
	s.AddMessage("user", "Hello!", "")

	// No trigger yet
	if trigged {
		t.Error("ResizeHandler should not be triggered for messages within limit")
	}

	// Add a long message that exceeds 50 bytes
	s.AddMessage("assistant", "This is a very long response that should definitely exceed the 50 byte limit we set for testing auto-resize functionality", "")

	if !trigged {
		t.Error("ResizeHandler should be triggered when AutoResize=true and message exceeds MaxLength")
	}

	// FullContext should have all messages (3 messages total)
	if len(s.FullContext) != 3 {
		t.Errorf("FullContext len = %d, want 3", len(s.FullContext))
	}

	// ContextWindow should be trimmed
	if len(s.ContextWindow) >= 3 {
		t.Errorf("ContextWindow was not trimmed: len=%d, should be less than FullContext len=3", len(s.ContextWindow))
	}
}

// Check 4.3.7: SetResizeHandler allows custom handler
func TestSetResizeHandler(t *testing.T) {
	s := NewSession("custom-handler", 100)

	customHandler := func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		return nil, nil
	}

	s.ResizeHandler = customHandler

	if s.ResizeHandler == nil {
		t.Fatal("ResizeHandler should be settable")
	}

	// Handler is settable
	resized, err := s.ResizeHandler(nil, nil)
	if resized != nil || err != nil {
		t.Error("custom handler returned unexpected non-nil results")
	}
}

// Test SimpleCutResizeHandler with empty window
func TestSimpleCutEmptyWindow(t *testing.T) {
	full := []ChatMessage{}
	window := []ChatMessage{}

	_, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler with empty window returned error: %v", err)
	}
}

// Test SimpleCutResizeHandler preserves order
func TestSimpleCutPreservesOrder(t *testing.T) {
	full := []ChatMessage{}
	window := []ChatMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
		{Role: "user", Content: "third"},
	}

	resized, err := SimpleCutResizeHandler(full, window)
	if err != nil {
		t.Fatalf("SimpleCutResizeHandler failed: %v", err)
	}

	if len(resized) == 0 {
		t.Fatal("should keep at least one message")
	}

	// Should keep most recent message at the end
	if resized[len(resized)-1].Content != "third" {
		t.Errorf("last message should be 'third', got %q", resized[len(resized)-1].Content)
	}
}

// TestSmartCompressResizeHandler verifies that tool results in the middle are
// compressed while recent messages and the first message are preserved.
func TestSmartCompressResizeHandler(t *testing.T) {
	handler := SmartCompressResizeHandler(3)

	window := []ChatMessage{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Read file A"},
		{Role: "tool", Content: "... 20KB of file content ...", Meta: map[string]any{"path": "/a.go"}},
		{Role: "assistant", Content: "Let me analyze this"},
		{Role: "user", Content: "Read file B"},
		{Role: "tool", Content: "... 30KB of file content ...", Meta: map[string]any{"path": "/b.go"}},
		{Role: "assistant", Content: "Now I'll write the fix"},
		{Role: "user", Content: "Please proceed"},
		{Role: "assistant", Content: "Writing files now"},
	}

	resized, err := handler(nil, window)
	if err != nil {
		t.Fatalf("SmartCompressResizeHandler failed: %v", err)
	}

	// First message (system) should be preserved.
	if resized[0].Role != "system" {
		t.Errorf("first message should be system, got %q", resized[0].Role)
	}

	// Last 3 messages should be preserved intact.
	if len(resized) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(resized))
	}
	if resized[len(resized)-1].Content != "Writing files now" {
		t.Errorf("last message should be 'Writing files now', got %q", resized[len(resized)-1].Content)
	}

	// Middle tool results should be compressed.
	for _, m := range resized[1 : len(resized)-3] {
		if m.Role == "tool" {
			content, _ := m.Content.(string)
			if len(content) > 100 {
				t.Errorf("middle tool result should be compressed, got %d bytes: %q", len(content), content)
			}
		}
	}
}

// TestSmartCompressResizeHandlerSmallWindow verifies no compression when
// the window is already small enough.
func TestSmartCompressResizeHandlerSmallWindow(t *testing.T) {
	handler := SmartCompressResizeHandler(10)

	window := []ChatMessage{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "tool", Content: "big content here", Meta: map[string]any{"path": "/x.go"}},
	}

	resized, err := handler(nil, window)
	if err != nil {
		t.Fatalf("SmartCompressResizeHandler failed: %v", err)
	}

	if len(resized) != len(window) {
		t.Errorf("small window should be unchanged, got %d vs %d", len(resized), len(window))
	}
	// Tool content should NOT be compressed in small windows.
	if resized[2].Content != "big content here" {
		t.Errorf("tool content should be preserved in small window, got %q", resized[2].Content)
	}
}
