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
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/session"
)

func TestSessionExtension_AddUserMessage(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 10000))

	ext.AddUserMessage("Hello, world!")
	prompt := ext.PreparePrompt()

	if len(prompt) != 1 {
		t.Fatalf("Expected 1 message in prompt, got %d", len(prompt))
	}
	if prompt[0].Role != "user" {
		t.Errorf("Expected role 'user', got %q", prompt[0].Role)
	}
	if prompt[0].Content != "Hello, world!" {
		t.Errorf("Content mismatch: got %q", prompt[0].Content)
	}
}

func TestSessionExtension_AddAssistantMessage(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 10000))

	ext.AddUserMessage("Hi")
	ext.AddAssistantMessage("Hello!")
	prompt := ext.PreparePrompt()

	if len(prompt) != 2 {
		t.Fatalf("Expected 2 messages in prompt, got %d", len(prompt))
	}
	if prompt[1].Role != "assistant" {
		t.Errorf("Second message role: got %q, want %q", prompt[1].Role, "assistant")
	}
}

func TestSessionExtension_AddActionResult(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 10000))

	result := &action.ActionResult{
		OK:     true,
		Status: "success",
		Result: "calculation done",
	}
	ext.AddActionResult("calc", result)
	prompt := ext.PreparePrompt()

	if len(prompt) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(prompt))
	}
	if prompt[0].Content == "" {
		t.Error("Content should not be empty")
	}
}

// TestSessionExtension_FileReadDedup verifies that repeated file_read
// results for the same path are replaced with a short reference marker.
func TestSessionExtension_FileReadDedup(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 100000))

	// First read: full content should be stored.
	firstContent := `{"path":"/src/main.go","bytes_read":1024,"content":"package main..."}`
	ext.AddToolResultNamed("tc1", "file_read", firstContent)

	// Second read of the same file: should be replaced with a marker.
	secondContent := `{"path":"/src/main.go","bytes_read":1024,"content":"package main..."}`
	ext.AddToolResultNamed("tc2", "file_read", secondContent)

	prompt := ext.PreparePrompt()
	if len(prompt) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(prompt))
	}

	// First message should have the original content.
	if prompt[0].Content != firstContent {
		t.Errorf("first message should have original content, got %q", prompt[0].Content)
	}

	// Second message should be the dedup marker.
	if prompt[1].Content == secondContent {
		t.Error("second message should be dedup marker, not the full content")
	}
	if len(prompt[1].Content) >= len(secondContent) {
		t.Errorf("dedup marker should be shorter than original: got %d >= %d", len(prompt[1].Content), len(secondContent))
	}
}

// TestSessionExtension_NonFileReadNotDedup verifies that non-file_read tools
// are not subject to deduplication.
func TestSessionExtension_NonFileReadNotDedup(t *testing.T) {
	ext := NewSessionExtension(session.NewSession("test", 100000))

	content := `{"result":"output"}`
	ext.AddToolResultNamed("tc1", "bash_executor", content)
	ext.AddToolResultNamed("tc2", "bash_executor", content)

	prompt := ext.PreparePrompt()
	if len(prompt) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(prompt))
	}
	// Both should have the original content.
	if prompt[0].Content != content || prompt[1].Content != content {
		t.Error("non-file_read results should not be deduplicated")
	}
}
