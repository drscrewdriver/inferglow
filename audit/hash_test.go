package audit

import (
	"testing"
	"time"
)

func TestComputeHash_EmptyEntry(t *testing.T) {
	e := &AuditEntry{}
	// Should not panic; deterministic hex string of length 64.
	h := ComputeHash(e)
	if len(h) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex, got %d: %q", len(h), h)
	}
	// Recompute: must be stable.
	if h2 := ComputeHash(e); h2 != h {
		t.Fatalf("ComputeHash not stable: %q vs %q", h, h2)
	}
}

func TestComputeHash_FullEntry(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	e := &AuditEntry{
		PrevHash:  "abc",
		Timestamp: ts,
		Source:    "agent",
		Action:    "decision",
		Input:     map[string]any{"k": "v", "n": 42},
		Output:    []any{1, 2, 3},
	}
	h := ComputeHash(e)
	if len(h) != 64 {
		t.Fatalf("expected 64-char hash, got %d", len(h))
	}
	// Mutating input order in a map must not change hash (canonical JSON).
	e2 := &AuditEntry{
		PrevHash:  "abc",
		Timestamp: ts,
		Source:    "agent",
		Action:    "decision",
		Input:     map[string]any{"n": 42, "k": "v"},
		Output:    []any{1, 2, 3},
	}
	if ComputeHash(e2) != h {
		t.Fatalf("ComputeHash not canonical: map key order changed the hash")
	}
}

func TestComputeHash_NilInput(t *testing.T) {
	ts := time.Now().UTC()
	e := &AuditEntry{
		PrevHash:  "",
		Timestamp: ts,
		Source:    "model",
		Action:    "request",
		Input:     nil,
		Output:    nil,
	}
	h := ComputeHash(e)
	if h == "" {
		t.Fatal("expected non-empty hash")
	}
	// nil vs empty should both serialize consistently and not panic.
	e2 := &AuditEntry{
		PrevHash:  "",
		Timestamp: ts,
		Source:    "model",
		Action:    "request",
	}
	if ComputeHash(e2) != h {
		t.Fatalf("nil Input/Output should produce same hash as zero-value")
	}
}

func TestComputeHash_ChainedDependency(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := &AuditEntry{
		Timestamp: ts,
		Source:    "agent",
		Action:    "decision",
		Input:     "hello",
		Output:    "world",
	}
	h1 := ComputeHash(base)

	// Different PrevHash must yield different Hash.
	withPrev := &AuditEntry{
		PrevHash:  h1,
		Timestamp: ts,
		Source:    "agent",
		Action:    "decision",
		Input:     "hello",
		Output:    "world",
	}
	h2 := ComputeHash(withPrev)
	if h1 == h2 {
		t.Fatal("PrevHash must contribute to ComputeHash output")
	}
}
