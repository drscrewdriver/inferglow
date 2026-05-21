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

package audit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"encoding/json"
)

// MemoryStorage keeps entries in an in-memory slice. It is safe for
// concurrent use. LoadAll returns the entries in the order they were
// saved (insertion order, not sorted).
type MemoryStorage struct {
	mu      sync.RWMutex
	entries []*AuditEntry
}

// NewMemoryStorage returns an empty MemoryStorage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{}
}

// Save appends entry to the in-memory slice.
func (m *MemoryStorage) Save(entry *AuditEntry) error {
	if entry == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

// LoadAll returns a copy of the in-memory entries.
func (m *MemoryStorage) LoadAll() ([]*AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*AuditEntry, len(m.entries))
	copy(out, m.entries)
	return out, nil
}

// JSONFileStorage persists each entry as one JSON object per line in a
// daily-rotated file named audit-YYYYMMDD.jsonl inside StoragePath. It
// is safe for concurrent use.
type JSONFileStorage struct {
	mu    sync.Mutex
	path  string
	clock func() time.Time // injectable for testing
}

// NewJSONFileStorage creates a JSONFileStorage rooted at path. The
// directory is created on first Save if it does not exist.
func NewJSONFileStorage(path string) *JSONFileStorage {
	return &JSONFileStorage{
		path:  path,
		clock: time.Now,
	}
}

// SetClock replaces the internal clock used to compute the daily file
// name. Intended for tests that need to simulate cross-day rotation.
func (s *JSONFileStorage) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = now
}

// Save appends the JSON-serialized entry to audit-YYYYMMDD.jsonl in
// StoragePath. The YYYYMMDD segment is derived from entry.Timestamp so
// that audit files are organized by when the event occurred, not when
// it was persisted. The directory and file are created on demand.
func (s *JSONFileStorage) Save(entry *AuditEntry) error {
	if entry == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.path, 0o755); err != nil {
		return err
	}
	filename := s.fileFor(entry)
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	// O_APPEND is atomic for individual Write calls of size <= PIPE_BUF
	// on POSIX, and on Windows the per-write lock in the OS kernel still
	// guarantees append-only writes for files opened with FILE_APPEND_DATA.
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

// LoadAll reads every audit-*.jsonl file in StoragePath, parses each
// non-empty line as an AuditEntry, and returns the combined slice
// sorted by Timestamp ascending. If no files exist it returns (nil, nil).
func (s *JSONFileStorage) LoadAll() ([]*AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []*AuditEntry
	for _, fi := range entries {
		if fi.IsDir() {
			continue
		}
		name := fi.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		full := filepath.Join(s.path, name)
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			e := &AuditEntry{}
			if err := json.Unmarshal([]byte(line), e); err != nil {
				return nil, err
			}
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out, nil
}

// fileFor returns the audit-YYYYMMDD.jsonl path for the given entry's
// Timestamp. If entry.Timestamp is the zero value, the storage's clock
// (wall time by default) is used as a fallback so Append-time entries
// always land in a real file.
func (s *JSONFileStorage) fileFor(entry *AuditEntry) string {
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = s.clock()
	}
	return filepath.Join(s.path, "audit-"+ts.UTC().Format("20060102")+".jsonl")
}
