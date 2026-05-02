package session

import (
	"reflect"
	"testing"
)

// TestContentToStringMultipleTextBlocks verifies that ContentToString
// concatenates ALL text blocks, not just returns empty string.
// Regression for BUG-1: ContentToString collected parts but returned "".
func TestContentToStringMultipleTextBlocks(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Data: "Hello "},
		{Type: "text", Data: "World"},
	}
	got := ContentToString(blocks)
	want := "Hello World"
	if got != want {
		t.Errorf("ContentToString() = %q, want %q", got, want)
	}
}

// TestContentToStringSingleTextBlock verifies basic case still works.
func TestContentToStringSingleTextBlock(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Data: "only"},
	}
	got := ContentToString(blocks)
	want := "only"
	if got != want {
		t.Errorf("ContentToString() = %q, want %q", got, want)
	}
}

// TestContentToStringSkipsNonText verifies non-text blocks are skipped.
func TestContentToStringSkipsNonText(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Data: "keep"},
		{Type: "image", Data: []byte("binary")},
		{Type: "text", Data: "this"},
	}
	got := ContentToString(blocks)
	want := "keepthis"
	if got != want {
		t.Errorf("ContentToString() = %q, want %q", got, want)
	}
}

// TestContentToStringUsedByAutoResize verifies ContentToString is used
// for byte counting in the old ResizeHandler path. Without the fix,
// ContentToString returns "" for multi-block content, so totalBytes is
// undercounted and resize is never triggered.
func TestContentToStringUsedByAutoResize(t *testing.T) {
	s := NewSession("test", 5) // very small max length
	s.AutoResize = true
	s.ResizeHandler = func(full, window []ChatMessage) ([]ChatMessage, error) {
		// Trivial resize: keep only last message
		if len(window) > 0 {
			return window[len(window)-1:], nil
		}
		return window, nil
	}

	// First message: short string (2 bytes) — does not exceed MaxLength.
	s.AddMessage("system", "Hi", "")
	// Second message: multi-block content (10 bytes when concatenated).
	// Without the fix, ContentToString returns "" so totalBytes stays at 2
	// and resize is never triggered.
	blocks := []ContentBlock{
		{Type: "text", Data: "Hello"},
		{Type: "text", Data: "World"},
	}
	s.AddMessage("user", blocks, "")

	// After fix: totalBytes == 12 > 5, resize triggers, only last msg remains.
	// Before fix: totalBytes == 2, no resize, both msgs remain.
	if len(s.ContextWindow) != 1 {
		t.Errorf("expected resize to trigger (1 msg remaining), got %d msgs", len(s.ContextWindow))
	}
}

// TestPreparePromptMultipleTextBlocks verifies PreparePrompt concatenates
// all text blocks of a message into a single string.
// Regression for BUG-3: PreparePrompt overwrote prompt[i].Content per block.
func TestPreparePromptMultipleTextBlocks(t *testing.T) {
	s := NewSession("test", 4096)
	blocks := []ContentBlock{
		{Type: "text", Data: "part1 "},
		{Type: "image", Data: "url"},
		{Type: "text", Data: "part2"},
	}
	s.AddMessage("user", blocks, "")

	prompt := s.PreparePrompt()
	if len(prompt) != 1 {
		t.Fatalf("expected 1 prompt message, got %d", len(prompt))
	}
	want := "part1 part2"
	got, ok := prompt[0].Content.(string)
	if !ok {
		t.Fatalf("expected Content to be string, got %T: %v", prompt[0].Content, prompt[0].Content)
	}
	if got != want {
		t.Errorf("PreparePrompt() Content = %q, want %q", got, want)
	}
}

// TestPreparePromptPreservesStringContent verifies plain string content
// is preserved as-is.
func TestPreparePromptPreservesStringContent(t *testing.T) {
	s := NewSession("test", 4096)
	s.AddMessage("user", "plain string", "")

	prompt := s.PreparePrompt()
	if len(prompt) != 1 {
		t.Fatalf("expected 1 prompt message, got %d", len(prompt))
	}
	if !reflect.DeepEqual(prompt[0].Content, "plain string") {
		t.Errorf("Content = %v, want %q", prompt[0].Content, "plain string")
	}
}
