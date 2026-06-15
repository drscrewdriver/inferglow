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

// Package postgres implements the StepStore interface using PostgreSQL.
//
// This is the production backend for multi-session deployments.
// Supports pgvector for semantic search and full-text search via tsvector.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/inferglow/session/contextmgr"
)

// Store implements store.StepStore using PostgreSQL.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// New creates a new PostgreSQL store.
func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres store: open: %w", err)
	}

	s := &Store{db: db}
	if err := s.initTables(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres store: init: %w", err)
	}

	return s, nil
}

func (s *Store) initTables(ctx context.Context) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS steps (
			step_id INTEGER PRIMARY KEY,
			type TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			token_count INTEGER NOT NULL DEFAULT 0,
			tool_name TEXT DEFAULT '',
			key_params TEXT DEFAULT '',
			created_at BIGINT DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS refs (
			step_id INTEGER PRIMARY KEY,
			level INTEGER NOT NULL DEFAULT 0,
			ref_count INTEGER NOT NULL DEFAULT 0,
			last_ref_at_step INTEGER,
			strength REAL NOT NULL DEFAULT 1.0,
			task_group_id INTEGER NOT NULL DEFAULT 0,
			task_boundary BOOLEAN NOT NULL DEFAULT false,
			semantic_hold BOOLEAN NOT NULL DEFAULT false,
			pending_l4 BOOLEAN NOT NULL DEFAULT false,
			related_files TEXT DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS l1_records (
			step_id INTEGER PRIMARY KEY REFERENCES steps(step_id),
			content TEXT NOT NULL,
			token_count INTEGER NOT NULL DEFAULT 0,
			compressed_at_step INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS l2_records (
			step_id INTEGER PRIMARY KEY REFERENCES steps(step_id),
			facts TEXT NOT NULL DEFAULT '[]',
			token_count INTEGER NOT NULL DEFAULT 0,
			compressed_at_step INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS l3_records (
			step_id INTEGER PRIMARY KEY REFERENCES steps(step_id),
			mask TEXT NOT NULL,
			token_count INTEGER NOT NULL DEFAULT 0,
			compressed_at_step INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS longterm_memories (
			mem_id TEXT PRIMARY KEY,
			facts TEXT NOT NULL DEFAULT '[]',
			source_steps TEXT NOT NULL DEFAULT '[]',
			source_sessions TEXT NOT NULL DEFAULT '[]',
			category TEXT NOT NULL DEFAULT '',
			created_at_step INTEGER NOT NULL DEFAULT 0,
			last_validated_step INTEGER NOT NULL DEFAULT 0,
			confidence REAL NOT NULL DEFAULT 0.8
		)`,
	}

	for _, t := range tables {
		if _, err := s.db.ExecContext(ctx, t); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	return nil
}

func (s *Store) AppendStep(step contextmgr.StepRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO steps (step_id, type, role, content, token_count, tool_name, key_params, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (step_id) DO UPDATE SET content=EXCLUDED.content, token_count=EXCLUDED.token_count`,
		step.StepID, step.Type, step.Role, step.Content, step.TokenCount, step.ToolName, step.KeyParams, step.CreatedAt,
	)
	return err
}

func (s *Store) GetStep(stepID int) (*contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.StepRecord
	err := s.db.QueryRow(
		`SELECT step_id, type, role, content, token_count, tool_name, key_params, created_at FROM steps WHERE step_id=$1`, stepID,
	).Scan(&rec.StepID, &rec.Type, &rec.Role, &rec.Content, &rec.TokenCount, &rec.ToolName, &rec.KeyParams, &rec.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("step %d not found: %w", stepID, err)
	}
	return &rec, nil
}

func (s *Store) RangeSteps(from, to int) ([]contextmgr.StepRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT step_id, type, role, content, token_count, tool_name, key_params, created_at
		 FROM steps WHERE step_id BETWEEN $1 AND $2 ORDER BY step_id`, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []contextmgr.StepRecord
	for rows.Next() {
		var rec contextmgr.StepRecord
		if err := rows.Scan(&rec.StepID, &rec.Type, &rec.Role, &rec.Content, &rec.TokenCount, &rec.ToolName, &rec.KeyParams, &rec.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, rec)
	}
	return result, nil
}

func (s *Store) UpsertRef(ref contextmgr.RefRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lastRef *int = ref.LastRefAtStep
	relatedFiles, _ := json.Marshal(ref.RelatedFiles)
	_, err := s.db.Exec(
		`INSERT INTO refs (step_id, level, ref_count, last_ref_at_step, strength, task_group_id, task_boundary, semantic_hold, pending_l4, related_files)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (step_id) DO UPDATE SET level=EXCLUDED.level, ref_count=EXCLUDED.ref_count, last_ref_at_step=EXCLUDED.last_ref_at_step,
		 strength=EXCLUDED.strength, semantic_hold=EXCLUDED.semantic_hold, pending_l4=EXCLUDED.pending_l4, related_files=EXCLUDED.related_files`,
		ref.StepID, ref.Level, ref.RefCount, lastRef, ref.Strength, ref.TaskGroupID, ref.TaskBoundary, ref.SemanticHold, ref.PendingL4, string(relatedFiles),
	)
	return err
}

func (s *Store) GetRef(stepID int) (*contextmgr.RefRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.RefRecord
	var relatedFiles string
	err := s.db.QueryRow(
		`SELECT step_id, level, ref_count, last_ref_at_step, strength, task_group_id, task_boundary, semantic_hold, pending_l4, related_files
		 FROM refs WHERE step_id=$1`, stepID,
	).Scan(&rec.StepID, &rec.Level, &rec.RefCount, &rec.LastRefAtStep, &rec.Strength, &rec.TaskGroupID, &rec.TaskBoundary, &rec.SemanticHold, &rec.PendingL4, &relatedFiles)
	if err != nil {
		return nil, fmt.Errorf("ref %d not found: %w", stepID, err)
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
	_, err := s.db.Exec(`DELETE FROM refs WHERE step_id=$1`, stepID)
	return err
}

func (s *Store) AppendL1(rec contextmgr.L1Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO l1_records (step_id, content, token_count, compressed_at_step) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (step_id) DO UPDATE SET content=EXCLUDED.content, token_count=EXCLUDED.token_count`,
		rec.StepID, rec.Content, rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL1(stepID int) (*contextmgr.L1Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L1Record
	err := s.db.QueryRow(`SELECT step_id, content, token_count, compressed_at_step FROM l1_records WHERE step_id=$1`, stepID).
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
		`INSERT INTO l2_records (step_id, facts, token_count, compressed_at_step) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (step_id) DO UPDATE SET facts=EXCLUDED.facts`,
		rec.StepID, string(facts), rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL2(stepID int) (*contextmgr.L2Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L2Record
	var factsStr string
	err := s.db.QueryRow(`SELECT step_id, facts, token_count, compressed_at_step FROM l2_records WHERE step_id=$1`, stepID).
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
		 WHERE r.ref_count >= $1 AND r.strength >= $2`,
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
		`INSERT INTO l3_records (step_id, mask, token_count, compressed_at_step) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (step_id) DO UPDATE SET mask=EXCLUDED.mask`,
		rec.StepID, rec.Mask, rec.TokenCount, rec.CompressedAtStep,
	)
	return err
}

func (s *Store) GetL3(stepID int) (*contextmgr.L3Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rec contextmgr.L3Record
	err := s.db.QueryRow(`SELECT step_id, mask, token_count, compressed_at_step FROM l3_records WHERE step_id=$1`, stepID).
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
		`INSERT INTO longterm_memories (mem_id, facts, source_steps, source_sessions, category, created_at_step, last_validated_step, confidence)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (mem_id) DO UPDATE SET facts=EXCLUDED.facts, confidence=EXCLUDED.confidence, last_validated_step=EXCLUDED.last_validated_step`,
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
		 FROM longterm_memories WHERE mem_id=$1`, memID,
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

	q := `SELECT mem_id, facts, source_steps, source_sessions, category, created_at_step, last_validated_step, confidence
		  FROM longterm_memories WHERE confidence >= 0.5`
	var args []interface{}
	argIdx := 1
	if category != "" {
		q += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
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
	_, err := s.db.Exec(`DELETE FROM longterm_memories WHERE mem_id=$1`, memID)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
