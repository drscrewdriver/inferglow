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

package memory

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is the scoped auto-memory store: project and global directories of
// one-fact-per-file Markdown notes, each with a MEMORY.md index.
// All write operations follow the Write + Sync convention.
type Store struct {
	Dir       string // {data_dir}/projects/{slug}/memory
	GlobalDir string // {data_dir}/memory/global
}

// StoreFor resolves the auto-memory directory for a project working dir.
func StoreFor(dataDir, cwd string) Store {
	slug := workspaceSlug(cwd)
	return Store{
		Dir:       filepath.Join(dataDir, "projects", slug, "memory"),
		GlobalDir: filepath.Join(dataDir, "memory", "global"),
	}
}

// DirFor returns the directory for an explicit fact scope.
func (s Store) DirFor(scope FactScope) string {
	if s.GlobalDir != "" && NormalizeFactScope(string(scope)) == FactScopeGlobal {
		return s.GlobalDir
	}
	return s.Dir
}

// dirs returns the directories to read from: GlobalDir first, then Dir.
func (s Store) dirs() []string {
	if s.GlobalDir != "" && s.GlobalDir != s.Dir {
		return []string{s.GlobalDir, s.Dir}
	}
	return []string{s.Dir}
}

// Save writes (or overwrites) a memory file and refreshes its MEMORY.md
// index line. It is the single mutation entry point. Write + Sync.
func (s Store) Save(m Memory) (string, error) {
	dir := s.DirFor(m.Scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("memory: create dir: %w", err)
	}

	// Assign defaults.
	now := time.Now()
	if m.ID == "" {
		m.ID = generateID()
	}
	if m.Revision <= 0 {
		m.Revision = 1
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.Name == "" {
		m.Name = slugify(m.Description)
	}
	if m.Title == "" {
		m.Title = m.Name
	}

	path := filepath.Join(dir, m.Name+".md")

	// Check for existing file to preserve CreatedAt and bump Revision.
	if existing, ok := loadMemory(path); ok {
		m.CreatedAt = existing.CreatedAt
		m.Revision = existing.Revision + 1
	}

	// Write .md file (Write + Sync).
	content := renderFile(m)
	if err := writeFileSync(path, content); err != nil {
		return "", fmt.Errorf("memory: write file: %w", err)
	}

	// Rebuild index (Write + Sync).
	if err := s.rebuildIndex(); err != nil {
		return path, fmt.Errorf("memory: rebuild index: %w", err)
	}

	return path, nil
}

// List returns all active memories from both directories, sorted by name.
func (s Store) List() []Memory {
	var memories []Memory
	for _, dir := range s.dirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == indexFile {
				continue
			}
			if m, ok := loadMemory(filepath.Join(dir, e.Name())); ok {
				memories = append(memories, m)
			}
		}
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Name < memories[j].Name
	})
	return memories
}

// Load reads a single memory by name.
func (s Store) Load(name string) (Memory, bool) {
	for _, dir := range s.dirs() {
		path := filepath.Join(dir, name+".md")
		if m, ok := loadMemory(path); ok {
			return m, true
		}
	}
	return Memory{}, false
}

// Path returns the absolute file path for a memory name.
func (s Store) Path(name string) string {
	for _, dir := range s.dirs() {
		p := filepath.Join(dir, name+".md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(s.Dir, name+".md")
}

// Archive removes a memory from the active store and moves its file under
// .archive/ for traceability. Write + Sync on the moved file.
func (s Store) Archive(name string) error {
	for _, dir := range s.dirs() {
		src := filepath.Join(dir, name+".md")
		if _, err := os.Stat(src); err != nil {
			continue
		}
		archiveDir := filepath.Join(dir, ".archive")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(archiveDir, name+".md")
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := writeFileBytesSync(dst, data); err != nil {
			return err
		}
		if err := os.Remove(src); err != nil {
			return err
		}
		return s.rebuildIndex()
	}
	return fmt.Errorf("memory: %q not found", name)
}

// Index returns the MEMORY.md contents (the per-line index of saved memories).
func (s Store) Index() string {
	memories := s.List()
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Memory Index\n\n")
	for _, m := range memories {
		scope := ""
		if m.Scope == FactScopeGlobal {
			scope = " [global]"
		}
		b.WriteString(fmt.Sprintf("- [%s] %s — %s (%s)%s\n", m.Name, m.Title, m.Description, m.Type, scope))
	}
	return b.String()
}

// rebuildIndex regenerates MEMORY.md in both directories. Write + Sync.
func (s Store) rebuildIndex() error {
	index := s.Index()
	for _, dir := range s.dirs() {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, indexFile)
		if index == "" {
			os.Remove(path) // best-effort
			continue
		}
		if err := writeFileSync(path, index); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const indexFile = "MEMORY.md"

// writeFileSync writes content to path with Write + Sync.
func writeFileSync(path, content string) error {
	return writeFileBytesSync(path, []byte(content))
}

// writeFileBytesSync writes bytes to path with Write + Sync.
func writeFileBytesSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// generateID creates a random memory ID.
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("mem-%x", b)
}

// slugify converts a description to a kebab-case slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Remove non-alphanumeric except hyphens.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 60 {
		result = result[:60]
	}
	return strings.Trim(result, "-")
}

// workspaceSlug creates a filesystem-safe slug from a working directory.
func workspaceSlug(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	slug := strings.ReplaceAll(abs, string(filepath.Separator), "-")
	slug = strings.TrimPrefix(slug, "-")
	if len(slug) > 80 {
		slug = slug[:80]
	}
	return slug
}
