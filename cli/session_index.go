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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionIndexEntry is a single entry in the session index.
type SessionIndexEntry struct {
	UUID      string `json:"uuid"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// SessionIndex manages an append-only index of sessions.
type SessionIndex struct {
	path    string
	entries []SessionIndexEntry
}

// NewSessionIndex creates or loads a session index from the given data directory.
func NewSessionIndex(dataDir string) (*SessionIndex, error) {
	dir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session index: create dir: %w", err)
	}

	idx := &SessionIndex{
		path: filepath.Join(dir, "index.jsonl"),
	}

	// Load existing entries.
	if err := idx.load(); err != nil {
		return nil, err
	}

	return idx, nil
}

// load reads the index file.
func (idx *SessionIndex) load() error {
	data, err := os.ReadFile(idx.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var entry SessionIndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		idx.entries = append(idx.entries, entry)
	}
	return nil
}

// Add appends a new session entry to the index.
func (idx *SessionIndex) Add(uuid, title string) error {
	now := time.Now().Unix()
	entry := SessionIndexEntry{
		UUID:      uuid,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(idx.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	idx.entries = append(idx.entries, entry)
	return nil
}

// List returns all session entries.
func (idx *SessionIndex) List() []SessionIndexEntry {
	return idx.entries
}

// splitLines splits text into lines, handling both \n and \r\n.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
