package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMD_FullSkill(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: go-test-fix
description: Run Go tests, diagnose failures, fix and re-run until green
runas: subagent
triggers: run tests, test failure
autouse: suggest
readonly: true
allowed_tools: bash, edit
---
# go-test-fix
1. Run ` + "`go test ./...`" + ` via bash
2. Diagnose failures
3. Fix and re-run until green
`
	path := filepath.Join(dir, "go-test-fix.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sk, err := ParseMD(path)
	if err != nil {
		t.Fatalf("ParseMD failed: %v", err)
	}

	if sk.Name != "go-test-fix" {
		t.Errorf("Name = %q, want %q", sk.Name, "go-test-fix")
	}
	if sk.Description != "Run Go tests, diagnose failures, fix and re-run until green" {
		t.Errorf("Description = %q, want %q", sk.Description, "Run Go tests, diagnose failures, fix and re-run until green")
	}
	if sk.RunAs != RunAsSubagent {
		t.Errorf("RunAs = %q, want %q", sk.RunAs, RunAsSubagent)
	}
	if sk.Scope != ScopeProject {
		t.Errorf("Scope = %q, want %q", sk.Scope, ScopeProject)
	}
	if sk.AutoUse != "suggest" {
		t.Errorf("AutoUse = %q, want %q", sk.AutoUse, "suggest")
	}
	if !sk.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
	if len(sk.Triggers) != 2 || sk.Triggers[0] != "run tests" || sk.Triggers[1] != "test failure" {
		t.Errorf("Triggers = %v, want [run tests test failure]", sk.Triggers)
	}
	if len(sk.AllowedTools) != 2 || sk.AllowedTools[0] != "bash" || sk.AllowedTools[1] != "edit" {
		t.Errorf("AllowedTools = %v, want [bash edit]", sk.AllowedTools)
	}
	if !strings.Contains(sk.Body, "go-test-fix") {
		t.Errorf("Body should contain skill name, got: %s", sk.Body)
	}
	if sk.Path != path {
		t.Errorf("Path = %q, want %q", sk.Path, path)
	}
	if sk.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if sk.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestParseMD_Minimal(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: hello-world
description: A simple hello world skill
---
Just say hello
`
	path := filepath.Join(dir, "hello-world.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sk, err := ParseMD(path)
	if err != nil {
		t.Fatalf("ParseMD failed: %v", err)
	}

	if sk.Name != "hello-world" {
		t.Errorf("Name = %q, want %q", sk.Name, "hello-world")
	}
	if sk.Description != "A simple hello world skill" {
		t.Errorf("Description = %q, want %q", sk.Description, "A simple hello world skill")
	}
	// Defaults
	if sk.RunAs != RunAsInline {
		t.Errorf("RunAs = %q, want %q", sk.RunAs, RunAsInline)
	}
	if sk.Scope != ScopeProject {
		t.Errorf("Scope = %q, want %q", sk.Scope, ScopeProject)
	}
	if sk.AutoUse != "off" {
		t.Errorf("AutoUse = %q, want %q", sk.AutoUse, "off")
	}
	if sk.ReadOnly {
		t.Error("ReadOnly = true, want false")
	}
	if len(sk.Triggers) != 0 {
		t.Errorf("Triggers = %v, want empty", sk.Triggers)
	}
	if len(sk.AllowedTools) != 0 {
		t.Errorf("AllowedTools = %v, want empty", sk.AllowedTools)
	}
	if strings.TrimSpace(sk.Body) != "Just say hello" {
		t.Errorf("Body = %q, want %q", sk.Body, "Just say hello")
	}
}

func TestParseMD_Invalid(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{"empty file", ""},
		{"no frontmatter", "just some text"},
		{"missing name", "---\ndescription: no name\n---\nbody"},
		{"missing description", "---\nname: no-desc\n---\nbody"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, "invalid.md")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := ParseMD(path)
			if err == nil {
				t.Error("ParseMD should return error for invalid content")
			}
		})
	}
}

func TestToMD(t *testing.T) {
	now := time.Now()
	sk := &Skill{
		Name:         "round-trip-test",
		Description:  "Test round-trip serialization",
		Body:         "1. Do something\n2. Do something else",
		Scope:        ScopeProject,
		RunAs:        RunAsSubagent,
		Triggers:     []string{"trigger1", "trigger2"},
		AutoUse:      "prefer",
		ReadOnly:     true,
		AllowedTools: []string{"bash", "grep", "edit"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	md := sk.ToMD()

	// Write to temp file and parse back
	dir := t.TempDir()
	path := filepath.Join(dir, "round-trip-test.md")
	if err := os.WriteFile(path, []byte(md), 0644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseMD(path)
	if err != nil {
		t.Fatalf("ParseMD round-trip failed: %v\nContent:\n%s", err, md)
	}

	if parsed.Name != sk.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, sk.Name)
	}
	if parsed.Description != sk.Description {
		t.Errorf("Description = %q, want %q", parsed.Description, sk.Description)
	}
	if parsed.RunAs != sk.RunAs {
		t.Errorf("RunAs = %q, want %q", parsed.RunAs, sk.RunAs)
	}
	if parsed.AutoUse != sk.AutoUse {
		t.Errorf("AutoUse = %q, want %q", parsed.AutoUse, sk.AutoUse)
	}
	if parsed.ReadOnly != sk.ReadOnly {
		t.Errorf("ReadOnly = %v, want %v", parsed.ReadOnly, sk.ReadOnly)
	}
	if strings.TrimSpace(parsed.Body) != strings.TrimSpace(sk.Body) {
		t.Errorf("Body = %q, want %q", parsed.Body, sk.Body)
	}
	if len(parsed.Triggers) != len(sk.Triggers) {
		t.Errorf("Triggers = %v, want %v", parsed.Triggers, sk.Triggers)
	} else {
		for i := range sk.Triggers {
			if parsed.Triggers[i] != sk.Triggers[i] {
				t.Errorf("Triggers[%d] = %q, want %q", i, parsed.Triggers[i], sk.Triggers[i])
			}
		}
	}
	if len(parsed.AllowedTools) != len(sk.AllowedTools) {
		t.Errorf("AllowedTools = %v, want %v", parsed.AllowedTools, sk.AllowedTools)
	} else {
		for i := range sk.AllowedTools {
			if parsed.AllowedTools[i] != sk.AllowedTools[i] {
				t.Errorf("AllowedTools[%d] = %q, want %q", i, parsed.AllowedTools[i], sk.AllowedTools[i])
			}
		}
	}
}
