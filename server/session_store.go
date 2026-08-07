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

	"github.com/inferglow/storage"
)

// SessionStatus enumerates the lifecycle states of a C-4 session.
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusStopped  SessionStatus = "stopped"
	SessionStatusArchived SessionStatus = "archived"
)

// SessionRecord describes a single management session (spec C-4).
// Title/Group/Pinned are GUI metadata (patchable via PATCH /v1/sessions/{id});
// all new fields use omitempty so existing responses stay byte-compatible.
type SessionRecord struct {
	ID        string        `json:"id"`
	Owner     string        `json:"owner,omitempty"`
	AgentID   string        `json:"agent_id,omitempty"`
	Status    SessionStatus `json:"status"`
	Title     string        `json:"title,omitempty"`
	Group     string        `json:"group,omitempty"`
	Pinned    bool          `json:"pinned,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// SessionStore is an in-memory store for session records.
// The backing KV storage is provided by the generic storage.Map primitive,
// mirroring the TeamStore template (team_store.go).
type SessionStore struct {
	*storage.Map[string, *SessionRecord]
	metaMu sync.RWMutex // guards nextID + the id-assembly critical section
	nextID int
}

// NewSessionStore creates an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		Map: storage.NewMap[string, *SessionRecord](),
	}
}

// Create adds a new session and returns its ID.
func (ss *SessionStore) Create(rec SessionRecord) (string, error) {
	if rec.AgentID == "" {
		return "", fmt.Errorf("agent_id is required")
	}

	ss.metaMu.Lock()
	defer ss.metaMu.Unlock()

	ss.nextID++
	id := fmt.Sprintf("sess-%d", ss.nextID)
	rec.ID = id
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = rec.CreatedAt
	if rec.Status == "" {
		rec.Status = SessionStatusActive
	}

	ss.Map.Set(id, &rec)
	return id, nil
}

// Get returns a session by ID, or nil if not found.
func (ss *SessionStore) Get(id string) *SessionRecord {
	v, _ := ss.Map.Get(id)
	return v
}

// List returns all session records.
func (ss *SessionStore) List() []*SessionRecord {
	return ss.Map.Values()
}

// Delete removes a session by ID.
func (ss *SessionStore) Delete(id string) error {
	if _, ok := ss.Map.Get(id); !ok {
		return fmt.Errorf("session %q not found", id)
	}
	ss.Map.Delete(id)
	return nil
}

// UpdateStatus mutates the status of an existing session in place and bumps
// UpdatedAt. It reports whether the session existed.
func (ss *SessionStore) UpdateStatus(id string, status SessionStatus) bool {
	rec := ss.Get(id)
	if rec == nil {
		return false
	}
	rec.Status = status
	rec.UpdatedAt = time.Now()
	return true
}

// SessionPatch carries optional session metadata fields for PATCH updates.
// Pointer fields distinguish "not provided" (nil, leave untouched) from
// "explicitly cleared" (non-nil empty string / false).
type SessionPatch struct {
	Title  *string `json:"title,omitempty"`
	Group  *string `json:"group,omitempty"`
	Pinned *bool   `json:"pinned,omitempty"`
	Status *string `json:"status,omitempty"`
}

// UpdateMeta applies a session metadata patch in place and bumps UpdatedAt.
// It reports whether the session existed.
func (ss *SessionStore) UpdateMeta(id string, patch SessionPatch) bool {
	rec := ss.Get(id)
	if rec == nil {
		return false
	}
	if patch.Title != nil {
		rec.Title = *patch.Title
	}
	if patch.Group != nil {
		rec.Group = *patch.Group
	}
	if patch.Pinned != nil {
		rec.Pinned = *patch.Pinned
	}
	if patch.Status != nil {
		rec.Status = SessionStatus(*patch.Status)
	}
	rec.UpdatedAt = time.Now()
	return true
}
