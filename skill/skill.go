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

// Package skill implements the Skill Store for procedural memory management.
// It provides a Reasonix-compatible skill system that supports progressive
// disclosure and on-demand loading of executable playbooks.
package skill

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Scope represents the skill scope.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
	ScopeBuiltin Scope = "builtin"
)

// RunAs represents how the skill should be executed.
type RunAs string

const (
	RunAsInline    RunAs = "inline"
	RunAsSubagent  RunAs = "subagent"
)

// Skill is a reusable playbook that can be invoked by the agent.
type Skill struct {
	Name         string    `json:"name"`                    // kebab-case identifier
	Description  string    `json:"description"`             // one-line summary (for index)
	Body         string    `json:"body"`                    // full playbook (loaded on demand)
	Scope        Scope     `json:"scope"`                   // project | global | builtin
	RunAs        RunAs     `json:"runas"`                   // inline | subagent
	Path         string    `json:"path"`                    // .md file path
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	AllowedTools []string  `json:"allowed_tools,omitempty"` // tool whitelist (subagent only)
	ReadOnly     bool      `json:"read_only,omitempty"`     // force read-only
	Triggers     []string  `json:"triggers,omitempty"`      // trigger keywords
	AutoUse      string    `json:"autouse,omitempty"`       // off | suggest | prefer | require
}

// ParseMD parses a skill from a .md file with YAML frontmatter.
// Format:
//   ---
//   name: go-test-fix
//   description: Run Go tests, diagnose failures, fix and re-run until green
//   runas: inline
//   triggers: run tests, test failure
//   autouse: suggest
//   ---
//   # go-test-fix
//   1. Run `go test ./...` via bash
//   ...
func ParseMD(path string) (*Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("skill: open %s: %w", path, err)
	}
	defer f.Close()

	sk := &Skill{
		Path:    path,
		Scope:   ScopeProject,
		RunAs:   RunAsInline,
		AutoUse: "off",
	}

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	bodyLines := []string{}

	for scanner.Scan() {
		line := scanner.Text()

		// Detect frontmatter boundaries
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			// End of frontmatter
			inFrontmatter = false
			continue
		}

		if inFrontmatter {
			// Parse frontmatter key: value
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			switch key {
			case "name":
				sk.Name = val
			case "description":
				sk.Description = val
			case "runas":
				sk.RunAs = RunAs(val)
			case "triggers":
				// Comma-separated list
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						sk.Triggers = append(sk.Triggers, t)
					}
				}
			case "autouse":
				sk.AutoUse = val
			case "readonly":
				sk.ReadOnly = val == "true"
			case "allowed_tools":
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						sk.AllowedTools = append(sk.AllowedTools, t)
					}
				}
			}
		} else {
			// Body content
			bodyLines = append(bodyLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("skill: scan %s: %w", path, err)
	}

	sk.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	// Validate required fields
	if sk.Name == "" {
		return nil, fmt.Errorf("skill: %s: missing 'name' in frontmatter", path)
	}
	if sk.Description == "" {
		return nil, fmt.Errorf("skill: %s: missing 'description' in frontmatter", path)
	}

	// Set timestamps
	stat, err := os.Stat(path)
	if err == nil {
		sk.CreatedAt = stat.ModTime()
		sk.UpdatedAt = stat.ModTime()
	} else {
		sk.CreatedAt = time.Now()
		sk.UpdatedAt = time.Now()
	}

	return sk, nil
}

// ToMD renders the skill as a .md file content with frontmatter.
func (s *Skill) ToMD() string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", s.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", s.Description))
	sb.WriteString(fmt.Sprintf("runas: %s\n", s.RunAs))

	if len(s.Triggers) > 0 {
		sb.WriteString(fmt.Sprintf("triggers: %s\n", strings.Join(s.Triggers, ", ")))
	}
	if s.AutoUse != "" && s.AutoUse != "off" {
		sb.WriteString(fmt.Sprintf("autouse: %s\n", s.AutoUse))
	}
	if s.ReadOnly {
		sb.WriteString("readonly: true\n")
	}
	if len(s.AllowedTools) > 0 {
		sb.WriteString(fmt.Sprintf("allowed_tools: %s\n", strings.Join(s.AllowedTools, ", ")))
	}
	sb.WriteString("---\n\n")

	sb.WriteString(s.Body)
	sb.WriteString("\n")

	return sb.String()
}
