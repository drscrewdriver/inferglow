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

package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store manages skill packages on disk.
type Store struct {
	Dir       string // {data_dir}/projects/{slug}/skills
	GlobalDir string // {data_dir}/skills/global
	mu        sync.RWMutex
	cache     map[string]*Skill // name → skill (lazy loaded)
}

// NewStore creates a skill store rooted at the given directories.
func NewStore(dir, globalDir string) *Store {
	return &Store{
		Dir:       dir,
		GlobalDir: globalDir,
		cache:     make(map[string]*Skill),
	}
}

// List returns all active skills from project and global directories.
func (s *Store) List() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var skills []Skill

	// Scan project dir
	if entries, err := os.ReadDir(s.Dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				if sk, err := ParseMD(filepath.Join(s.Dir, e.Name())); err == nil {
					sk.Scope = ScopeProject
					skills = append(skills, *sk)
				}
			}
		}
	}

	// Scan global dir
	if s.GlobalDir != "" {
		if entries, err := os.ReadDir(s.GlobalDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					if sk, err := ParseMD(filepath.Join(s.GlobalDir, e.Name())); err == nil {
						sk.Scope = ScopeGlobal
						skills = append(skills, *sk)
					}
				}
			}
		}
	}

	return skills
}

// Read loads a skill by name, including its full body.
func (s *Store) Read(name string) (Skill, bool) {
	s.mu.RLock()
	if sk, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return *sk, true
	}
	s.mu.RUnlock()

	// Search in project dir
	path := filepath.Join(s.Dir, name+".md")
	if sk, err := ParseMD(path); err == nil {
		sk.Scope = ScopeProject
		s.mu.Lock()
		s.cache[name] = sk
		s.mu.Unlock()
		return *sk, true
	}

	// Search in global dir
	if s.GlobalDir != "" {
		path = filepath.Join(s.GlobalDir, name+".md")
		if sk, err := ParseMD(path); err == nil {
			sk.Scope = ScopeGlobal
			s.mu.Lock()
			s.cache[name] = sk
			s.mu.Unlock()
			return *sk, true
		}
	}

	return Skill{}, false
}

// Save writes a skill to disk as a .md file.
func (s *Store) Save(sk Skill) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Determine target directory
	dir := s.Dir
	if sk.Scope == ScopeGlobal {
		dir = s.GlobalDir
	}

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("skill: create dir %s: %w", dir, err)
	}

	// Write .md file
	path := filepath.Join(dir, sk.Name+".md")
	content := sk.ToMD()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("skill: write %s: %w", path, err)
	}

	// Update cache
	s.cache[sk.Name] = &sk

	return nil
}

// Delete removes a skill from disk.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Try project dir first
	path := filepath.Join(s.Dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("skill: remove %s: %w", path, err)
		}
		delete(s.cache, name)
		return nil
	}

	// Try global dir
	if s.GlobalDir != "" {
		path = filepath.Join(s.GlobalDir, name+".md")
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("skill: remove %s: %w", path, err)
			}
			delete(s.cache, name)
			return nil
		}
	}

	return fmt.Errorf("skill: %s not found", name)
}

// IndexBlock renders a skill index block for system prompt injection.
// Output is limited to ≤4000 chars to avoid prompt bloat.
func (s *Store) IndexBlock() string {
	skills := s.List()
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Skills — playbooks you can invoke\n\n")

	totalLen := 0
	maxLen := 4000

	for _, sk := range skills {
		line := fmt.Sprintf("- %s — %s\n", sk.Name, sk.Description)
		if totalLen+len(line) > maxLen {
			sb.WriteString("... (truncated)\n")
			break
		}
		sb.WriteString(line)
		totalLen += len(line)
	}

	sb.WriteString("\nCall run_skill({ name: \"<skill-name>\", arguments: \"<task>\" })\n")

	return sb.String()
}

// ProjectInstructions scans the project root for meta-instruction files
// (INFERGLOW.md, AGENTS.md, REASONIX.md) and returns the first found content.
// Returns empty string if none found.
func (s *Store) ProjectInstructions(root string) string {
	// Priority order: INFERGLOW.md > AGENTS.md > REASONIX.md
	candidates := []string{
		"INFERGLOW.md",
		"AGENTS.md",
		"REASONIX.md",
	}

	for _, name := range candidates {
		path := filepath.Join(root, name)
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}

	return ""
}
