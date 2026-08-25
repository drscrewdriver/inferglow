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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWorkspace(t *testing.T) *WorkspaceSwitch {
	t.Helper()
	w := &WorkspaceSwitch{
		current:     WorkspaceInfo{Path: mustGetwd()},
		historyPath: filepath.Join(t.TempDir(), "hist.json"), // never touch the real ~/.inferglow file
	}
	// SetCurrentDir calls os.Chdir; restore the test binary's cwd afterwards.
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return w
}

func TestWorkspaceSetCurrentDir(t *testing.T) {
	dir := t.TempDir()
	w := newTestWorkspace(t)
	if err := w.SetCurrentDir(dir); err != nil {
		t.Fatalf("SetCurrentDir(%q): %v", dir, err)
	}
	abs, _ := filepath.Abs(dir)
	if w.GetCurrentDir() != abs {
		t.Fatalf("GetCurrentDir() = %q, want %q", w.GetCurrentDir(), abs)
	}
	if w.current.Previous == "" {
		t.Fatal("previous workspace should be recorded")
	}
	// File, not a directory.
	f := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.SetCurrentDir(f); err == nil {
		t.Fatal("SetCurrentDir(file) should fail")
	}
	// Nonexistent path.
	if err := w.SetCurrentDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("SetCurrentDir(nonexistent) should fail")
	}
}

func TestWorkspaceHistory(t *testing.T) {
	w := newTestWorkspace(t)
	dirs := []string{"/a", "/b", "/c", "/a", "/d"}
	for _, d := range dirs {
		w.AddHistory(d)
	}
	hist := w.GetHistory()
	if len(hist) > workspaceHistoryMax {
		t.Fatalf("history length %d exceeds max %d", len(hist), workspaceHistoryMax)
	}
	// Dedup: "/a" added twice → appears once.
	if strings.Count(strings.Join(hist, ","), "/a") != 1 {
		t.Fatalf("history should dedupe: %v", hist)
	}
	if hist[0] != "/d" {
		t.Fatalf("most recent should be first: %v", hist)
	}
	// Cap.
	w2 := newTestWorkspace(t)
	for i := 0; i < 20; i++ {
		w2.AddHistory("/p" + string(rune('a'+i%26)) + string(rune('0'+i/26%10)))
	}
	if len(w2.GetHistory()) > workspaceHistoryMax {
		t.Fatalf("history not capped: %d", len(w2.GetHistory()))
	}
}

func TestWorkspaceHistoryPersist(t *testing.T) {
	w := newTestWorkspace(t)
	path := filepath.Join(t.TempDir(), "hist.json")
	w.AddHistory("/x")
	w.AddHistory("/y")
	w.saveHistoryTo(path)

	w2 := newTestWorkspace(t)
	w2.loadHistoryFrom(path)
	if len(w2.GetHistory()) != 2 || w2.GetHistory()[0] != "/y" {
		t.Fatalf("loaded history wrong: %v", w2.GetHistory())
	}
}

func TestWorkspaceRenderStatus(t *testing.T) {
	w := newTestWorkspace(t)
	if out := w.RenderStatus(); out == "" {
		t.Fatal("RenderStatus() should be non-empty with a current dir")
	}
	if !strings.Contains(w.RenderStatus(), "📁") {
		t.Fatalf("RenderStatus() missing folder icon: %q", w.RenderStatus())
	}
	// Long paths get truncated.
	w.current.Path = strings.Repeat("x", 80)
	if strings.Contains(w.RenderStatus(), strings.Repeat("x", 80)) {
		t.Fatalf("RenderStatus() should truncate long paths: %q", w.RenderStatus())
	}
	empty := &WorkspaceSwitch{}
	if out := empty.RenderStatus(); out != "" {
		t.Fatalf("RenderStatus() on empty workspace should be \"\", got %q", out)
	}
}

func TestWorkspaceSwitchToPrevious(t *testing.T) {
	w := newTestWorkspace(t)
	if err := w.SwitchToPrevious(); err == nil {
		t.Fatal("SwitchToPrevious() without previous should fail")
	}
	orig := w.GetCurrentDir()
	dir := t.TempDir()
	if err := w.SetCurrentDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := w.SwitchToPrevious(); err != nil {
		t.Fatalf("SwitchToPrevious() after SetCurrentDir: %v", err)
	}
	if w.GetCurrentDir() != orig {
		t.Fatalf("SwitchToPrevious() = %q, want %q", w.GetCurrentDir(), orig)
	}
}
