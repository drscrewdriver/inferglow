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

package actions

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Spawn statuses for SubagentRegistry records.
const (
	SpawnStatusRunning = "running"
	SpawnStatusDone    = "done"
	SpawnStatusError   = "error"
)

// SpawnRecord is one spawn_agent invocation observed by the registry.
// The record is created (running) before the sub-agent loop starts and
// finalized (done/error) when it returns, so a polling observer sees live
// rows while the (synchronous) sub-agent run is in flight.
type SpawnRecord struct {
	ID            string `json:"id"`
	ParentSession string `json:"parent_session,omitempty"`
	Task          string `json:"task"`
	Status        string `json:"status"` // running | done | error
	SystemPrompt  string `json:"system_prompt,omitempty"`
	StartedAt     int64  `json:"started_at"` // unix ms
	EndedAt       int64  `json:"ended_at,omitempty"`
	Result        string `json:"result,omitempty"`
	Error         string `json:"error,omitempty"`
}

// SubagentRegistry tracks spawn_agent invocations. It mirrors the TaskStore
// pattern (R8): one shared instance created by the host before agent
// assembly, handed both to the spawn_agent action (which records) and to
// the observation surface (which reads).
type SubagentRegistry struct {
	mu     sync.RWMutex
	spawns map[string]*SpawnRecord
	seq    int
}

// NewSubagentRegistry creates an empty registry.
func NewSubagentRegistry() *SubagentRegistry {
	return &SubagentRegistry{spawns: make(map[string]*SpawnRecord)}
}

// Start records a newly spawned sub-agent run and returns its record (the
// caller finalizes it via Finish using the returned ID).
func (r *SubagentRegistry) Start(parentSession, task, systemPrompt string) *SpawnRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	rec := &SpawnRecord{
		ID:            fmt.Sprintf("spawn-%04d", r.seq),
		ParentSession: parentSession,
		Task:          task,
		Status:        SpawnStatusRunning,
		SystemPrompt:  systemPrompt,
		StartedAt:     time.Now().UnixMilli(),
	}
	r.spawns[rec.ID] = rec
	return rec
}

// Finish finalizes a spawn record. ok=false marks the record as errored with
// errMsg; an empty result is preserved as-is either way.
func (r *SubagentRegistry) Finish(id string, ok bool, result, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, okRec := r.spawns[id]
	if !okRec {
		return
	}
	rec.EndedAt = time.Now().UnixMilli()
	if ok {
		rec.Status = SpawnStatusDone
		rec.Result = result
	} else {
		rec.Status = SpawnStatusError
		rec.Error = errMsg
	}
}

// List returns spawn records, newest first. When parentSession is non-empty
// only records attributed to that session are returned; the empty string
// returns everything.
func (r *SubagentRegistry) List(parentSession string) []*SpawnRecord {
	r.mu.RLock()
	out := make([]*SpawnRecord, 0, len(r.spawns))
	for _, rec := range r.spawns {
		if parentSession != "" && rec.ParentSession != parentSession {
			continue
		}
		out = append(out, rec)
	}
	r.mu.RUnlock()
	// Newest first by StartedAt; monotonic IDs break same-ms ties so the
	// order never depends on map iteration.
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt > out[j].StartedAt
	})
	return out
}
