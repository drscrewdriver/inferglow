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
	"sort"
	"strings"
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
	ID      string        `json:"id"`
	Owner   string        `json:"owner,omitempty"`
	AgentID string        `json:"agent_id,omitempty"`
	Status  SessionStatus `json:"status"`
	Title   string        `json:"title,omitempty"`
	Group   string        `json:"group,omitempty"`
	Pinned  bool          `json:"pinned,omitempty"`
	// Workspace names the workspace this conversation belongs to (R8: the
	// sidebar groups sessions under their workspace; empty = unassigned and
	// the client groups by its active workspace).
	Workspace string    `json:"workspace,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionStore is an in-memory store for session records.
// The backing KV storage is provided by the generic storage.Map primitive,
// mirroring the TeamStore template (team_store.go).
type SessionStore struct {
	*storage.Map[string, *SessionRecord]
	metaMu sync.RWMutex // guards nextID + the id-assembly critical section
	nextID int
	// persistence: snapshot path; every mutation rewrites the file so a
	// restart restores the conversation list (data volumes are small — the
	// snapshot is a single JSON document, written synchronously).
	path string
}

// NewSessionStore creates an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		Map: storage.NewMap[string, *SessionRecord](),
	}
}

// SetPersistence directs the store to snapshot every mutation to path and
// returns whether a prior snapshot was restored.
func (ss *SessionStore) SetPersistence(path string) bool {
	ss.path = path
	return ss.Load(path)
}

// Load restores sessions from a snapshot file. Missing file = no-op (false).
func (ss *SessionStore) Load(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var recs []*SessionRecord
	if err := json.Unmarshal(data, &recs); err != nil {
		log.Printf("session snapshot %s corrupt (%v) — starting empty", path, err)
		return false
	}
	maxID := 0
	for _, rec := range recs {
		if rec == nil || rec.ID == "" {
			continue
		}
		ss.Map.Set(rec.ID, rec)
		if n := parseSessID(rec.ID); n > maxID {
			maxID = n
		}
	}
	ss.metaMu.Lock()
	ss.nextID = maxID
	ss.metaMu.Unlock()
	return true
}

func parseSessID(id string) int {
	var n int
	if _, err := fmt.Sscanf(id, "sess-%d", &n); err != nil {
		return 0
	}
	return n
}

// snapshot writes the full store to disk (called with mutations).
func (ss *SessionStore) snapshot() {
	if ss.path == "" {
		return
	}
	keys := ss.Map.Keys()
	recs := make([]*SessionRecord, 0, len(keys))
	for _, k := range keys {
		if rec, ok := ss.Map.Get(k); ok && rec != nil {
			recs = append(recs, rec)
		}
	}
	data, err := json.MarshalIndent(recs, "", " ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(ss.path), 0o755); err != nil {
		return
	}
	tmp := ss.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("session snapshot write: %v", err)
		return
	}
	_ = os.Rename(tmp, ss.path)
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
	ss.snapshot()
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

// SessionListFilter specifies optional search / grouping / pin filters for
// ListFiltered. A nil Pinned means "don't filter by pin state".
type SessionListFilter struct {
	Q      string
	Group  string
	Pinned *bool
}

// ListFiltered returns sessions matching the given filter, ordered by
// pinned-first then UpdatedAt descending. Q does a case-insensitive substring
// match across title/group/agent_id/owner. Group filters by exact group name
// (empty matches ungrouped sessions only when Group is exactly "").
func (ss *SessionStore) ListFiltered(f SessionListFilter) []*SessionRecord {
	q := strings.ToLower(strings.TrimSpace(f.Q))
	out := make([]*SessionRecord, 0, ss.Map.Len())
	for _, rec := range ss.Map.Values() {
		if f.Pinned != nil && rec.Pinned != *f.Pinned {
			continue
		}
		if f.Group != "" && rec.Group != f.Group {
			continue
		}
		if q != "" {
			hay := strings.ToLower(rec.Title + "|" + rec.Group + "|" + rec.AgentID + "|" + rec.Owner)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, rec)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// Delete removes a session by ID.
func (ss *SessionStore) Delete(id string) error {
	if _, ok := ss.Map.Get(id); !ok {
		return fmt.Errorf("session %q not found", id)
	}
	ss.Map.Delete(id)
	ss.snapshot()
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
	ss.snapshot()
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

// RenameWorkspace rewrites the workspace field of every session bound to
// oldName (R9 workspace rename follow-along) and snapshots. Returns the
// number of records updated.
func (ss *SessionStore) RenameWorkspace(oldName, newName string) int {
	n := 0
	for _, k := range ss.Keys() {
		rec, ok := ss.Map.Get(k)
		if !ok || rec == nil {
			continue
		}
		if rec.Workspace == oldName {
			rec.Workspace = newName
			rec.UpdatedAt = time.Now()
			ss.Set(k, rec)
			n++
		}
	}
	if n > 0 {
		ss.snapshot()
	}
	return n
}
