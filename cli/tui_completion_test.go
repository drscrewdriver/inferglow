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
	"strings"
	"testing"
)

func TestCompletionWantsOpen(t *testing.T) {
	var p completionPopup
	cases := []struct {
		input string
		state tuiState
		want  bool
	}{
		{"/", tuiIdle, true},
		{"/mo", tuiIdle, true},
		{"/model ", tuiIdle, false},   // space = already selected command
		{"/mo\nx", tuiIdle, false},    // cursor not on first line
		{"mo", tuiIdle, false},        // not a slash command
		{"/mo", tuiRunning, false},    // busy
		{"/" + strings.Repeat("x", 40), tuiIdle, false}, // over length limit
	}
	for _, c := range cases {
		if got := p.wantsOpen(c.input, c.state); got != c.want {
			t.Errorf("wantsOpen(%q, %v) = %v, want %v", c.input, c.state, got, c.want)
		}
	}
}

func TestCompletionRefreshMoveSelect(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "model", Description: "model desc", Implemented: true},
		&SlashCommand{Name: "memory", Description: "memory desc", Implemented: true},
		&SlashCommand{Name: "mcp", Description: "mcp desc", Implemented: true},
	)
	var p completionPopup
	p.Refresh("mo", r)
	if !p.active || len(p.items) != 2 {
		t.Fatalf("Refresh(mo): active=%v items=%d, want active, 2 items", p.active, len(p.items))
	}
	if p.Selected() == nil || p.Selected().Name != "model" {
		t.Fatal("first candidate should be selected by default")
	}
	p.Move(+1)
	if p.Selected().Name != "memory" {
		t.Fatalf("Move(+1) selected = %v, want memory", p.Selected().Name)
	}
	p.Move(+1) // cycle: memory -> model
	if p.Selected().Name != "model" {
		t.Fatalf("Move(+1) again = %s, want model (cyclic)", p.Selected().Name)
	}
	p.Move(-1)
	if p.Selected().Name != "memory" {
		t.Fatalf("Move(-1) = %s, want memory", p.Selected().Name)
	}
	// Typing more keeps a valid selection.
	p.Refresh("mem", r)
	if p.Selected() == nil || p.Selected().Name != "memory" {
		t.Fatalf("Refresh(mem) should keep valid selection, got %v", p.Selected())
	}
	// Refresh with a query that yields nothing keeps popup open but empty.
	p.Refresh("zzz", r)
	if len(p.items) != 0 || p.Selected() != nil {
		t.Fatal("Refresh(zzz) should yield empty items and nil selection")
	}
	p.Close()
	if p.active || p.Render(80) != "" {
		t.Fatal("Close() should deactivate and render nothing")
	}
}

func TestCompletionRender(t *testing.T) {
	r := NewSlashRegistry()
	r.Register(&SlashCommand{Name: "model", Description: "switch model", Implemented: true})
	r.RegisterOverlay(&SlashCommand{Name: "graphify", Description: "knowledge graph", Source: "skill", Implemented: true})
	r.RegisterOverlay(&SlashCommand{Name: "vim", Description: "vim keymap", Source: "codex", Implemented: false})
	var p completionPopup
	p.Refresh("", r)
	out := p.Render(80)
	if !strings.Contains(out, "/model") || !strings.Contains(out, "switch model") {
		t.Fatalf("Render missing implemented row:\n%s", out)
	}
	if !strings.Contains(out, "/vim") {
		t.Fatalf("Render missing stub row:\n%s", out)
	}
	if !strings.Contains(out, "○") {
		t.Fatalf("Render stub row should use hollow marker:\n%s", out)
	}
	// SC-6: skill rows use the placeholder marker ◇.
	if !strings.Contains(out, "◇") || !strings.Contains(out, "/graphify") {
		t.Fatalf("Render skill row should use placeholder marker ◇:\n%s", out)
	}
	if strings.Contains(out, "● /graphify") {
		t.Fatalf("skill row must not use the implemented marker:\n%s", out)
	}
	p.Move(+1)
	out = p.Render(80)
	if !strings.Contains(out, "\033[48;5;") {
		t.Fatalf("Render should highlight the selected row:\n%s", out)
	}
	// Width truncation must not panic and must shorten the output.
	out = p.Render(8)
	if visibleWidth(out) > 8*3 { // three rows, each truncated to ~8
		t.Fatalf("Render(8) not truncated: %q", out)
	}
}

