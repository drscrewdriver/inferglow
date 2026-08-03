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

package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/action"
)

func TestListDirActionSpec(t *testing.T) {
	a := NewListDirAction(ListDirConfig{})
	if a.Name != ListDirActionID {
		t.Errorf("Name = %q, want %q", a.Name, ListDirActionID)
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

func TestListDirActionRegistry(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewListDirAction(ListDirConfig{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(ListDirActionID) {
		t.Errorf("registry missing %q", ListDirActionID)
	}
}

func TestListDirExecutorSuccess(t *testing.T) {
	// Create a temp directory with a file and a subdirectory.
	dir := t.TempDir()
	file1 := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}

	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{dir}})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	result, ok := res.Result.(ListDirResult)
	if !ok {
		t.Fatalf("Result not ListDirResult: %T", res.Result)
	}
	if result.Path != dir {
		t.Errorf("Path = %q, want %q", result.Path, dir)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}

	// Check entries (order may vary).
	entryMap := make(map[string]DirEntry)
	for _, e := range result.Entries {
		entryMap[e.Name] = e
	}

	if e, ok := entryMap["hello.txt"]; !ok {
		t.Error("missing hello.txt entry")
	} else if e.Type != "file" {
		t.Errorf("hello.txt type = %q, want file", e.Type)
	} else if e.Size != 5 {
		t.Errorf("hello.txt size = %d, want 5", e.Size)
	}

	if e, ok := entryMap["sub"]; !ok {
		t.Error("missing sub entry")
	} else if e.Type != "dir" {
		t.Errorf("sub type = %q, want dir", e.Type)
	}
}

func TestListDirExecutorMissingPath(t *testing.T) {
	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{"/tmp"}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing path")
	}
	if res.Error != "list_dir: path is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestListDirExecutorPathTraversal(t *testing.T) {
	dir := t.TempDir()
	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{dir}})
	// Attempt to list a path outside the allowed directory.
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": os.TempDir(),
	})
	if res.OK {
		t.Errorf("expected OK=false for path traversal")
	}
	if res.Status != "blocked" {
		t.Errorf("Status = %q, want blocked", res.Status)
	}
	if res.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestListDirExecutorNotADirectory(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file1, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": file1,
	})
	if res.OK {
		t.Errorf("expected OK=false for non-directory path")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestListDirExecutorEmptyAllowedDirs(t *testing.T) {
	dir := t.TempDir()
	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": dir,
	})
	if res.OK {
		t.Errorf("expected OK=false when no allowed dirs")
	}
	if res.Status != "blocked" {
		t.Errorf("Status = %q, want blocked", res.Status)
	}
}

func TestListDirExecutorNonExistentPath(t *testing.T) {
	dir := t.TempDir()
	a := NewListDirAction(ListDirConfig{AllowedDirs: []string{dir}})
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"path": filepath.Join(dir, "nonexistent"),
	})
	if res.OK {
		t.Errorf("expected OK=false for non-existent path")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestIsUnderAllowedDir(t *testing.T) {
	// Use OS-appropriate paths for the separator check.
	allowedDir := t.TempDir()
	subDir := filepath.Join(allowedDir, "sub")
	otherDir := filepath.Join(t.TempDir(), "other")

	tests := []struct {
		name    string
		path    string
		allowed []string
		want    bool
	}{
		{"exact match", allowedDir, []string{allowedDir}, true},
		{"under dir", subDir, []string{allowedDir}, true},
		{"outside dir", otherDir, []string{allowedDir}, false},
		{"empty allowed", allowedDir, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUnderAllowedDir(tt.path, tt.allowed)
			if got != tt.want {
				t.Errorf("isUnderAllowedDir(%q, %v) = %v, want %v", tt.path, tt.allowed, got, tt.want)
			}
		})
	}
}