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

// Package recordstore provides a unified execution record storage layer
// for the inferglow orchestrator. It is the Go equivalent of Agently's
// RecordStore: a single source of truth for action results, decisions,
// model responses, flow steps, checkpoints, snapshots, and events.
//
// The RecordStore interface supports scope-based isolation (agent, session,
// execution dimensions) and both in-memory and persistent backends.
package recordstore

import (
	"time"
)

// Scope identifies the dimensional context of a record.
type Scope struct {
	// AgentID is the agent that produced the record.
	AgentID string `json:"agent_id,omitempty"`

	// SessionID is the conversation session.
	SessionID string `json:"session_id,omitempty"`

	// ExecutionID is the specific execution run.
	ExecutionID string `json:"execution_id,omitempty"`
}

// Record is a single execution record stored in the RecordStore.
type Record struct {
	// ID is the unique record identifier.
	ID string `json:"id"`

	// Scope is the dimensional context.
	Scope Scope `json:"scope"`

	// Kind categorizes the record (e.g. "action_result", "decision",
	// "model_response", "flow_step").
	Kind string `json:"kind"`

	// Timestamp records when the record was created.
	Timestamp time.Time `json:"timestamp"`

	// Data holds the record payload.
	Data any `json:"data,omitempty"`

	// Metadata carries implementation-specific key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Checkpoint captures a resumable execution state.
type Checkpoint struct {
	// ExecutionID identifies the execution.
	ExecutionID string `json:"execution_id"`

	// StepIndex is the index of the last completed step.
	StepIndex int `json:"step_index"`

	// State holds the serialized execution state.
	State any `json:"state,omitempty"`

	// CreatedAt records when the checkpoint was saved.
	CreatedAt time.Time `json:"created_at"`
}

// Snapshot captures a full execution snapshot for persistence and restart.
type Snapshot struct {
	// ExecutionID identifies the execution.
	ExecutionID string `json:"execution_id"`

	// Data holds the serialized snapshot.
	Data any `json:"data,omitempty"`

	// CreatedAt records when the snapshot was saved.
	CreatedAt time.Time `json:"created_at"`

	// Metadata carries additional snapshot metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Event is a timestamped occurrence within an execution.
type Event struct {
	// ID is the unique event identifier.
	ID string `json:"id"`

	// ExecutionID identifies the execution.
	ExecutionID string `json:"execution_id"`

	// Kind categorizes the event (e.g. "action_started", "llm_called").
	Kind string `json:"kind"`

	// Timestamp records when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Data holds the event payload.
	Data any `json:"data,omitempty"`

	// Metadata carries additional event metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EventFilter specifies criteria for querying events.
type EventFilter struct {
	// ExecutionID filters by execution. Empty matches all.
	ExecutionID string `json:"execution_id,omitempty"`

	// Kind filters by event kind. Empty matches all.
	Kind string `json:"kind,omitempty"`

	// Since filters events after this time. Zero means no lower bound.
	Since time.Time `json:"since,omitempty"`

	// Until filters events before this time. Zero means no upper bound.
	Until time.Time `json:"until,omitempty"`

	// Limit caps the number of returned events. Zero means no limit.
	Limit int `json:"limit,omitempty"`
}

// RecordStore is the unified storage interface for execution records,
// checkpoints, snapshots, and events.
type RecordStore interface {
	// AppendRecord stores a new record.
	AppendRecord(rec Record) error

	// GetRecord retrieves a record by ID. Returns nil, nil if not found.
	GetRecord(id string) (*Record, error)

	// QueryRecords returns records matching the given scope and kind.
	// An empty scope matches all scopes; an empty kind matches all kinds.
	QueryRecords(scope Scope, kind string) ([]Record, error)

	// SaveCheckpoint persists a checkpoint for the given execution.
	SaveCheckpoint(executionID string, cp *Checkpoint) error

	// LoadCheckpoint retrieves the latest checkpoint for an execution.
	// Returns nil, nil if no checkpoint exists.
	LoadCheckpoint(executionID string) (*Checkpoint, error)

	// SaveSnapshot persists a full snapshot for the given execution.
	SaveSnapshot(executionID string, snap *Snapshot) error

	// LoadSnapshot retrieves the latest snapshot for an execution.
	// Returns nil, nil if no snapshot exists.
	LoadSnapshot(executionID string) (*Snapshot, error)

	// AppendEvent stores a new event.
	AppendEvent(evt Event) error

	// QueryEvents returns events matching the given filter.
	QueryEvents(filter EventFilter) ([]Event, error)
}
