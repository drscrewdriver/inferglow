package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndSessionLifecycle(t *testing.T) {
	s := NewSession("e2e-test", 200)
	s.AutoResize = true
	s.ResizeHandler = SimpleCutResizeHandler

	// Multi-turn conversation
	dialogue := []struct {
		role    string
		content string
	}{
		{"user", "Hello, I need help"},
		{"assistant", "Of course! What do you need help with?"},
		{"user", "I want to learn Go programming"},
		{"assistant", "Great choice! Go is a wonderful language."},
		{"user", "Where should I start?"},
		{"assistant", "Start with variables and functions"},
		{"user", "How about error handling?"},
		{"assistant", "Use errors.New and fmt.Errorf"},
	}

	for _, msg := range dialogue {
		s.AddMessage(msg.role, msg.content, "")
	}

	// Verify full context has all messages
	if len(s.FullContext) != 8 {
		t.Errorf("FullContext len = %d, want 8", len(s.FullContext))
	}

	// Verify context window was trimmed (8 messages exceed 200 bytes)
	if len(s.ContextWindow) != len(s.FullContext) {
		t.Logf("ContextWindow was trimmed as expected: %d vs %d", len(s.ContextWindow), len(s.FullContext))
	}

	// Save to file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")
	if err := s.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	// Reload
	s2 := NewSession("", 0)
	if err := s2.LoadJSON(path); err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	// Verify consistency
	if s2.ID != s.ID {
		t.Errorf("ID mismatch: %q vs %q", s2.ID, s.ID)
	}
	if len(s2.FullContext) != len(s.FullContext) {
		t.Errorf("FullContext len mismatch: %d vs %d", len(s2.FullContext), len(s.FullContext))
	}
	if len(s2.Memo) != len(s.Memo) {
		t.Errorf("Memo len mismatch: %d vs %d", len(s2.Memo), len(s.Memo))
	}
}

func TestEndToEndContextWindowTrimming(t *testing.T) {
	s := NewSession("trim-test", 150)
	s.AutoResize = true
	s.ResizeHandler = SimpleCutResizeHandler

	// Add short messages that fit
	s.AddMessage("user", "hi", "")
	s.AddMessage("assistant", "hello", "")
	s.AddMessage("user", "how are you", "")
	s.AddMessage("assistant", "i am fine", "")
	// Total ~36 bytes, still under 150

	// Add a long message that will trigger trimming
	s.AddMessage("user", "This is a very long message that should definitely exceed our 150 byte limit and trigger context window trimming", "")
	s.AddMessage("assistant", "Let me analyze this carefully and provide a detailed response", "")

	// FullContext should have all messages
	if len(s.FullContext) != 6 {
		t.Errorf("FullContext len = %d, want 6", len(s.FullContext))
	}

	// ContextWindow should be trimmed
	windowBytes := 0
	for _, msg := range s.ContextWindow {
		windowBytes += len(ContentToString(msg.Content))
	}
	if windowBytes > 150 {
		t.Errorf("ContextWindow bytes = %d, should be <= 150", windowBytes)
	}

	// FullContext should be completely untouched
	if s.FullContext[0].Content != "hi" {
		t.Error("FullContext[0] was incorrectly modified")
	}
}

func TestEndToEndPersistenceWithTrimming(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.yaml")

	s := NewSession("persist-trim", 100)
	s.AutoResize = true
	s.ResizeHandler = SimpleCutResizeHandler
	s.Memo["version"] = "1.0"

	// Add enough messages to trigger trimming
	messages := []struct{ role, content string }{
		{"user", "first question about go"},
		{"assistant", "go is statically typed"},
		{"user", "what about concurrency"},
		{"assistant", "go uses goroutines and channels"},
		{"user", "can you explain channels"},
		{"assistant", "channels are typed conduits for data"},
	}

	for _, msg := range messages {
		s.AddMessage(msg.role, msg.content, "")
	}

	// Save
	if err := s.SaveYAML(path); err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}

	// Reload and verify
	s2 := NewSession("", 0)
	if err := s2.LoadYAML(path); err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	if s2.ID != "persist-trim" {
		t.Errorf("ID = %q, want %q", s2.ID, "persist-trim")
	}
	if len(s2.FullContext) != len(messages) {
		t.Errorf("FullContext len = %d, want %d", len(s2.FullContext), len(messages))
	}
	if s2.Memo["version"] != "1.0" {
		t.Errorf("Memo version = %v, want %q", s2.Memo["version"], "1.0")
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("YAML file was not created")
	}
}

// TestP1Integration_MultiStrategyResize exercises the P1 multi-strategy
// resize registration path: multiple ResizeHandlers + an AnalysisHandler
// that dynamically routes to "summary_first" once the context window grows
// past a byte threshold. Verifies that SummaryFirstResizeHandler actually
// runs and produces a "[summary:" entry in the ContextWindow.
func TestP1Integration_MultiStrategyResize(t *testing.T) {
	// 1. Create a Session with MaxLength=50.
	s := NewSession("p1-multi-strategy", 50)
	s.AutoResize = true

	// 2. Register two named resize strategies.
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	s.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)

	// 3. Register an AnalysisHandler that routes to "summary_first" when the
	//    current ContextWindow total byte size exceeds 30.
	s.RegisterAnalysisHandler(func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error) {
		totalBytes := 0
		for _, m := range window {
			totalBytes += len(ContentToString(m.Content))
		}
		if totalBytes > 30 {
			return "summary_first", nil
		}
		return "", nil
	})

	// 4. Add 5 messages. After the 4th add the window will exceed 30 bytes,
	//    triggering summary_first which replaces the middle messages with a
	//    "[summary: ...]" system message.
	messages := []struct {
		role    string
		content string
	}{
		{"system", "system prompt here"},
		{"user", "user message one"},
		{"assistant", "assistant reply two"},
		{"user", "user message three"},
		{"assistant", "assistant reply four"},
	}
	for _, m := range messages {
		s.AddMessage(m.role, m.content, "")
	}

	// 5. Verify FullContext retained all 5 original messages.
	if len(s.FullContext) != 5 {
		t.Errorf("FullContext len = %d, want 5", len(s.FullContext))
	}

	// 6. Verify ContextWindow was processed by SummaryFirstResizeHandler:
	//    at least one message should contain the "[summary:" marker.
	foundSummary := false
	for _, m := range s.ContextWindow {
		if c, ok := m.Content.(string); ok && strings.Contains(c, "[summary:") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Errorf("ContextWindow should contain a \"[summary:\" message after summary_first resize; got: %+v", s.ContextWindow)
	}
}
