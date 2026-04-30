package session

import (
	"fmt"
	"reflect"
	"testing"
)

// msgFieldsEqual compares Role, Content, Name of two ChatMessages (ignoring Timestamp and Meta)
func msgFieldsEqual(a, b ChatMessage) bool {
	return a.Role == b.Role && a.Content == b.Content && a.Name == b.Name
}

// msgsEqual compares slices of ChatMessages using msgFieldsEqual
func msgsEqual(a, b []ChatMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !msgFieldsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestAddChatHistory(t *testing.T) {
s := NewSession("test", 4096)

// Add a few initial messages via AddMessage
s.AddMessage("system", "You are helpful.", "")
s.AddMessage("user", "Hello!", "")

messages := []ChatMessage{
{Role: "assistant", Content: "Hi there!"},
{Role: "user", Content: "How are you?"},
}

s.AddChatHistory(messages)

// Check FullContext: should have all 4 messages
expectedFull := []ChatMessage{
{Role: "system", Content: "You are helpful."},
{Role: "user", Content: "Hello!"},
{Role: "assistant", Content: "Hi there!"},
{Role: "user", Content: "How are you?"},
}
if !msgsEqual(s.FullContext, expectedFull) {
	t.Errorf("FullContext = %v, want %v", s.FullContext, expectedFull)
}

// Check ContextWindow: should also have all 4 messages
if !msgsEqual(s.ContextWindow, expectedFull) {
t.Errorf("ContextWindow = %v, want %v", s.ContextWindow, expectedFull)
}
}

func TestSetChatHistory(t *testing.T) {
s := NewSession("test", 4096)

// Set initial messages
s.SetChatHistory([]ChatMessage{
{Role: "system", Content: "You are helpful."},
{Role: "user", Content: "Hello!"},
})

// Set new messages - should replace ContextWindow
s.SetChatHistory([]ChatMessage{
{Role: "assistant", Content: "Hi there!"},
{Role: "user", Content: "How are you?"},
})

expected := []ChatMessage{
{Role: "assistant", Content: "Hi there!"},
{Role: "user", Content: "How are you?"},
}

if !reflect.DeepEqual(s.ContextWindow, expected) {
t.Errorf("ContextWindow = %v, want %v", s.ContextWindow, expected)
}

// ContextWindow should only have 2 messages
if len(s.ContextWindow) != 2 {
t.Errorf("ContextWindow length = %d, want 2", len(s.ContextWindow))
}
}

func TestPreparePrompt(t *testing.T) {
s := NewSession("test", 4096)

s.AddMessage("system", "You are helpful.", "")
s.AddMessage("user", "Hello!", "")
s.SetChatHistory([]ChatMessage{
{Role: "assistant", Content: "Hi there!"},
})

prompt := s.PreparePrompt()

expected := []ChatMessage{
{Role: "assistant", Content: "Hi there!"},
}

if !reflect.DeepEqual(prompt, expected) {
t.Errorf("PreparePrompt() = %v, want %v", prompt, expected)
}

// Verify it returns a copy (modifying the return value should not affect session)
	prompt[0].Content = "Modified"
	if fmt.Sprint(s.ContextWindow[0].Content) == "Modified" {
		t.Error("PreparePrompt did not return a copy; modifying the result affected the session")
	}
}

func TestGetFullContext(t *testing.T) {
s := NewSession("test", 4096)

s.AddMessage("system", "You are helpful.", "")
s.AddMessage("user", "Hello!", "")
s.AddMessage("assistant", "Hi there!", "")

full := s.GetFullContext()

expected := []ChatMessage{
{Role: "system", Content: "You are helpful."},
{Role: "user", Content: "Hello!"},
{Role: "assistant", Content: "Hi there!"},
}

if !msgsEqual(full, expected) {
	t.Errorf("GetFullContext() = %v, want %v", full, expected)
}

// Verify it returns a copy
	full[0].Content = "Modified"
	if fmt.Sprint(s.FullContext[0].Content) == "Modified" {
		t.Error("GetFullContext did not return a copy; modifying the result affected the session")
	}
}

func TestGetContextWindow(t *testing.T) {
s := NewSession("test", 4096)

s.AddMessage("system", "You are helpful.", "")
s.AddMessage("user", "Hello!", "")
s.SetChatHistory([]ChatMessage{
{Role: "assistant", Content: "Hi there!"},
})

window := s.GetContextWindow()

expected := []ChatMessage{
{Role: "assistant", Content: "Hi there!"},
}

if !reflect.DeepEqual(window, expected) {
t.Errorf("GetContextWindow() = %v, want %v", window, expected)
}

// Verify it returns a copy
	window[0].Content = "Modified"
	if fmt.Sprint(s.ContextWindow[0].Content) == "Modified" {
		t.Error("GetContextWindow did not return a copy; modifying the result affected the session")
	}
}

func TestFullContextNotAffectedBySet(t *testing.T) {
s := NewSession("test", 4096)

// Add some initial messages
s.AddMessage("system", "You are helpful.", "")
s.AddMessage("user", "Hello!", "")

fullLenBefore := len(s.FullContext)

// Set chat history to completely different messages
s.SetChatHistory([]ChatMessage{
{Role: "assistant", Content: "Different message 1"},
{Role: "user", Content: "Different message 2"},
{Role: "assistant", Content: "Different message 3"},
})

// FullContext should still have 2 messages (original messages)
if len(s.FullContext) != fullLenBefore {
t.Errorf("FullContext length changed from %d to %d after SetChatHistory; FullContext should not be affected", fullLenBefore, len(s.FullContext))
}

// FullContext should still contain the original messages
expectedFull := []ChatMessage{
{Role: "system", Content: "You are helpful."},
{Role: "user", Content: "Hello!"},
}
if !msgsEqual(s.FullContext, expectedFull) {
	t.Errorf("FullContext after SetChatHistory = %v, want %v", s.FullContext, expectedFull)
}
}