//go:build sqlite_fts5

package sqlite

import (
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3" // register the sqlite3 driver

	contextmgr "github.com/inferglow/context"
)

// TestTransientRoundTrip verifies the transient columns survive a full
// append → read → reload cycle in SQLite.
func TestTransientRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	step := contextmgr.StepRecord{
		StepID: 42, Type: "tool", Content: "tool output",
		Transient: true, TransientScope: "tool_call", TransientRound: 5,
	}
	if err := s.AppendStep(step); err != nil {
		t.Fatal(err)
	}

	// Read back in-memory.
	got, err := s.GetStep(42)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Transient || got.TransientScope != "tool_call" || got.TransientRound != 5 {
		t.Errorf("transient fields lost on read; got %+v", got)
	}

	// RangeSteps must also surface the transient fields.
	steps, err := s.RangeSteps(42, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || !steps[0].Transient || steps[0].TransientScope != "tool_call" {
		t.Errorf("RangeSteps lost transient fields; got %+v", steps)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reload from disk — transient fields must persist.
	s2, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got2, err := s2.GetStep(42)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.Transient || got2.TransientScope != "tool_call" || got2.TransientRound != 5 {
		t.Errorf("transient fields lost after reload; got %+v", got2)
	}
}

// TestAppendStepUpsert verifies re-appending the same step_id updates the row
// (including clearing transient from true to false) rather than duplicating.
func TestAppendStepUpsert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Content: "v1", Transient: true, TransientScope: "scratch", TransientRound: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Content: "v2", Transient: false}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetStep(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "v2" || got.Transient {
		t.Errorf("expected updated non-transient row; got %+v", got)
	}
}

// TestMigrateStepsOnExistingDB verifies the migration helper adds the transient
// columns to a pre-existing steps table (idempotent, no error).
func TestMigrateStepsOnExistingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	// Create a store with the legacy schema (no transient columns) by using a
	// raw legacy CREATE TABLE, then point Store at it and run initTables.
	s, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Drop the modern table and recreate the legacy one to simulate a pre-B1 DB.
	if _, err := s.db.Exec(`DROP TABLE steps`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TABLE steps (
		step_id INTEGER PRIMARY KEY,
		type TEXT, role TEXT, content TEXT, token_count INTEGER,
		tool_name TEXT, key_params TEXT, created_at INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO steps (step_id, content) VALUES (1, 'legacy')`); err != nil {
		t.Fatal(err)
	}

	// Re-run initTables → migrateSteps must add the transient columns.
	if err := s.initTables(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Reading the legacy row must now work with the transient columns present.
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Content: "new", Transient: true, TransientScope: "tool_call", TransientRound: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStep(1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Transient || got.TransientScope != "tool_call" || got.TransientRound != 2 {
		t.Errorf("migrated schema failed to persist transient; got %+v", got)
	}
}

// TestMigrateStepsIdempotent verifies calling initTables twice is safe.
func TestMigrateStepsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.initTables(); err != nil {
		t.Fatalf("second initTables failed: %v", err)
	}
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 2, Content: "x", Transient: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStep(2)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Transient {
		t.Errorf("expected transient persisted; got %+v", got)
	}
}