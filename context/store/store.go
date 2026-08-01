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

// Package store defines the StepStore interface for contextmgr persistence.
package store

import "github.com/inferglow/context"

// StepStore is the persistence interface for all compression-level data.
// Implementations exist for JSONL (default), SQLite, PostgreSQL, and Redis cache.
type StepStore interface {
	// --- L0 original content (.jsonl) ---

	// AppendStep persists a step record, upserting by step_id (idempotent).
	// Existing steps are updated in place; new steps are appended to L0.
	AppendStep(step contextmgr.StepRecord) error
	// GetStep retrieves a step record by ID from L0.
	GetStep(stepID int) (*contextmgr.StepRecord, error)
	// RangeSteps retrieves step records in [from, to] inclusive.
	RangeSteps(from, to int) ([]contextmgr.StepRecord, error)

	// --- refs (.refs.jsonl) ---

	// UpsertRef inserts or updates a ref record for the given step.
	UpsertRef(ref contextmgr.RefRecord) error
	// GetRef retrieves the ref record for a step.
	GetRef(stepID int) (*contextmgr.RefRecord, error)
	// AllActiveStepIDs returns all step IDs present in refs, ascending.
	AllActiveStepIDs() ([]int, error)
	// RemoveRef removes a step from refs (L4 discard).
	RemoveRef(stepID int) error

	// --- L1 simple compression (.l1.jsonl) ---

	// AppendL1 appends an L1 compression record.
	AppendL1(rec contextmgr.L1Record) error
	// GetL1 retrieves the L1 record for a step.
	GetL1(stepID int) (*contextmgr.L1Record, error)

	// --- L2 fact extraction (.l2.jsonl) ---

	// AppendL2 appends an L2 fact extraction record.
	AppendL2(rec contextmgr.L2Record) error
	// GetL2 retrieves the L2 record for a step.
	GetL2(stepID int) (*contextmgr.L2Record, error)
	// HotFacts returns L2 records meeting minimum ref_count and strength thresholds.
	HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error)

	// --- L3 behavior mask (.l3.jsonl) ---

	// AppendL3 appends an L3 mask record.
	AppendL3(rec contextmgr.L3Record) error
	// GetL3 retrieves the L3 record for a step.
	GetL3(stepID int) (*contextmgr.L3Record, error)

	// --- Long-term memory (longmem.jsonl / longterm_memories table) ---

	// UpsertLongMem inserts or updates a long-term memory record.
	UpsertLongMem(mem contextmgr.LongMemRecord) error
	// GetLongMem retrieves a long-term memory by ID.
	GetLongMem(memID string) (*contextmgr.LongMemRecord, error)
	// SearchLongMem searches long-term memories by query, category, and limit.
	SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error)
	// RemoveLongMem removes a long-term memory by ID.
	RemoveLongMem(memID string) error

	// --- lifecycle ---

	// AppendAudit appends an append-only audit log entry.
	AppendAudit(rec contextmgr.AuditRecord) error

	// Close releases any resources held by the store.
	Close() error
}
