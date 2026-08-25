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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"testing"
)

func testRegistry(commands ...*SlashCommand) *SlashRegistry {
	r := NewSlashRegistry()
	for _, c := range commands {
		r.Register(c)
	}
	return r
}

func names(cs []*SlashCommand) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestSuggestPrefix(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "model"},
		&SlashCommand{Name: "memory"},
		&SlashCommand{Name: "money"},
		&SlashCommand{Name: "help"},
	)
	got := names(r.Suggest("mo", 5))
	if len(got) != 3 {
		t.Fatalf("Suggest(mo) = %v, want 3 matches", got)
	}
	want := map[string]bool{"model": true, "memory": true, "money": true}
	for _, n := range got {
		if !want[n] {
			t.Errorf("Suggest(mo) unexpected match: %s", n)
		}
	}
}

func TestSuggestSubsequence(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "btw"},
		&SlashCommand{Name: "model"},
		&SlashCommand{Name: "memory"},
		&SlashCommand{Name: "mcp"},
	)
	got := names(r.Suggest("bt", 5))
	if len(got) != 1 || got[0] != "btw" {
		t.Fatalf("Suggest(bt) = %v, want [btw] (subsequence)", got)
	}
	// "mo" should hit model/memory via subsequence (mcp lacks 'o').
	got = names(r.Suggest("mo", 5))
	if len(got) != 2 {
		t.Fatalf("Suggest(mo) subsequence = %v, want 2 matches", got)
	}
}

func TestSuggestEmptyReturnsAll(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "help"},
		&SlashCommand{Name: "mode"},
		&SlashCommand{Name: "clear"},
		&SlashCommand{Name: "quit"},
		&SlashCommand{Name: "resume"},
		&SlashCommand{Name: "compact"},
	)
	if got := len(r.Suggest("", 0)); got != 6 {
		t.Fatalf("Suggest(\"\") = %d, want 6", got)
	}
	if got := len(r.Suggest("", 4)); got != 4 {
		t.Fatalf("Suggest(\"\", 4) = %d, want 4 (limit)", got)
	}
}

func TestSuggestAliasDedup(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "quit", Aliases: []string{"q", "exit"}},
		&SlashCommand{Name: "clear"},
	)
	// "q" matches via alias AND is a subsequence of "quit" — must appear once.
	got := r.Suggest("q", 5)
	if len(got) != 1 || got[0].Name != "quit" {
		t.Fatalf("Suggest(q) = %v, want single [quit]", names(got))
	}
}

func TestSuggestCaseInsensitive(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "Model"},
		&SlashCommand{Name: "Clear"},
	)
	got := names(r.Suggest("MO", 5))
	if len(got) != 1 || got[0] != "Model" {
		t.Fatalf("Suggest(MO) = %v, want [Model] (case-insensitive)", got)
	}
}

func TestSuggestOrdering(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "mcp"},
		&SlashCommand{Name: "model"},
		&SlashCommand{Name: "memory"},
	)
	got := names(r.Suggest("m", 5))
	// Prefix matches sorted by name length: mcp(3) before model(5)/memory(6).
	if len(got) != 3 || got[0] != "mcp" {
		t.Fatalf("Suggest(m) = %v, want shortest-name first", got)
	}
}

func TestIsSubsequence(t *testing.T) {
	cases := []struct {
		prefix, s string
		want      bool
	}{
		{"bt", "btw", true},
		{"mo", "model", true},
		{"mo", "memory", true},
		{"mo", "mcp", false},
		{"mb", "memory", false},
		{"xz", "model", false},
		{"", "anything", true},
		{"mm", "memory", true},
		{"mry", "memory", true},
		{"my", "memory", true},
	}
	for _, c := range cases {
		if got := isSubsequence(c.prefix, c.s); got != c.want {
			t.Errorf("isSubsequence(%q, %q) = %v, want %v", c.prefix, c.s, got, c.want)
		}
	}
}

