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
