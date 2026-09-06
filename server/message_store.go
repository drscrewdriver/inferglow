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

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MessageRole enumerates the roles stored in the session message log.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
	// MessageRoleTrace carries one run's observability summary (spans
	// timeline + usage, JSON in Content). Infrastructure records: excluded
	// from chat history listing (ListBefore), served by the session trace
	// endpoint (ListTraces) so the 轨迹/上下文 panels survive restarts and
	// session restores.
	MessageRoleTrace MessageRole = "trace"
)

// MessageRecord is a single entry in a session's message log. It backs the
// paginated history endpoint GET /v1/sessions/{id}/messages for the GUI.
type MessageRecord struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"session_id"`
	Role       MessageRole `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolName   string      `json:"tool_name,omitempty"`
	ToolStatus string      `json:"tool_status,omitempty"` // "run" | "ok" | "error"
	CreatedAt  time.Time   `json:"created_at"`
}

// MessageStore is an in-memory append-only per-session message log.
// Each session owns an ordered slice ordered by CreatedAt (oldest first);
// ListBefore walks it backwards to serve newest-first pagination.
type MessageStore struct {
	mu   sync.RWMutex
	seq  int
	msgs map[string][]*MessageRecord // sessionID -> ordered log
	// persistence: snapshot path; Append rewrites the file so chat history,
	// tool records and run traces survive restarts (R8).
	path string
}

// NewMessageStore creates an empty MessageStore.
func NewMessageStore() *MessageStore {
	return &MessageStore{msgs: make(map[string][]*MessageRecord)}
}

// SetPersistence directs the store to snapshot every Append to path.
func (ms *MessageStore) SetPersistence(path string) {
	ms.path = path
	ms.Load(path)
}

// Load restores messages (incl. trace records) from a snapshot. Missing or
// corrupt file = silent no-op (fresh deployment).
func (ms *MessageStore) Load(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snap struct {
		Seq  int                         `json:"seq"`
		Msgs map[string][]*MessageRecord `json:"msgs"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("message snapshot %s corrupt (%v) — starting empty", path, err)
		return
	}
	ms.mu.Lock()
	ms.seq = snap.Seq
	ms.msgs = snap.Msgs
	if ms.msgs == nil {
		ms.msgs = make(map[string][]*MessageRecord)
	}
	ms.mu.Unlock()
}

// snapshot writes the full log atomically (called with mu held by mutators).
func (ms *MessageStore) snapshot() {
	if ms.path == "" {
		return
	}
	snap := struct {
		Seq  int                         `json:"seq"`
		Msgs map[string][]*MessageRecord `json:"msgs"`
	}{Seq: ms.seq, Msgs: ms.msgs}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(ms.path), 0o755); err != nil {
		return
	}
	tmp := ms.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("message snapshot write: %v", err)
		return
	}
	_ = os.Rename(tmp, ms.path)
}

// Append adds a message to the session's log and returns the stored record.
func (ms *MessageStore) Append(sessionID string, rec MessageRecord) (*MessageRecord, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.seq++
	rec.ID = fmt.Sprintf("msg-%d", ms.seq)
	rec.SessionID = sessionID
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	stored := rec
	ms.msgs[sessionID] = append(ms.msgs[sessionID], &stored)
	ms.snapshot()
	return &stored, nil
}

// ListBefore returns up to limit messages older than before (newest first).
// A zero before returns the newest messages. The bool reports whether older
// messages remain (has_more), so clients can keep paginating.
func (ms *MessageStore) ListBefore(sessionID string, before time.Time, limit int) ([]*MessageRecord, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	log := ms.msgs[sessionID]
	if len(log) == 0 || limit <= 0 {
		return nil, false
	}

	// Find the first index with CreatedAt < before (log is oldest-first).
	start := len(log)
	for i := len(log) - 1; i >= 0; i-- {
		if before.IsZero() || log[i].CreatedAt.Before(before) {
			start = i
			break
		}
	}
	if start == len(log) {
		return nil, false
	}

	// Collect newest-first up to limit. Trace records are infrastructure,
	// never chat history.
	out := make([]*MessageRecord, 0, min(limit, len(log)-start))
	for i := start; i >= 0 && len(out) < limit; i-- {
		if log[i].Role == MessageRoleTrace {
			continue
		}
		out = append(out, log[i])
	}
	hasMore := start-len(out)+1 > 0
	return out, hasMore
}

// ListTraces returns the session's run-summary trace records, newest first.
func (ms *MessageStore) ListTraces(sessionID string, limit int) []MessageRecord {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	log := ms.msgs[sessionID]
	out := make([]MessageRecord, 0, min(limit, len(log)))
	for i := len(log) - 1; i >= 0 && len(out) < limit; i-- {
		if log[i].Role == MessageRoleTrace {
			out = append(out, *log[i])
		}
	}
	return out
}

// Count returns the number of messages stored for a session (test helper).
func (ms *MessageStore) Count(sessionID string) int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.msgs[sessionID])
}
