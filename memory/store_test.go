package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_Save(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	m := Memory{
		Title:       "Save Test",
		Type:        TypeProject,
		Body:        "Save test content.",
		Description: "save test description",
	}

	path, err := store.Save(m)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("File should exist after save: %s", path)
	}

	// Verify the file is in the correct directory
	if filepath.Dir(path) != dir {
		t.Errorf("File should be saved in store dir, got dir: %q, want: %q", filepath.Dir(path), dir)
	}
}

func TestStore_Load(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	original := Memory{
		Title:       "Load Test",
		Type:        TypeUser,
		Scope:       FactScopeGlobal,
		Body:        "Load test body content.",
		Description: "load test description",
	}

	savedPath, err := store.Save(original)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	_ = savedPath

	// Load by name (auto-generated slug from description)
	loaded, ok := store.Load("load-test-description")
	if !ok {
		t.Fatal("Load should find the saved memory")
	}

	if loaded.Title != original.Title {
		t.Errorf("Title = %q, want %q", loaded.Title, original.Title)
	}
	if loaded.Type != original.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, original.Type)
	}
	if loaded.Scope != original.Scope {
		t.Errorf("Scope = %q, want %q", loaded.Scope, original.Scope)
	}
	if strings.TrimRight(loaded.Body, "\n") != original.Body {
		t.Errorf("Body = %q, want %q", loaded.Body, original.Body)
	}
	if loaded.Description != original.Description {
		t.Errorf("Description = %q, want %q", loaded.Description, original.Description)
	}
}

func TestStore_List(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	memories := []Memory{
		{Title: "Memory A", Type: TypeProject, Body: "Body A", Description: "memory-a"},
		{Title: "Memory B", Type: TypeUser, Body: "Body B", Description: "memory-b"},
		{Title: "Memory C", Type: TypeReference, Body: "Body C", Description: "memory-c"},
	}

	for _, m := range memories {
		if _, err := store.Save(m); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	list := store.List()
	if len(list) != 3 {
		t.Errorf("List should return 3 memories, got %d", len(list))
	}

	// Verify list is sorted by name
	for i := 1; i < len(list); i++ {
		if list[i].Name < list[i-1].Name {
			t.Errorf("List should be sorted by name: %q < %q", list[i].Name, list[i-1].Name)
		}
	}
}

func TestStore_Archive(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	m := Memory{
		Title:       "Archive Test",
		Type:        TypeProject,
		Body:        "Archive test body.",
		Description: "archive-test-memory",
	}

	if _, err := store.Save(m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it's in the list
	if len(store.List()) != 1 {
		t.Fatal("Expected 1 memory before archive")
	}

	// Archive it
	if err := store.Archive("archive-test-memory"); err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	// Should be removed from active list
	if len(store.List()) != 0 {
		t.Errorf("Expected 0 memories after archive, got %d", len(store.List()))
	}

	// File should be moved to .archive/
	archivePath := filepath.Join(dir, ".archive", "archive-test-memory.md")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Errorf("Archived file should exist at: %s", archivePath)
	}

	// Original file should be removed
	originalPath := filepath.Join(dir, "archive-test-memory.md")
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Errorf("Original file should be removed after archive")
	}
}

func TestStore_Index(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	// Empty store should return empty index
	if idx := store.Index(); idx != "" {
		t.Errorf("Empty store should return empty index, got: %q", idx)
	}

	// Save a memory
	_, err := store.Save(Memory{
		Title:       "Index Test",
		Type:        TypeProject,
		Body:        "Index test body.",
		Description: "index-test-memory",
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Index should contain the memory
	idx := store.Index()
	if idx == "" {
		t.Fatal("Index should not be empty after saving a memory")
	}
	if !contains(idx, "Index Test") {
		t.Errorf("Index should contain the memory title, got: %s", idx)
	}
	if !contains(idx, "index-test-memory") {
		t.Errorf("Index should contain the memory name, got: %s", idx)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
