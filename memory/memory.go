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

// Package memory implements a file-based auto-memory store compatible with
// Reasonix's memory system. Each memory is an independent .md file with
// YAML frontmatter, indexed by MEMORY.md for injection into the system prompt.
package memory

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Type classifies a memory, mirroring the Reasonix auto-memory taxonomy.
type Type string

const (
	TypeUser      Type = "user"      // who the user is: role, preferences, expertise
	TypeFeedback  Type = "feedback"  // guidance on how to work (with why + how-to-apply)
	TypeProject   Type = "project"   // ongoing work / goals / constraints not in the code
	TypeReference Type = "reference" // pointers to external resources (URLs, tickets)
)

// NormalizeType coerces an arbitrary string to a known Type, defaulting to
// TypeProject so a sloppy tool argument never blocks a save.
func NormalizeType(s string) Type {
	t := Type(strings.ToLower(strings.TrimSpace(s)))
	switch t {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		return t
	default:
		return TypeProject
	}
}

// FactScope controls where an auto-memory fact is active.
type FactScope string

const (
	FactScopeProject FactScope = "project"
	FactScopeGlobal  FactScope = "global"
)

// NormalizeFactScope defaults to the current project.
func NormalizeFactScope(s string) FactScope {
	if FactScope(strings.ToLower(strings.TrimSpace(s))) == FactScopeGlobal {
		return FactScopeGlobal
	}
	return FactScopeProject
}

// Memory is one stored fact.
type Memory struct {
	ID          string    `yaml:"id"`
	Revision    int       `yaml:"revision"`
	CreatedAt   time.Time `yaml:"created"`
	UpdatedAt   time.Time `yaml:"updated"`
	Name        string    `yaml:"-"` // kebab-case slug; also the file stem ({name}.md)
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Type        Type      `yaml:"type"`
	Scope       FactScope `yaml:"scope"`
	Body        string    `yaml:"-"` // the fact itself (Markdown), stored after frontmatter
}

// frontmatter is the YAML header written to each .md file.
type frontmatter struct {
	ID          string `yaml:"id"`
	Revision    int    `yaml:"revision"`
	Type        string `yaml:"type"`
	Scope       string `yaml:"scope"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Created     string `yaml:"created"`
	Updated     string `yaml:"updated"`
}

// loadMemory reads a .md file and parses its frontmatter + body.
func loadMemory(path string) (Memory, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Memory{}, false
	}

	content := string(data)
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		return Memory{}, false
	}

	created, _ := time.Parse(time.RFC3339, fm.Created)
	updated, _ := time.Parse(time.RFC3339, fm.Updated)

	// Derive name from file stem.
	name := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		name = path[idx+1:]
	}
	name = strings.TrimSuffix(name, ".md")

	return Memory{
		ID:          fm.ID,
		Revision:    fm.Revision,
		CreatedAt:   created,
		UpdatedAt:   updated,
		Name:        name,
		Title:       fm.Title,
		Description: fm.Description,
		Type:        NormalizeType(fm.Type),
		Scope:       NormalizeFactScope(fm.Scope),
		Body:        body,
	}, true
}

// splitFrontmatter separates YAML frontmatter from Markdown body.
func splitFrontmatter(content string) (frontmatter, string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return frontmatter{}, content, nil
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return frontmatter{}, content, fmt.Errorf("unterminated frontmatter")
	}
	yamlPart := content[4 : 4+end]
	body := content[4+end+5:] // skip "\n---\n"

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return frontmatter{}, "", err
	}
	return fm, strings.TrimLeft(body, "\n"), nil
}

// renderFile serializes a Memory to .md file content (frontmatter + body).
func renderFile(m Memory) string {
	fm := frontmatter{
		ID:          m.ID,
		Revision:    m.Revision,
		Type:        string(m.Type),
		Scope:       string(m.Scope),
		Title:       m.Title,
		Description: m.Description,
		Created:     m.CreatedAt.Format(time.RFC3339),
		Updated:     m.UpdatedAt.Format(time.RFC3339),
	}
	yamlBytes, _ := yaml.Marshal(fm)
	return "---\n" + string(yamlBytes) + "---\n\n" + m.Body + "\n"
}
