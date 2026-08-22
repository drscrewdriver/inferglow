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

package action

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveText_PrivateModeAndExclusiveCreate(t *testing.T) {
	root := t.TempDir()
	store := NewLocalSpillStore(root)

	ref, err := store.SaveText(context.Background(), "session-1", "file_read", "big.log", "hello world")
	if err != nil {
		t.Fatalf("SaveText error: %v", err)
	}
	if ref.Bytes != int64(len("hello world")) {
		t.Errorf("Bytes = %d, want %d", ref.Bytes, len("hello world"))
	}
	if ref.Locator == "" || ref.RetrievalHint == "" {
		t.Fatalf("expected non-empty locator and hint, got %+v", ref)
	}
	if _, err := os.Stat(ref.Locator); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	info, err := os.Stat(ref.Locator)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("artifact mode %o, want owner-only (0600)", perm)
		}
	}

	// Re-saving the same name must not collide (exclusive create).
	ref2, err := store.SaveText(context.Background(), "session-1", "file_read", "big.log", "second")
	if err != nil {
		t.Fatalf("second SaveText error: %v", err)
	}
	if ref2.Locator == ref.Locator {
		t.Errorf("expected distinct artifact paths, both %q", ref.Locator)
	}

	// Owner directories must be private (POSIX only; Windows has no mode bits).
	if runtime.GOOS != "windows" {
		ownerDir := filepath.Dir(ref.Locator)
		if perm := statPerm(t, ownerDir); perm&0o077 != 0 {
			t.Errorf("owner dir mode %o, want 0700", perm)
		}
	}
}

func TestSaveText_SanitizeName(t *testing.T) {
	root := t.TempDir()
	store := NewLocalSpillStore(root)

	ref, err := store.SaveText(context.Background(), "s", "tool", "../../etc/passwd\\evil", "x")
	if err != nil {
		t.Fatalf("SaveText error: %v", err)
	}
	base := filepath.Base(ref.Locator)
	if strings.ContainsAny(base, `/\`) {
		t.Errorf("artifact base %q contains path separators", base)
	}
	if strings.Contains(base, "..") {
		t.Errorf("artifact base %q contains dot-dot", base)
	}
	if strings.Contains(base, " ") || strings.Contains(base, ".") {
		// sanitized name may retain dots only inside a safe segment; require no traversal
	}
}

func TestSaveText_WriteFailure(t *testing.T) {
	// Root path that cannot be created as a directory (a file in the way).
	root := t.TempDir()
	blocker := filepath.Join(root, "spill")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	store := NewLocalSpillStore(root)
	if _, err := store.SaveText(context.Background(), "s", "tool", "a.txt", "content"); err == nil {
		t.Fatalf("expected error when owner dir cannot be created")
	}
}

func TestPositionalDigest_HeadTailOffset(t *testing.T) {
	content := []byte("0123456789ABCDEF")
	d := NewPositionalDigest(content, 4, 4)

	if d.Head != "0123" {
		t.Errorf("Head = %q, want %q", d.Head, "0123")
	}
	if d.Tail != "CDEF" {
		t.Errorf("Tail = %q, want %q", d.Tail, "CDEF")
	}
	if d.HeadBytes != 4 || d.TailBytes != 4 {
		t.Errorf("HeadBytes/TailBytes = %d/%d, want 4/4", d.HeadBytes, d.TailBytes)
	}
	if d.Offset != 12 {
		t.Errorf("Offset = %d, want 12", d.Offset)
	}
	if d.TotalBytes != 16 {
		t.Errorf("TotalBytes = %d, want 16", d.TotalBytes)
	}
	if !d.Truncated {
		t.Errorf("Truncated = false, want true")
	}
}

func TestPositionalDigest_SmallContentNotTruncated(t *testing.T) {
	content := []byte("abc")
	d := NewPositionalDigest(content, 8, 8)
	if d.Head != "abc" || d.Tail != "abc" {
		t.Errorf("Head/Tail = %q/%q, want both %q", d.Head, d.Tail, "abc")
	}
	if d.Truncated {
		t.Errorf("Truncated = true for content smaller than preview bounds")
	}
	if d.Lines != 1 {
		t.Errorf("Lines = %d, want 1", d.Lines)
	}
}

func TestPositionalDigest_MultiLineCount(t *testing.T) {
	d := NewPositionalDigest([]byte("a\nb\nc\n"), 4, 4)
	if d.Lines != 4 {
		t.Errorf("Lines = %d, want 4", d.Lines)
	}
}

func TestPositionalDigest_EmptyContent(t *testing.T) {
	d := NewPositionalDigest(nil, 4, 4)
	if d.TotalBytes != 0 || d.Lines != 0 {
		t.Errorf("TotalBytes/Lines = %d/%d, want 0/0", d.TotalBytes, d.Lines)
	}
	if d.Truncated {
		t.Errorf("Truncated = true for empty content")
	}
}

// statPerm is a test helper returning the permission bits of a path.
func statPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// textExecutor returns a fixed string result for registry spill tests.
type textExecutor struct {
	text string
}

func (e *textExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	return &ActionResult{OK: true, Status: "success", Result: e.text}, nil
}

// failingStore is a SpillStore that always rejects saves.
type failingStore struct{}

func (failingStore) SaveText(ctx context.Context, owner, source, suggestedName, content string) (*SpillRef, error) {
	return nil, errors.New("disk full")
}

func TestExecute_SpillReplacesOversize(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Action{Name: "echo", Executor: &textExecutor{text: strings.Repeat("x", 5000)}}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	r.SetSpiller(&DefaultSpiller{
		Store:          NewLocalSpillStore(t.TempDir()),
		Owner:          "sess",
		Source:         "echo",
		MaxInlineBytes: 100,
	})

	res, err := r.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(SpilledOutput)
	if !ok {
		t.Fatalf("Result not SpilledOutput: %T", res.Result)
	}
	if out.Locator == "" || out.RetrievalHint == "" || out.Digest == nil {
		t.Errorf("expected locator/hint/digest, got %+v", out)
	}
	if res.Metadata == nil || res.Metadata["spilled"] != true {
		t.Errorf("expected spilled metadata, got %+v", res.Metadata)
	}
	// Artifact keeps the full text.
	full, err := os.ReadFile(out.Locator)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(full) != strings.Repeat("x", 5000) {
		t.Errorf("artifact content mismatch (%d bytes)", len(full))
	}
}

func TestExecute_SpillBestEffortKeepsInlineOnFailure(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Action{Name: "echo", Executor: &textExecutor{text: strings.Repeat("y", 5000)}}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	r.SetSpiller(&DefaultSpiller{
		Store:          failingStore{},
		Owner:          "sess",
		Source:         "echo",
		MaxInlineBytes: 100,
	})

	res, err := r.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true despite spill failure, got %+v", res)
	}
	if text, ok := res.Result.(string); !ok || text != strings.Repeat("y", 5000) {
		t.Errorf("expected original inline result kept, got %T", res.Result)
	}
}

func TestExecute_SpillSkipsSmallResults(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Action{Name: "echo", Executor: &textExecutor{text: "small"}}); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	r.SetSpiller(&DefaultSpiller{
		Store:          NewLocalSpillStore(t.TempDir()),
		Owner:          "sess",
		Source:         "echo",
		MaxInlineBytes: 100,
	})

	res, err := r.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if text, ok := res.Result.(string); !ok || text != "small" {
		t.Errorf("expected inline result untouched, got %T", res.Result)
	}
}
