package extension

import (
	"testing"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// TestAddUserContentBlocksFlow verifies that multimodal content blocks added
// via AddUserContentBlocks are preserved through PreparePrompt into
// model.ChatMessage.ContentBlocks (the agent-path image/audio/video channel).
func TestAddUserContentBlocksFlow(t *testing.T) {
	s := session.NewSession("contentblocks", 10000)
	ext := NewSessionExtension(s)

	img := model.ImageBlock("image/png", []byte{0x89, 0x50, 0x4e, 0x47})
	ext.AddUserContentBlocks("describe this", []model.ContentBlock{img})

	prompt := ext.PreparePrompt()
	if len(prompt) != 1 {
		t.Fatalf("want 1 prompt message, got %d", len(prompt))
	}
	m := prompt[0]
	if m.Role != "user" {
		t.Errorf("role: got %q want user", m.Role)
	}
	if len(m.ContentBlocks) != 2 {
		t.Fatalf("want 2 content blocks (text+image), got %d: %#v", len(m.ContentBlocks), m.ContentBlocks)
	}
	if m.ContentBlocks[0].Type != model.ContentText {
		t.Errorf("block[0] type: got %v, want text", m.ContentBlocks[0].Type)
	}
	if m.ContentBlocks[1].Type != model.ContentImage {
		t.Errorf("block[1] type: got %v, want image", m.ContentBlocks[1].Type)
	}
}

// TestAddUserMessageNoBlocks verifies the pure-text path stays string content
// (no regression to existing behavior).
func TestAddUserMessageNoBlocks(t *testing.T) {
	s := session.NewSession("text", 1000)
	ext := NewSessionExtension(s)
	ext.AddUserMessage("only text")

	prompt := ext.PreparePrompt()
	if len(prompt) != 1 {
		t.Fatalf("want 1 message, got %d", len(prompt))
	}
	if len(prompt[0].ContentBlocks) != 0 {
		t.Errorf("text message should have no content blocks, got %d", len(prompt[0].ContentBlocks))
	}
}