func TestCompletionCommonPrefix(t *testing.T) {
	var p completionPopup
	p.items = []*SlashCommand{
		{Name: "mode"},
		{Name: "model"},
		{Name: "money"},
		{Name: "memory"},
	}
	cases := []struct{ cur, want string }{
		{"mo", "mo"},      // all share exactly "mo"
		{"mod", "mode"},   // mode/model share "mode"
		{"mone", "money"}, // only money extends "mone"
		{"x", "x"},        // no candidate starts with x
	}
	for _, c := range cases {
		if got := p.commonPrefix(c.cur); got != c.want {
			t.Errorf("commonPrefix(%q) = %q, want %q", c.cur, got, c.want)
		}
	}
	// Subsequence-only candidates (no strict prefix) → no alignment.
	p.items = []*SlashCommand{{Name: "compose"}}
	if got := p.commonPrefix("mo"); got != "mo" {
		t.Errorf("commonPrefix(mo) with subsequence-only candidate = %q, want %q", got, "mo")
	}
}

func TestCompletionCycleAlign(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "mode", Description: "mode desc", Implemented: true},
		&SlashCommand{Name: "model", Description: "model desc", Implemented: true},
		&SlashCommand{Name: "money", Description: "money desc", Implemented: true},
	)
	var p completionPopup
	p.Refresh("mod", r) // candidates: mode, model (money does not extend "mod")
	if len(p.items) != 2 {
		t.Fatalf("Refresh(mod) items = %d, want 2", len(p.items))
	}

	// First Tab: align to the longest common prefix "mode".
	if got := p.Cycle("/mod"); got != "/mode " {
		t.Fatalf("Cycle(/mod) = %q, want aligned \"/mode \"", got)
	}
	// Input is now aligned; the selection has not moved yet.
	if sel := p.Selected(); sel == nil || sel.Name != "mode" {
		t.Fatalf("selection after align = %v, want mode (unchanged)", sel)
	}

	// Next Tabs: cycle through the candidates, committing full names.
	if got := p.Cycle("/mode "); got != "/model " {
		t.Fatalf("Cycle(/mode ) = %q, want \"/model \"", got)
	}
	if got := p.Cycle("/model "); got != "/mode " {
		t.Fatalf("Cycle(/model ) = %q, want \"/mode \" (wrap)", got)
	}
}

func TestCompletionCycleNoAlign(t *testing.T) {
	r := testRegistry(
		&SlashCommand{Name: "mode", Description: "mode desc", Implemented: true},
		&SlashCommand{Name: "model", Description: "model desc", Implemented: true},
		&SlashCommand{Name: "money", Description: "money desc", Implemented: true},
	)
	var p completionPopup
	p.Refresh("mo", r) // candidates: mode(4), model(5), money(5); lcp == "mo"

	// No alignment possible → first Tab cycles straight to the next candidate.
	if got := p.Cycle("/mo"); got != "/model " {
		t.Fatalf("Cycle(/mo) = %q, want \"/model \" (next candidate)", got)
	}
	if got := p.Cycle("/model "); got != "/money " {
		t.Fatalf("Cycle(/model ) = %q, want \"/money \"", got)
	}
	if got := p.Cycle("/money "); got != "/mode " {
		t.Fatalf("Cycle(/money ) = %q, want \"/mode \" (wrap)", got)
	}
}

func TestCompletionCycleClosed(t *testing.T) {
	var p completionPopup // not active
	if got := p.Cycle("/mod"); got != "" {
		t.Fatalf("Cycle on closed popup = %q, want \"\"", got)
	}
}

func TestCompletionIsCycling(t *testing.T) {
	var p completionPopup
	p.items = []*SlashCommand{
		{Name: "mode"},
		{Name: "model"},
	}
	p.active = true
	cases := []struct {
		input string
		want  bool
	}{
		{"/mode ", true},   // exact candidate name (Tab-cycle commit)
		{"/model ", true},  // exact candidate name
		{"/mod ", true},    // prefix of a candidate (alignment commit)
		{"/modex ", false}, // not a candidate name or prefix
		{"/gr ", false},    // unrelated
		{"mode", false},    // missing leading slash
	}
	for _, c := range cases {
		if got := p.isCycling(c.input); got != c.want {
			t.Errorf("isCycling(%q) = %v, want %v", c.input, got, c.want)
		}
	}
	// Closed popup never cycles.
	p.active = false
	if p.isCycling("/mode ") {
		t.Error("closed popup must not report cycling")
	}
}
