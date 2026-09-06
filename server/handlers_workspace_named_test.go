// Copyright 2026 InferGlow Authors

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNamedWorkspaceSelection — seeded workspaces are selectable via
// ?workspace=; the empty param keeps the default chain; unknown names fail
// loudly instead of silently showing another workspace.
func TestNamedWorkspaceSelection(t *testing.T) {
	dirA := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "marker-a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirB, "marker-b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	srv.SeedWorkspaces([]WorkspaceSeed{
		{Name: "ws-a", Root: dirA},
		{Name: "ws-b", Root: dirB},
		{Name: "bad", Root: filepath.Join(dirA, "does-not-exist")}, // skipped, logged
	})

	// Registry lists the two good seeds (bad skipped) with absolute roots.
	if w := doJSON(t, srv, "GET", "/v1/workspaces", ""); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"name":"ws-a"`) ||
		!strings.Contains(w.Body.String(), `"name":"ws-b"`) ||
		strings.Contains(w.Body.String(), `"name":"bad"`) {
		t.Fatalf("workspaces list unexpected: %s", w.Body.String())
	}

	// Named selection hits each root.
	if w := doJSON(t, srv, "GET", "/v1/fs/tree?workspace=ws-a", ""); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "marker-a.txt") || strings.Contains(w.Body.String(), "marker-b.txt") {
		t.Fatalf("ws-a tree: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(t, srv, "GET", "/v1/fs/tree?workspace=ws-b", ""); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "marker-b.txt") {
		t.Fatalf("ws-b tree: %d %s", w.Code, w.Body.String())
	}

	// Empty param keeps the default chain → first registered (name order: ws-a).
	if w := doJSON(t, srv, "GET", "/v1/fs/tree", ""); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "marker-a.txt") {
		t.Fatalf("default tree: %d %s", w.Code, w.Body.String())
	}

	// Unknown name fails loudly (400), never falls back silently.
	if w := doJSON(t, srv, "GET", "/v1/fs/tree?workspace=nope", ""); w.Code != 400 {
		t.Fatalf("unknown workspace: want 400, got %d (%s)", w.Code, w.Body.String())
	}

	// Traversal inside a named workspace still refused.
	if w := doJSON(t, srv, "GET", "/v1/fs/tree?workspace=ws-a&path=../../", ""); w.Code == 200 {
		t.Fatalf("traversal escaped: %s", w.Body.String())
	}

	// Exec's named resolution shares the same gate (unknown → 400).
	if w := doJSON(t, srv, "GET", "/v1/produced-files?workspace=nope", ""); w.Code != 400 {
		t.Fatalf("produced-files unknown workspace: want 400, got %d", w.Code)
	}
}


// TestContextModesEndpoint — the context chip's config list source.
func TestContextModesEndpoint(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	w := doJSON(t, srv, "GET", "/v1/context/modes", "")
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, mode := range []string{"passthrough", "three_zone", "summary", "hybrid", "assembly"} {
		if !strings.Contains(body, `"`+mode+`"`) {
			t.Fatalf("mode %q missing: %s", mode, body)
		}
	}
}
