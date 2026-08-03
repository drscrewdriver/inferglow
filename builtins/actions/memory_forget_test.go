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

	"github.com/inferglow/memory"
)

// newMemoryTestStore creates a memory.Store backed by a temporary directory
// and saves the given memories into it. Cleanup is automatic via t.TempDir.
func newMemoryTestStore(t *testing.T, memories ...memory.Memory) memory.Store {
	t.Helper()
	store := memory.Store{Dir: t.TempDir()}
	for _, m := range memories {
		if _, err := store.Save(m); err != nil {
			t.Fatalf("failed to save memory %q: %v", m.Name, err)
		}
	}
	return store
}

func TestForgetExecutor_NormalArchive(t *testing.T) {
	store := newMemoryTestStore(t, memory.Memory{
		Name:        "test-memory",
		Description: "a test memory",
		Body:        "hello world",
	})

	a := NewMemoryForgetAction(MemoryForgetConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "test-memory",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "archived" {
		t.Errorf("Status = %q, want %q", res.Status, "archived")
	}

	// Verify the memory is no longer loadable.
	if _, ok := store.Load("test-memory"); ok {
		t.Error("memory should not be loadable after archive")
	}
}

func TestForgetExecutor_EmptyName(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryForgetAction(MemoryForgetConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for empty name")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error != "forget: name is required" {
		t.Errorf("Error = %q, want %q", res.Error, "forget: name is required")
	}
}

func TestForgetExecutor_MemoryNotFound(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryForgetAction(MemoryForgetConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "nonexistent",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for missing memory")
	}
	if res.Status != "not_found" {
		t.Errorf("Status = %q, want %q", res.Status, "not_found")
	}
	if res.Error != `forget: memory "nonexistent" not found` {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestForgetExecutor_ArchiveFails(t *testing.T) {
	dir := t.TempDir()
	store := memory.Store{Dir: dir}

	// Save a memory first.
	_, err := store.Save(memory.Memory{
		Name:        "blocked-memory",
		Description: "will be blocked",
		Body:        "cannot archive",
	})
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Create a file at the .archive path to prevent MkdirAll from creating
	// a directory there.
	archivePath := filepath.Join(dir, ".archive")
	if err := os.WriteFile(archivePath, []byte("i am a file, not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	a := NewMemoryForgetAction(MemoryForgetConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "blocked-memory",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when archive fails")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error == "" {
		t.Error("expected non-empty error message")
	}
}