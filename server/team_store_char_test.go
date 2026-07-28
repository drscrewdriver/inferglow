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

// Characterization tests locking down TeamStore's observable behavior against
// the ORIGINAL map-based implementation. These must continue to pass unchanged
// after the storage abstraction refactor, proving old/new equivalence.

package server

import "testing"

func teamCfg(name string) TeamConfig {
	return TeamConfig{
		Name:    name,
		Members: []TeamMemberConfig{{AgentID: "a1", Role: "lead"}, {AgentID: "a2", Role: "worker"}},
	}
}

func TestTeamStoreCharCreateIDsIncrement(t *testing.T) {
	ts := NewTeamStore()
	id1, err := ts.Create(teamCfg("teamA"))
	if err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	if id1 != "team-1" {
		t.Fatalf("id1 = %q, want team-1", id1)
	}
	id2, err := ts.Create(teamCfg("teamB"))
	if err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if id2 != "team-2" {
		t.Fatalf("id2 = %q, want team-2", id2)
	}
}

func TestTeamStoreCharCreateDefaults(t *testing.T) {
	ts := NewTeamStore()
	id, err := ts.Create(teamCfg("teamA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := ts.Get(id)
	if got == nil {
		t.Fatal("expected created team to be retrievable")
	}
	if got.MaxRounds != 3 {
		t.Fatalf("MaxRounds default = %d, want 3", got.MaxRounds)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be non-zero")
	}
	if got.ID != id {
		t.Fatalf("stored ID = %q, want %q", got.ID, id)
	}
}

func TestTeamStoreCharCreateValidation(t *testing.T) {
	ts := NewTeamStore()
	// Empty name must be rejected.
	if _, err := ts.Create(TeamConfig{Members: []TeamMemberConfig{{AgentID: "a"}}}); err == nil {
		t.Fatal("expected error for empty name")
	}
	// No members must be rejected.
	if _, err := ts.Create(TeamConfig{Name: "x"}); err == nil {
		t.Fatal("expected error for no members")
	}
	if n := len(ts.List()); n != 0 {
		t.Fatalf("List len after failed creates = %d, want 0", n)
	}
}

func TestTeamStoreCharGetMissing(t *testing.T) {
	ts := NewTeamStore()
	if got := ts.Get("nope"); got != nil {
		t.Fatalf("expected nil for missing team, got %+v", got)
	}
}

func TestTeamStoreCharList(t *testing.T) {
	ts := NewTeamStore()
	for _, name := range []string{"a", "b", "c"} {
		if _, err := ts.Create(teamCfg(name)); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}
	if n := len(ts.List()); n != 3 {
		t.Fatalf("List len = %d, want 3", n)
	}
}

func TestTeamStoreCharDelete(t *testing.T) {
	ts := NewTeamStore()
	id, err := ts.Create(teamCfg("teamA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.Delete(id); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if got := ts.Get(id); got != nil {
		t.Fatalf("expected team deleted, still retrievable: %+v", got)
	}
	// Delete a missing team must error.
	if err := ts.Delete(id); err == nil {
		t.Fatal("expected error deleting already-deleted team")
	}
}