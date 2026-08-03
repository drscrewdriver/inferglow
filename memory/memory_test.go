package memory

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestType_Normalize(t *testing.T) {
	tests := []struct {
		input string
		want  Type
	}{
		{"user", TypeUser},
		{"feedback", TypeFeedback},
		{"project", TypeProject},
		{"reference", TypeReference},
		{"USER", TypeUser},
		{"User", TypeUser},
		{"unknown", TypeProject},      // defaults to TypeProject
		{"random-value", TypeProject}, // defaults to TypeProject
		{"", TypeProject},
	}
	for _, tt := range tests {
		got := NormalizeType(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFactScope_Normalize(t *testing.T) {
	tests := []struct {
		input string
		want  FactScope
	}{
		{"global", FactScopeGlobal},
		{"GLOBAL", FactScopeGlobal},
		{"Global", FactScopeGlobal},
		{"project", FactScopeProject},
		{"long_term", FactScopeProject}, // defaults to FactScopeProject
		{"", FactScopeProject},
	}
	for _, tt := range tests {
		got := NormalizeFactScope(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeFactScope(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMemory_New(t *testing.T) {
	now := time.Now()
	m := Memory{
		Title:       "Test Memory Title",
		Type:        TypeUser,
		Scope:       FactScopeGlobal,
		Body:        "This is the memory body content.",
		Description: "A description of this memory.",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if m.Title != "Test Memory Title" {
		t.Errorf("Title = %q, want %q", m.Title, "Test Memory Title")
	}
	if m.Type != TypeUser {
		t.Errorf("Type = %q, want %q", m.Type, TypeUser)
	}
	if m.Scope != FactScopeGlobal {
		t.Errorf("Scope = %q, want %q", m.Scope, FactScopeGlobal)
	}
	if m.Body != "This is the memory body content." {
		t.Errorf("Body = %q, want %q", m.Body, "This is the memory body content.")
	}
	if m.Description != "A description of this memory." {
		t.Errorf("Description = %q, want %q", m.Description, "A description of this memory.")
	}
}

func TestMemory_Marshal(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	m := Memory{
		Title:       "Marshal Test",
		Type:        TypeReference,
		Scope:       FactScopeProject,
		Body:        "Test content for marshal verification.",
		Description: "marshal test description",
	}

	path, err := store.Save(m)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read the file back and verify YAML frontmatter + body structure
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", path, err)
	}
	content := string(data)

	// Must start with ---\n
	if !strings.HasPrefix(content, "---\n") {
		t.Errorf("File content should start with YAML frontmatter delimiter, got prefix: %q", content[:min(10, len(content))])
	}

	// Must contain the body
	if !strings.Contains(content, "Test content for marshal verification.") {
		t.Errorf("File content should contain the memory body")
	}

	// Must contain the title in frontmatter
	if !strings.Contains(content, "title: Marshal Test") {
		t.Errorf("File content should contain the title in frontmatter")
	}

	// Must contain type in frontmatter
	if !strings.Contains(content, "type: reference") {
		t.Errorf("File content should contain the type in frontmatter")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
