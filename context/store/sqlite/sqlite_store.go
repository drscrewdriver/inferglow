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

// Package sqlite implements the StepStore interface using SQLite.
//
// This provides full-text search via FTS5 and is suitable for single-user
// or lightweight deployments. For production multi-session use, see postgres.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/inferglow/context"
)

// Store implements store.StepStore using SQLite.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// New creates a new SQLite store.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: open: %w", err)
	}

	s := &Store{db: db}
	if err := s.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite store: init: %w", err)
	}

	return s, nil
}

func (s *Store) initTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS steps (
			step_id INTEGER PRIMARY KEY,
			type TEXT, role TEXT, content TEXT, token_count INTEGER,
			tool_name TEXT, key_params TEXT, created_at INTEGER,
			transient INTEGER DEFAULT 0,
			transient_scope TEXT DEFAULT '',
			transient_round INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS refs (
			step_id INTEGER PRIMARY KEY,
			level INTEGER, ref_count INTEGER, last_ref_at_step INTEGER,
			strength REAL, task_group_id INTEGER, task_boundary INTEGER,
			semantic_hold INTEGER, pending_l4 INTEGER, related_files TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS l1_records (
			step_id INTEGER PRIMARY KEY,
			content TEXT, token_count INTEGER, compressed_at_step INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS l2_records (
			step_id INTEGER PRIMARY KEY,
			facts TEXT, token_count INTEGER, compressed_at_step INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS l3_records (
			step_id INTEGER PRIMARY KEY,
			mask TEXT, token_count INTEGER, compressed_at_step INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS longmem (
			mem_id TEXT PRIMARY KEY,
			facts TEXT, source_steps TEXT, source_sessions TEXT,
			category TEXT, created_at_step INTEGER,
			last_validated_step INTEGER, confidence REAL
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS steps_fts USING fts5(content, step_id UNINDEXED)`,
	}

	for _, t := range tables {
		if _, err := s.db.Exec(t); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	s.migrateSteps()
	return nil
}

// migrateSteps ensures the transient columns exist on the steps table for
// existing databases. Migration is idempotent (columns are probed via
// PRAGMA table_info first); failures are logged but never block startup.
func (s *Store) migrateSteps() {
	cols, err := s.db.Query(`PRAGMA table_info(steps)`)
	if err != nil {
		log.Printf("sqlite store: probe steps columns: %v", err)
		return
	}
	defer cols.Close()

	existing := make(map[string]bool)
	for cols.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt *string
		if err := cols.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			log.Printf("sqlite store: read steps columns: %v", err)
			return
		}
		existing[name] = true
	}

	add := []struct{ name, ddl string }{
		{"transient", `ALTER TABLE steps ADD COLUMN transient INTEGER DEFAULT 0`},
		{"transient_scope", `ALTER TABLE steps ADD COLUMN transient_scope TEXT DEFAULT ''`},
		{"transient_round", `ALTER TABLE steps ADD COLUMN transient_round INTEGER DEFAULT 0`},
	}
	for _, c := range add {
		if existing[c.name] {
			continue
		}
		if _, err := s.db.Exec(c.ddl); err != nil {
			log.Printf("sqlite store: migrate steps: %v", err)
		}
	}
}

func (s *Store) AppendStep(step contextmgr.StepRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO steps (step_id, type, role, content, token_count, tool_name, key_params, created_at, transient, transient_scope, transient_round)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.StepID, step.Type, step.Role, step.Content, step.TokenCount, step.ToolName, step.KeyParams, step.CreatedAt,
		step.Transient, step.TransientScope, step.TransientRound,
	)
	return err
}

func (s *Store) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rec contextmgr.StepRecord
	err := s.db.QueryRow(
		`SELECT step_id, type, role, content, token_count, tool_name, key_params, created_at, transient, transient_scope, transient_round FROM steps WHERE step_id = ?`,
		stepID,
	).Scan(&rec.StepID, &rec.Type, &rec.Role, &rec.Content, &rec.TokenCount, &rec.ToolName, &rec.KeyParams, &rec.CreatedAt, &rec.Transient, &rec.TransientScope, &rec.TransientRound)
	if err != nil {
		return nil, fmt.Errorf("step %d not found: %w", stepID, err)
	}
	return &rec, nil
}

func (s *Store) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT step_id, type, role, content, token_count, tool_name, key_params, created_at, transient, transient_scope, transient_round
		 FROM steps WHERE step_id BETWEEN ? AND ? ORDER BY step_id`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []contextmgr.StepRecord
	for rows.Next() {
		var rec contextmgr.StepRecord
		if err := rows.Scan(&rec.StepID, &rec.Type, &rec.Role, &rec.Content, &rec.TokenCount, &rec.ToolName, &rec.KeyParams, &rec.CreatedAt, &rec.Transient, &rec.TransientScope, &rec.TransientRound); err != nil {
			return nil, err
		}
		result = append(result, rec)
	}
	return result, nil
}

func (s *Store) UpsertRef(ref contextmgr.RefRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastRef sql.NullInt64
	if ref.LastRefAtStep != nil {
		lastRef = sql.NullInt64{Int64: int64(*ref.LastRefAtStep), Valid: true}
	}
	relatedFiles, _ := json.Marshal(ref.RelatedFiles)

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO refs (step_id, level, ref_count, last_ref_at_step, strength, task_group_id, task_boundary, semantic_hold, pending_l4, related_files)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ref.StepID, ref.Level, ref.RefCount, lastRef, ref.Strength, ref.TaskGroupID, ref.TaskBoundary, ref.SemanticHold, ref.PendingL4, string(relatedFiles),
	)
	return err
}

func (s *Store) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rec contextmgr.RefRecord
	var lastRef sql.NullInt64
	var relatedFiles string
	err := s.db.QueryRow(
		`SELECT step_id, level, ref_count, last_ref_at_step, strength, task_group_id, task_boundary, semantic_hold, pending_l4, related_files
		 FROM refs WHERE step_id = ?`,
		stepID,
	).Scan(&rec.StepID, &rec.Level, &rec.RefCount, &lastRef, &rec.Strength, &rec.TaskGroupID, &rec.TaskBoundary, &rec.SemanticHold, &rec.PendingL4, &relatedFiles)
	if err != nil {
		return nil, fmt.Errorf("ref %d not found: %w", stepID, err)
	}
	if lastRef.Valid {
		v := int(lastRef.Int64)
		rec.LastRefAtStep = &v
	}
	_ = json.Unmarshal([]byte(relatedFiles), &rec.RelatedFiles)
	return &rec, nil
}

func (s *Store) AllActiveStepIDs() ([]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT step_id FROM refs ORDER BY step_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) RemoveRef(stepID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM refs WHERE step_id = ?`, stepID)
	return err
}

func (s *Store) AppendL1(rec contextmgr.L1Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO l1_records (step_id, content, token_count, compressed_at_step) VALUES (?, ?, ?, ?)`,
		rec.StepID, rec.Content, rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL1(stepID int) (*contextmgr.L1Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L1Record
	err := s.db.QueryRow(`SELECT step_id, content, token_count, compressed_at_step FROM l1_records WHERE step_id = ?`, stepID).
		Scan(&rec.StepID, &rec.Content, &rec.TokenCount, &rec.CompressedAtStep)
	if err != nil {
		return nil, fmt.Errorf("l1 %d not found: %w", stepID, err)
	}
	return &rec, nil
}

func (s *Store) AppendL2(rec contextmgr.L2Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, _ := json.Marshal(rec.Facts)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO l2_records (step_id, facts, token_count, compressed_at_step) VALUES (?, ?, ?, ?)`,
		rec.StepID, string(facts), rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL2(stepID int) (*contextmgr.L2Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L2Record
	var factsStr string
	err := s.db.QueryRow(`SELECT step_id, facts, token_count, compressed_at_step FROM l2_records WHERE step_id = ?`, stepID).
		Scan(&rec.StepID, &factsStr, &rec.TokenCount, &rec.CompressedAtStep)
	if err != nil {
		return nil, fmt.Errorf("l2 %d not found: %w", stepID, err)
	}
	_ = json.Unmarshal([]byte(factsStr), &rec.Facts)
	return &rec, nil
}

func (s *Store) HotFacts(minRefCount int, minStrength float64) ([]contextmgr.L2Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT l.step_id, l.facts, l.token_count, l.compressed_at_step
		 FROM l2_records l JOIN refs r ON l.step_id = r.step_id
		 WHERE r.ref_count >= ? AND r.strength >= ?`,
		minRefCount, minStrength,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []contextmgr.L2Record
	for rows.Next() {
		var rec contextmgr.L2Record
		var factsStr string
		if err := rows.Scan(&rec.StepID, &factsStr, &rec.TokenCount, &rec.CompressedAtStep); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(factsStr), &rec.Facts)
		result = append(result, rec)
	}
	return result, nil
}

func (s *Store) AppendL3(rec contextmgr.L3Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO l3_records (step_id, mask, token_count, compressed_at_step) VALUES (?, ?, ?, ?)`,
		rec.StepID, rec.Mask, rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL3(stepID int) (*contextmgr.L3Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L3Record
	err := s.db.QueryRow(`SELECT step_id, mask, token_count, compressed_at_step FROM l3_records WHERE step_id = ?`, stepID).
		Scan(&rec.StepID, &rec.Mask, &rec.TokenCount, &rec.CompressedAtStep)
	if err != nil {
		return nil, fmt.Errorf("l3 %d not found: %w", stepID, err)
	}
	return &rec, nil
}

func (s *Store) UpsertLongMem(mem contextmgr.LongMemRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, _ := json.Marshal(mem.Facts)
	steps, _ := json.Marshal(mem.SourceSteps)
	sessions, _ := json.Marshal(mem.SourceSessions)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO longmem (mem_id, facts, source_steps, source_sessions, category, created_at_step, last_validated_step, confidence)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		mem.MemID, string(facts), string(steps), string(sessions), mem.Category, mem.CreatedAtStep, mem.LastValidatedStep, mem.Confidence,
	)
	return err
}

func (s *Store) GetLongMem(memID string) (*contextmgr.LongMemRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.LongMemRecord
	var factsStr, stepsStr, sessionsStr string
	err := s.db.QueryRow(
		`SELECT mem_id, facts, source_steps, source_sessions, category, created_at_step, last_validated_step, confidence
		 FROM longmem WHERE mem_id = ?`, memID,
	).Scan(&rec.MemID, &factsStr, &stepsStr, &sessionsStr, &rec.Category, &rec.CreatedAtStep, &rec.LastValidatedStep, &rec.Confidence)
	if err != nil {
		return nil, fmt.Errorf("longmem %s not found: %w", memID, err)
	}
	_ = json.Unmarshal([]byte(factsStr), &rec.Facts)
	_ = json.Unmarshal([]byte(stepsStr), &rec.SourceSteps)
	_ = json.Unmarshal([]byte(sessionsStr), &rec.SourceSessions)
	return &rec, nil
}

func (s *Store) SearchLongMem(query string, category string, limit int) ([]contextmgr.LongMemRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := "SELECT mem_id, facts, source_steps, source_sessions, category, created_at_step, last_validated_step, confidence FROM longmem WHERE confidence >= 0.5"
	var args []interface{}
	if category != "" {
		q += " AND category = ?"
		args = append(args, category)
	}
	q += " ORDER BY confidence DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []contextmgr.LongMemRecord
	queryLower := strings.ToLower(query)
	for rows.Next() {
		var rec contextmgr.LongMemRecord
		var factsStr, stepsStr, sessionsStr string
		if err := rows.Scan(&rec.MemID, &factsStr, &stepsStr, &sessionsStr, &rec.Category, &rec.CreatedAtStep, &rec.LastValidatedStep, &rec.Confidence); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(factsStr), &rec.Facts)
		_ = json.Unmarshal([]byte(stepsStr), &rec.SourceSteps)
		_ = json.Unmarshal([]byte(sessionsStr), &rec.SourceSessions)

		// Simple keyword filter
		if query != "" {
			matched := false
			for _, f := range rec.Facts {
				if strings.Contains(strings.ToLower(f), queryLower) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, rec)
	}
	return result, nil
}

func (s *Store) RemoveLongMem(memID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM longmem WHERE mem_id = ?`, memID)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
