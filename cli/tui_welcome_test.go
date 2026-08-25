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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTipsForGroup(t *testing.T) {
	all := tipsForGroup("")
	if len(all) == 0 {
		t.Fatal("tip pool should be non-empty")
	}
	// Every tip ≤60 chars.
	for _, tip := range all {
		if len([]rune(tip.text)) > 60 {
			t.Fatalf("tip %s exceeds 60 chars: %d", tip.id, len([]rune(tip.text)))
		}
	}
	keys := tipsForGroup("keys")
	if len(keys) == 0 {
		t.Fatal("keys group should be non-empty")
	}
	for _, tip := range keys {
		if tip.group != "keys" {
			t.Fatalf("tip %s in wrong group %s", tip.id, tip.group)
		}
	}
	// Unknown group → empty.
	if got := tipsForGroup("nope"); len(got) != 0 {
		t.Fatalf("unknown group should yield empty, got %d", len(got))
	}
}

func TestWelcomeGroups(t *testing.T) {
	groups := welcomeGroups(welcomeTips)
	// Canonical order.
	expected := []string{"keys", "commands", "workflow", "display", "pitfalls"}
	if len(groups) != len(expected) {
		t.Fatalf("groups = %v, want %v", groups, expected)
	}
	for i, g := range groups {
		if g != expected[i] {
			t.Fatalf("group order: %v, want %v", groups, expected)
		}
	}
}

func TestRenderWelcomePagination(t *testing.T) {
	m := &chatTUI{}
	m.welcome.visible = true
	out := m.renderWelcome(60)
	if out == "" {
		t.Fatal("welcome should render when visible")
	}
	if !strings.Contains(out, "Esc 关闭") {
		t.Fatal("welcome should include the close hint")
	}
	// More tips than one page → page indicator + Tab hint.
	m.welcome.group = "commands"
	out2 := m.renderWelcome(60)
	if !strings.Contains(out2, "Tab 翻页") && len(tipsForGroup("commands")) > welcomePageSize {
		t.Fatalf("multi-page welcome should mention Tab: %s", out2)
	}
	// Hidden → empty.
	m.welcome.visible = false
	if out := m.renderWelcome(60); out != "" {
		t.Fatal("hidden welcome should render empty")
	}
}

func TestWelcomeSeenFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), welcomeSeenFile)
	if welcomeSeenFrom(path) {
		t.Fatal("missing marker should be unseen")
	}
	// Write a marker directly to the temp path (never touch the real
	// ~/.inferglow/welcome_seen.json — that would suppress the first-run
	// welcome page for the user).
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"seen":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !welcomeSeenFrom(path) {
		t.Fatal("marker with seen=true should be seen")
	}
	// Corrupt → treated as seen (never nag again).
	path2 := filepath.Join(t.TempDir(), welcomeSeenFile)
	if err := os.WriteFile(path2, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !welcomeSeenFrom(path2) {
		t.Fatal("corrupt marker should be treated as seen")
	}
}

func TestHandleTipsGroupFilter(t *testing.T) {
	m := &chatTUI{}
	_, _ = tuiHandleTips(m, "display")
	if !m.welcome.visible || m.welcome.group != "display" {
		t.Fatalf("tips display: visible=%v group=%q", m.welcome.visible, m.welcome.group)
	}
	// Unknown group is rejected (welcome untouched).
	m.welcome.group = ""
	m.welcome.visible = false
	_, _ = tuiHandleTips(m, "bogus")
	if m.welcome.visible || m.welcome.group != "" {
		t.Fatal("unknown tip group should be rejected")
	}
}

func TestRenderWelcomeGroupHeader(t *testing.T) {
	m := &chatTUI{}
	m.welcome.visible = true
	m.welcome.group = "pitfalls"
	out := m.renderWelcome(60)
	if !strings.Contains(out, "避坑") {
		t.Fatalf("group header missing: %s", out)
	}
}

// TestRenderWelcomeZeroWidth is a regression test for the first-frame panic:
// Bubble Tea renders View with width=0 before the first WindowSizeMsg, and
// renderWelcome must not panic on negative strings.Repeat counts (BUG: fixed
// by skipping rendering while the terminal size is unknown).
func TestRenderWelcomeZeroWidth(t *testing.T) {
	m := &chatTUI{}
	m.welcome.visible = true
	for _, w := range []int{0, 1, 5, 9} {
		if out := m.renderWelcome(w); out != "" {
			t.Fatalf("renderWelcome(%d) should be empty, got %q", w, out)
		}
	}
	// Normal width still renders.
	if out := m.renderWelcome(60); out == "" {
		t.Fatal("renderWelcome(60) should render")
	}
}

// TestRenderWelcomeLogo verifies the firefly logo renders on wide terminals
// and is skipped on narrow ones (no overflow).
func TestRenderWelcomeLogo(t *testing.T) {
	m := &chatTUI{}
	m.welcome.visible = true
	// Logo rows are ≤ 81 chars — assert the constant stays sane.
	for i, ln := range inferglowLogo {
		if n := len([]rune(ln)); n > 84 {
			t.Fatalf("logo row %d is %d chars, want ≤84", i, n)
		}
	}
	// Wide terminal: logo + sparkles present.
	wide := m.renderWelcome(90)
	if !strings.Contains(wide, "██╗") || !strings.Contains(wide, "✦") {
		t.Fatal("wide welcome should render the firefly logo")
	}
	if !strings.Contains(wide, "快速上手") {
		t.Fatal("wide welcome should still render the tips box")
	}
	// Narrow terminal: no logo, tips box only.
	narrow := m.renderWelcome(60)
	if strings.Contains(narrow, "██╗") {
		t.Fatal("narrow welcome should skip the logo")
	}
	if !strings.Contains(narrow, "快速上手") {
		t.Fatal("narrow welcome should still render the tips box")
	}
}
