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
