package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_New(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global")

	store := NewStore(dir, globalDir)
	if store.Dir != dir {
		t.Errorf("Dir = %q, want %q", store.Dir, dir)
	}
	if store.GlobalDir != globalDir {
		t.Errorf("GlobalDir = %q, want %q", store.GlobalDir, globalDir)
	}
	if store.cache == nil {
		t.Error("cache should not be nil")
	}
}

func TestStore_Save(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	sk := Skill{
		Name:        "test-skill",
		Description: "A test skill",
		Body:        "1. Do something",
		RunAs:       RunAsInline,
		Scope:       ScopeProject,
	}

	if err := store.Save(sk); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-skill.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file %s should exist after Save", path)
	}

	// Verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "name: test-skill") {
		t.Errorf("file content should contain 'name: test-skill', got: %s", content)
	}
}

func TestStore_Save_Global(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global")
	store := NewStore(dir, globalDir)

	sk := Skill{
		Name:        "global-skill",
		Description: "A global skill",
		Body:        "1. Do globally",
		RunAs:       RunAsInline,
		Scope:       ScopeGlobal,
	}

	if err := store.Save(sk); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	path := filepath.Join(globalDir, "global-skill.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file %s should exist after Save with ScopeGlobal", path)
	}
}

func TestStore_Read(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	original := Skill{
		Name:        "read-test",
		Description: "Test reading skills",
		Body:        "1. Read me\n2. Verify me",
		RunAs:       RunAsInline,
		Scope:       ScopeProject,
	}

	if err := store.Save(original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read from disk (cache should be populated by Save)
	read, ok := store.Read("read-test")
	if !ok {
		t.Fatal("Read should return true for existing skill")
	}

	if read.Name != original.Name {
		t.Errorf("Name = %q, want %q", read.Name, original.Name)
	}
	if read.Description != original.Description {
		t.Errorf("Description = %q, want %q", read.Description, original.Description)
	}
	if strings.TrimSpace(read.Body) != strings.TrimSpace(original.Body) {
		t.Errorf("Body = %q, want %q", read.Body, original.Body)
	}
	if read.Scope != original.Scope {
		t.Errorf("Scope = %q, want %q", read.Scope, original.Scope)
	}
}

func TestStore_Read_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	_, ok := store.Read("nonexistent")
	if ok {
		t.Error("Read should return false for nonexistent skill")
	}
}

func TestStore_Read_Global(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global")
	store := NewStore(dir, globalDir)

	sk := Skill{
		Name:        "global-read",
		Description: "Global read test",
		Body:        "1. Global",
		RunAs:       RunAsInline,
		Scope:       ScopeGlobal,
	}
	if err := store.Save(sk); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	read, ok := store.Read("global-read")
	if !ok {
		t.Fatal("Read should find global skill")
	}
	if read.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want %q", read.Scope, ScopeGlobal)
	}
}

func TestStore_List(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	skills := []Skill{
		{Name: "skill-a", Description: "First skill", Body: "A", RunAs: RunAsInline, Scope: ScopeProject},
		{Name: "skill-b", Description: "Second skill", Body: "B", RunAs: RunAsInline, Scope: ScopeProject},
		{Name: "skill-c", Description: "Third skill", Body: "C", RunAs: RunAsInline, Scope: ScopeProject},
	}

	for _, sk := range skills {
		if err := store.Save(sk); err != nil {
			t.Fatalf("Save %s failed: %v", sk.Name, err)
		}
	}

	list := store.List()
	if len(list) != 3 {
		t.Errorf("List returned %d skills, want 3", len(list))
	}

	// Verify all names are present
	names := make(map[string]bool)
	for _, sk := range list {
		names[sk.Name] = true
	}
	for _, sk := range skills {
		if !names[sk.Name] {
			t.Errorf("List missing skill %q", sk.Name)
		}
	}
}

func TestStore_List_GlobalAndProject(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global")
	store := NewStore(dir, globalDir)

	projectSkill := Skill{
		Name: "proj-skill", Description: "Project skill", Body: "P",
		RunAs: RunAsInline, Scope: ScopeProject,
	}
	globalSkill := Skill{
		Name: "glob-skill", Description: "Global skill", Body: "G",
		RunAs: RunAsInline, Scope: ScopeGlobal,
	}

	if err := store.Save(projectSkill); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(globalSkill); err != nil {
		t.Fatal(err)
	}

	list := store.List()
	if len(list) != 2 {
		t.Errorf("List returned %d skills, want 2", len(list))
	}
}

func TestStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	sk := Skill{
		Name: "delete-me", Description: "To be deleted", Body: "delete",
		RunAs: RunAsInline, Scope: ScopeProject,
	}
	if err := store.Save(sk); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it exists
	if _, ok := store.Read("delete-me"); !ok {
		t.Fatal("Skill should exist before delete")
	}

	// Delete
	if err := store.Delete("delete-me"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	path := filepath.Join(dir, "delete-me.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not exist after Delete")
	}

	// Verify cache is cleared
	if _, ok := store.Read("delete-me"); ok {
		t.Error("Read should return false after Delete")
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	err := store.Delete("nonexistent")
	if err == nil {
		t.Error("Delete should return error for nonexistent skill")
	}
}

func TestStore_ProjectInstructions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	content := "# Project Instructions\n\nRun `go test ./...` before committing."

	// Create AGENTS.md (second priority)
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := store.ProjectInstructions(dir)
	if result != content {
		t.Errorf("ProjectInstructions = %q, want %q", result, content)
	}
}

func TestStore_ProjectInstructions_Priority(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	// Create all three files
	os.WriteFile(filepath.Join(dir, "INFERGLOW.md"), []byte("inferglow content"), 0644)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents content"), 0644)
	os.WriteFile(filepath.Join(dir, "REASONIX.md"), []byte("reasonix content"), 0644)

	result := store.ProjectInstructions(dir)
	// INFERGLOW.md has highest priority
	if result != "inferglow content" {
		t.Errorf("ProjectInstructions = %q, want %q", result, "inferglow content")
	}
}

func TestStore_ProjectInstructions_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	result := store.ProjectInstructions(dir)
	if result != "" {
		t.Errorf("ProjectInstructions = %q, want empty string", result)
	}
}

func TestStore_IndexBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	skills := []Skill{
		{Name: "alpha", Description: "First skill", Body: "A", RunAs: RunAsInline, Scope: ScopeProject},
		{Name: "beta", Description: "Second skill", Body: "B", RunAs: RunAsInline, Scope: ScopeProject},
	}

	for _, sk := range skills {
		if err := store.Save(sk); err != nil {
			t.Fatal(err)
		}
	}

	block := store.IndexBlock()
	if !strings.Contains(block, "alpha") {
		t.Error("IndexBlock should contain 'alpha'")
	}
	if !strings.Contains(block, "beta") {
		t.Error("IndexBlock should contain 'beta'")
	}
	if !strings.Contains(block, "First skill") {
		t.Error("IndexBlock should contain 'First skill'")
	}
	if !strings.Contains(block, "Second skill") {
		t.Error("IndexBlock should contain 'Second skill'")
	}
	if !strings.Contains(block, "run_skill") {
		t.Error("IndexBlock should contain 'run_skill'")
	}
}

func TestStore_IndexBlock_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "")

	block := store.IndexBlock()
	if block != "" {
		t.Errorf("IndexBlock = %q, want empty string", block)
	}
}
