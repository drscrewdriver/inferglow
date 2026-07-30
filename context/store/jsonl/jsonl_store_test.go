package jsonl

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contextmgr "github.com/inferglow/context"
)

// TestAppendStepIdempotentUpsert verifies B1: appending the same step_id twice
// must not create duplicate on-disk rows, and the latest value (including the
// transient fields) must win.
func TestAppendStepIdempotentUpsert(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "sess")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// First append (new step) → fast append path.
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Type: "reasoning", Content: "v1"}); err != nil {
		t.Fatal(err)
	}
	// Second append, same step_id → idempotent upsert, updates transient fields.
	if err := s.AppendStep(contextmgr.StepRecord{
		StepID: 1, Type: "reasoning", Content: "v2",
		Transient: true, TransientScope: "tool_call", TransientRound: 3,
	}); err != nil {
		t.Fatal(err)
	}

	// GetStep reflects the latest value.
	got, err := s.GetStep(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "v2" || !got.Transient || got.TransientScope != "tool_call" || got.TransientRound != 3 {
		t.Errorf("expected latest step with transient set; got %+v", got)
	}

	// On-disk file must contain exactly one row for step 1.
	lines := fileLines(t, filepath.Join(dir, "sess.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 line, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"content":"v2"`) {
		t.Errorf("expected updated content on disk; got %s", lines[0])
	}
	if !strings.Contains(lines[0], `"transient":true`) {
		t.Errorf("expected transient on disk; got %s", lines[0])
	}
}

// TestAppendStepKeepsDistinctSteps verifies distinct steps still each get one row.
func TestAppendStepKeepsDistinctSteps(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "sess")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 2, Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendStep(contextmgr.StepRecord{StepID: 1, Content: "a2"}); err != nil {
		t.Fatal(err)
	}

	lines := fileLines(t, filepath.Join(dir, "sess.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (one per distinct step), got %d", len(lines))
	}
}

// TestAppendStepPersistsTransientAcrossReload verifies the transient fields
// survive a store reload (they must be written to disk, not just memory).
func TestAppendStepPersistsTransientAcrossReload(t *testing.T) {
	dir := t.TempDir()
	if err := func() error {
		s, err := New(dir, "sess")
		if err != nil {
			return err
		}
		defer s.Close()
		return s.AppendStep(contextmgr.StepRecord{
			StepID: 7, Content: "x", Transient: true, TransientScope: "scratch", TransientRound: 2,
		})
	}(); err != nil {
		t.Fatal(err)
	}

	s2, err := New(dir, "sess")
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetStep(7)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Transient || got.TransientScope != "scratch" || got.TransientRound != 2 {
		t.Errorf("transient fields lost after reload; got %+v", got)
	}
}

func fileLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}