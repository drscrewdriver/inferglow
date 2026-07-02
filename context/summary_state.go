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

package contextmgr

import (
	"encoding/json"
	"os"
	"sync"
)

// SummaryState tracks compaction state across daemon restarts.
// Persisted as JSON alongside the session data.
type SummaryState struct {
	LastCompactAt       int64  `json:"last_compact_at"`
	CompactCount        int    `json:"compact_count"`
	ConsecutiveCompacts int    `json:"consecutive_compacts"`
	CompactStuck        bool   `json:"compact_stuck"`
	LastArchivePath     string `json:"last_archive_path"`
}

// SummaryStateStore manages persistence of SummaryState.
// All writes follow the Write + Sync convention.
type SummaryStateStore struct {
	mu   sync.Mutex
	path string
}

// NewSummaryStateStore creates a state store at the given path.
func NewSummaryStateStore(path string) *SummaryStateStore {
	return &SummaryStateStore{path: path}
}

// Load reads the state from disk. Returns zero state if file doesn't exist.
func (s *SummaryStateStore) Load() SummaryState {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return SummaryState{}
	}
	var state SummaryState
	if err := json.Unmarshal(data, &state); err != nil {
		return SummaryState{}
	}
	return state
}

// Save writes the state to disk with Write + Sync.
func (s *SummaryStateStore) Save(state SummaryState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
