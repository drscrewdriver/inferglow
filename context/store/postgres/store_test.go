//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

	contextmgr "github.com/inferglow/context"
)

func TestPostgresStore_New(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("Skipping: POSTGRES_TEST_DSN not set")
	}

	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer s.Close()

	if s.db == nil {
		t.Fatal("expected non-nil db handle")
	}
}

func TestPostgresStore_AppendGetStep(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("Skipping: POSTGRES_TEST_DSN not set")
	}

	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer s.Close()

	step := contextmgr.StepRecord{
		StepID: 1, Type: "reasoning", Role: "assistant",
		Content: "test postgres step", TokenCount: 10,
	}
	if err := s.AppendStep(step); err != nil {
		t.Fatalf("AppendStep returned error: %v", err)
	}

	got, err := s.GetStep(1)
	if err != nil {
		t.Fatalf("GetStep returned error: %v", err)
	}
	if got == nil {
		t.Fatal("GetStep returned nil")
	}
	if got.StepID != 1 || got.Content != "test postgres step" {
		t.Errorf("unexpected step: %+v", got)
	}
}
