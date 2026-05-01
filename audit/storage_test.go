package audit

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMemoryStorage_RoundTrip(t *testing.T) {
	m := NewMemoryStorage()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		e := &AuditEntry{
			ID:        "id-" + string(rune('a'+i)),
			Timestamp: ts.Add(time.Duration(i) * time.Second),
			Source:    "agent",
			Action:    "decision",
		}
		if err := m.Save(e); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	loaded, err := m.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(loaded))
	}
	for i, e := range loaded {
		want := "id-" + string(rune('a'+i))
		if e.ID != want {
			t.Fatalf("entry %d: ID=%q want %q", i, e.ID, want)
		}
	}
}

func TestMemoryStorage_ConcurrentSave(t *testing.T) {
	m := NewMemoryStorage()
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(n int) {
			defer wg.Done()
			_ = m.Save(&AuditEntry{
				ID:        "id",
				Timestamp: time.Now(),
				Source:    "agent",
			})
		}(i)
	}
	wg.Wait()
	loaded, err := m.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != N {
		t.Fatalf("expected %d, got %d", N, len(loaded))
	}
}

func TestJSONFileStorage_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONFileStorage(dir)

	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		e := &AuditEntry{
			ID:        "id-" + string(rune('a'+i)),
			Timestamp: ts.Add(time.Duration(i) * time.Second),
			Source:    "agent",
			Action:    "decision",
			Input:     map[string]any{"k": i},
			Output:    "ok",
		}
		if err := s.Save(e); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Verify file exists with expected naming.
	expected := filepath.Join(dir, "audit-20260101.jsonl")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file %s: %v", expected, err)
	}

	loaded, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(loaded))
	}
	// LoadAll must return sorted by Timestamp.
	for i := 1; i < len(loaded); i++ {
		if loaded[i].Timestamp.Before(loaded[i-1].Timestamp) {
			t.Fatalf("LoadAll not sorted by Timestamp")
		}
	}
}

func TestJSONFileStorage_ConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONFileStorage(dir)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = s.Save(&AuditEntry{
				ID:        "id",
				Timestamp: time.Now(),
				Source:    "agent",
			})
		}()
	}
	wg.Wait()
	loaded, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != N {
		t.Fatalf("expected %d, got %d", N, len(loaded))
	}
}

func TestJSONFileStorage_CrossDayFileSplit(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONFileStorage(dir)

	// Day 1
	day1 := time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return day1 })
	if err := s.Save(&AuditEntry{ID: "a", Timestamp: day1, Source: "agent"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Day 2 — different file
	day2 := time.Date(2026, 1, 2, 0, 1, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return day2 })
	if err := s.Save(&AuditEntry{ID: "b", Timestamp: day2, Source: "agent"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Both files must exist.
	if _, err := os.Stat(filepath.Join(dir, "audit-20260101.jsonl")); err != nil {
		t.Fatalf("day1 file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "audit-20260102.jsonl")); err != nil {
		t.Fatalf("day2 file missing: %v", err)
	}

	loaded, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries across both files, got %d", len(loaded))
	}
	// Sorted by Timestamp: a before b.
	if loaded[0].ID != "a" || loaded[1].ID != "b" {
		t.Fatalf("LoadAll not sorted: %+v", loaded)
	}
}

func TestJSONFileStorage_LoadAllEmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewJSONFileStorage(dir)
	loaded, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on empty dir: %v", err)
	}
	if loaded != nil && len(loaded) != 0 {
		t.Fatalf("expected nil/empty, got %v", loaded)
	}
}

func TestJSONFileStorage_LoadAllMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	s := NewJSONFileStorage(dir)
	loaded, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on missing dir should be nil error: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty, got %d", len(loaded))
	}
}
