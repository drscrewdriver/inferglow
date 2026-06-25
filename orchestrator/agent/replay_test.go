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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/session"
)

func TestReplay_LoadGoldenSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden.json")

	// Create a golden session with PromptVersion.
	sd := session.SessionData{
		ID:            "golden-1",
		PromptVersion: "v1.0.0",
		FullContext: []session.ChatMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "how are you?"},
			{Role: "assistant", Content: "I'm fine, thanks!"},
		},
	}
	data, _ := json.MarshalIndent(sd, "", "  ")
	os.WriteFile(path, data, 0644)

	result, err := Replay(context.Background(), ReplayConfig{
		SessionFile:   path,
		PromptVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	if !result.Match {
		t.Errorf("expected Match=true, got false. Diffs: %v", result.Diffs)
	}
	if result.GoldenPromptVersion != "v1.0.0" {
		t.Errorf("GoldenPromptVersion = %q, want %q", result.GoldenPromptVersion, "v1.0.0")
	}
	if len(result.UserMessages) != 2 {
		t.Fatalf("UserMessages len = %d, want 2", len(result.UserMessages))
	}
	if result.UserMessages[0] != "hello" {
		t.Errorf("UserMessages[0] = %q, want %q", result.UserMessages[0], "hello")
	}
}

func TestReplay_PromptVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "golden.json")

	sd := session.SessionData{
		ID:            "golden-2",
		PromptVersion: "v1.0.0",
		FullContext: []session.ChatMessage{
			{Role: "user", Content: "test"},
		},
	}
	data, _ := json.MarshalIndent(sd, "", "  ")
	os.WriteFile(path, data, 0644)

	result, err := Replay(context.Background(), ReplayConfig{
		SessionFile:   path,
		PromptVersion: "v2.0.0",
	})
	if err != nil {
		t.Fatalf("Replay error: %v", err)
	}
	if result.Match {
		t.Error("expected Match=false for version mismatch")
	}
	if len(result.Diffs) == 0 {
		t.Error("expected at least one diff for version mismatch")
	}
}

func TestReplayCompare(t *testing.T) {
	msgs := []session.ChatMessage{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "answer"},
	}

	if !ReplayCompare(msgs, "answer") {
		t.Error("expected match for 'answer'")
	}
	if ReplayCompare(msgs, "wrong") {
		t.Error("expected no match for 'wrong'")
	}
}

func TestReplayToolCallSequence(t *testing.T) {
	expected := []string{"read_file", "write_file", "bash"}
	actual := []string{"read_file", "write_file", "bash"}

	match, diffs := ReplayToolCallSequence(expected, actual)
	if !match {
		t.Errorf("expected match, got diffs: %v", diffs)
	}

	actual2 := []string{"read_file", "bash"}
	match2, diffs2 := ReplayToolCallSequence(expected, actual2)
	if match2 {
		t.Error("expected mismatch for different lengths")
	}
	if len(diffs2) == 0 {
		t.Error("expected diffs for different lengths")
	}
}

func TestExtractText(t *testing.T) {
	// String content.
	if got := extractText("hello"); got != "hello" {
		t.Errorf("extractText(string) = %q, want %q", got, "hello")
	}

	// Slice content (multi-modal).
	content := []any{
		map[string]any{"type": "text", "text": "part1"},
		map[string]any{"type": "text", "text": "part2"},
	}
	if got := extractText(content); got != "part1part2" {
		t.Errorf("extractText(slice) = %q, want %q", got, "part1part2")
	}
}