func TestMatch(t *testing.T) {
	cmd := &SlashCommand{Name: "model", Aliases: []string{"m", "models"}}
	for _, prefix := range []string{"mo", "MOD", "models", "mdl"} {
		if !cmd.Match(prefix) {
			t.Errorf("Match(%q) = false, want true", prefix)
		}
	}
	for _, prefix := range []string{"x", "zebra"} {
		if cmd.Match(prefix) {
			t.Errorf("Match(%q) = true, want false", prefix)
		}
	}
}

func TestRegisterOverlaySkipsConflict(t *testing.T) {
	r := NewSlashRegistry()
	r.Register(&SlashCommand{Name: "clear", Source: "inferglow", Implemented: true})
	// Overlay with the same name: must not panic, must not replace native.
	r.RegisterOverlay(&SlashCommand{Name: "clear", Aliases: []string{"reset", "new"}, Source: "compat", Implemented: true})
	if _, _, found := r.Dispatch(nil, "reset", ""); !found {
		t.Error("alias 'reset' should resolve after overlay merge")
	}
	// The native command still owns its name.
	got := r.Suggest("clear", 5)
	if len(got) != 1 || got[0].Source != "inferglow" {
		t.Fatalf("Suggest(clear) = %v, native owner must survive overlay", names(got))
	}
}

func TestRegisterOverlayNewName(t *testing.T) {
	r := NewSlashRegistry()
	r.RegisterOverlay(&SlashCommand{Name: "vim", Source: "codex", Implemented: false})
	if len(r.All()) != 1 {
		t.Fatalf("All() = %d, want 1", len(r.All()))
	}
	if r.index["vim"] == nil || r.index["vim"].Source != "codex" {
		t.Fatal("overlay-registered command not indexed")
	}
}

// TestRegisterOverlayReplacesStub verifies that an unimplemented stub can be
// replaced by a real implementation (e.g. a user skill with the same name,
// SC-6): the stub's aliases are kept and repointed, and the replacement
// occupies the same slot in registration order.
func TestRegisterOverlayReplacesStub(t *testing.T) {
	r := NewSlashRegistry()
	r.RegisterOverlay(&SlashCommand{Name: "vim", Aliases: []string{"vi"}, Source: "codex", Implemented: false})
	r.RegisterOverlay(&SlashCommand{Name: "vim", Description: "Vim skill", Source: "skill", Implemented: true})

	if got := r.SourceOf("vim"); got != "skill" {
		t.Errorf("SourceOf(vim) = %q, want skill (stub replaced)", got)
	}
	if !r.IsImplemented("vim") {
		t.Error("replaced /vim should be implemented")
	}
	if got := r.SourceOf("vi"); got != "skill" {
		t.Errorf("stub alias /vi should be repointed to the replacement, got %q", got)
	}
	if len(r.All()) != 1 {
		t.Errorf("All() = %d, want 1 (replacement occupies the stub's slot)", len(r.All()))
	}
	got := r.Suggest("vim", 5)
	if len(got) != 1 || got[0].Description != "Vim skill" {
		t.Fatalf("Suggest(vim) = %v, want [Vim skill]", names(got))
	}
}

// TestRegisterOverlayKeepsImplemented verifies implemented commands are never
// replaced by overlay registrations (the native-wins invariant).
func TestRegisterOverlayKeepsImplemented(t *testing.T) {
	r := NewSlashRegistry()
	r.RegisterOverlay(&SlashCommand{Name: "tasks", Source: "skill", Implemented: true, Description: "skill tasks"})
	r.RegisterOverlay(&SlashCommand{Name: "tasks", Source: "skill", Implemented: true, Description: "second skill tasks"})
	if got := r.index["tasks"].Description; got != "skill tasks" {
		t.Errorf("implemented /tasks was replaced: %q", got)
	}
	if len(r.All()) != 1 {
		t.Errorf("All() = %d, want 1", len(r.All()))
	}
}

func TestImplementedField(t *testing.T) {
	c := &SlashCommand{Name: "vim", Source: "codex", Implemented: false}
	if c.Implemented {
		t.Fatal("Implemented should default false for stub commands")
	}
	native := &SlashCommand{Name: "clear", Source: "inferglow", Implemented: true}
	if !native.Implemented {
		t.Fatal("native command should be Implemented=true")
	}
}
