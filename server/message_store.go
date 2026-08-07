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
	"fmt"
	"sync"
	"time"
)

// MessageRole enumerates the roles stored in the session message log.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
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
}

// NewMessageStore creates an empty MessageStore.
func NewMessageStore() *MessageStore {
	return &MessageStore{msgs: make(map[string][]*MessageRecord)}
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

	// Collect newest-first up to limit.
	out := make([]*MessageRecord, 0, min(limit, len(log)-start))
	for i := start; i >= 0 && len(out) < limit; i-- {
		out = append(out, log[i])
	}
	hasMore := start-len(out)+1 > 0
	return out, hasMore
}

// Count returns the number of messages stored for a session (test helper).
func (ms *MessageStore) Count(sessionID string) int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.msgs[sessionID])
}
