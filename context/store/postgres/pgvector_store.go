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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/inferglow/context/retrieval"
)

// VectorStore implements retrieval.VectorStoreBackend using pgvector.
// It reuses the *sql.DB connection pool from the existing postgres.Store.
type VectorStore struct {
	db *sql.DB
}

// NewVectorStore creates a pgvector-backed VectorStoreBackend.
// It requires the pgvector extension to be installed in PostgreSQL.
// The provided *sql.DB should be the same connection pool used by the main Store.
func NewVectorStore(ctx context.Context, db *sql.DB) (*VectorStore, error) {
	vs := &VectorStore{db: db}
	if err := vs.initTable(ctx); err != nil {
		return nil, fmt.Errorf("pgvector store: init: %w", err)
	}
	return vs, nil
}

func (v *VectorStore) initTable(ctx context.Context) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE TABLE IF NOT EXISTS vectors (
			id TEXT PRIMARY KEY,
			embedding vector(1536),
			session_id TEXT DEFAULT '',
			step_id INTEGER DEFAULT 0,
			text TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS vectors_embedding_idx ON vectors USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`,
	}
	for _, s := range stmts {
		if _, err := v.db.ExecContext(ctx, s); err != nil {
			// IVFFlat index may fail if table is empty; that's OK — it will be
			// created after data is inserted via Reindex().
			if !strings.Contains(err.Error(), "ivfflat") {
				return fmt.Errorf("exec %q: %w", s[:40], err)
			}
		}
	}
	return nil
}

// Add inserts a single vector.
func (v *VectorStore) Add(ctx context.Context, id string, vec []float32, meta retrieval.VectorMeta) error {
	embeddingStr := formatVector(vec)
	_, err := v.db.ExecContext(ctx,
		`INSERT INTO vectors (id, embedding, session_id, step_id, text)
		 VALUES ($1, $2::vector, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET embedding=EXCLUDED.embedding, session_id=EXCLUDED.session_id, step_id=EXCLUDED.step_id, text=EXCLUDED.text`,
		id, embeddingStr, meta.SessionID, meta.StepID, meta.Text,
	)
	return err
}

// Search finds the top-k most similar vectors using cosine distance.
func (v *VectorStore) Search(ctx context.Context, query []float32, limit int) ([]retrieval.SearchResult, error) {
	queryStr := formatVector(query)
	rows, err := v.db.QueryContext(ctx,
		`SELECT step_id, text, 1 - (embedding <=> $1::vector) AS score
		 FROM vectors
		 ORDER BY embedding <=> $1::vector
		 LIMIT $2`,
		queryStr, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pgvector search: %w", err)
	}
	defer rows.Close()

	var results []retrieval.SearchResult
	for rows.Next() {
		var r retrieval.SearchResult
		if err := rows.Scan(&r.StepID, &r.Text, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// Delete removes a vector by ID.
func (v *VectorStore) Delete(ctx context.Context, id string) error {
	_, err := v.db.ExecContext(ctx, `DELETE FROM vectors WHERE id=$1`, id)
	return err
}

// BatchAdd inserts multiple vectors in a single transaction.
func (v *VectorStore) BatchAdd(ctx context.Context, items []retrieval.VectorItem) error {
	tx, err := v.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO vectors (id, embedding, session_id, step_id, text)
		 VALUES ($1, $2::vector, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET embedding=EXCLUDED.embedding`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		embeddingStr := formatVector(item.Vec)
		if _, err := stmt.ExecContext(ctx, item.ID, embeddingStr, item.Meta.SessionID, item.Meta.StepID, item.Meta.Text); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Reindex rebuilds the IVFFlat index. Call after bulk inserts.
func (v *VectorStore) Reindex(ctx context.Context) error {
	_, err := v.db.ExecContext(ctx, `REINDEX INDEX vectors_embedding_idx`)
	return err
}

// formatVector converts a []float32 to pgvector string format: "[0.1,0.2,...]"
func formatVector(vec []float32) string {
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
