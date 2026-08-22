package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionFileExists(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create one real session file.
	if err := os.WriteFile(filepath.Join(sessDir, "abc.jsonl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := CLIConfig{DataDir: dir}

	if !sessionFileExists(cfg, "abc") {
		t.Error("abc should exist")
	}
	if sessionFileExists(cfg, "missing") {
		t.Error("missing should not exist")
	}
	// Path traversal / slashes must be rejected.
	if sessionFileExists(cfg, "../../etc/passwd") {
		t.Error("path traversal must be rejected")
	}
	// Empty id rejected.
	if sessionFileExists(cfg, "") {
		t.Error("empty id must be rejected")
	}
}