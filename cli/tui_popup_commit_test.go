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

package cli

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// popupTestTUI builds a minimal chatTUI for popup-interaction tests.
func popupTestTUI(r *SlashRegistry) *chatTUI {
	ti := textarea.New()
	ti.SetValue("/")
	return &chatTUI{
		input:       ti,
		completion:  completionPopup{},
		cmdRegistry: r,
	}
}

// TestCommitPopupSelectionSkillPlaceholder verifies the SC-6 placeholder
// contract: pressing Enter on a skill candidate commits only the command
// name into the input box (nothing is dispatched, nothing is loaded) — the
// skill activates later when the user confirms with a second Enter.
func TestCommitPopupSelectionSkillPlaceholder(t *testing.T) {
	r := NewSlashRegistry()
	r.RegisterOverlay(&SlashCommand{Name: "graphify", Description: "knowledge graph", Source: "skill", Implemented: true})
	m := popupTestTUI(r)
	m.completion.Refresh("gr", r)
	sel := m.completion.Selected()
	if sel == nil || sel.Name != "graphify" {
		t.Fatalf("selection = %v, want graphify", sel)
	}
	cmd, quit := m.commitPopupSelection(sel)
	if quit || cmd != nil {
		t.Fatalf("placeholder commit must not dispatch: quit=%v cmd=%v", quit, cmd)
	}
	if m.completion.active {
		t.Error("popup must close after commit")
	}
	if got := m.input.Value(); got != "/graphify " {
		t.Fatalf("input after skill commit = %q, want placeholder \"/graphify \"", got)
	}
}

// TestCommitPopupSelectionBuiltinDispatches verifies non-skill candidates
// keep the blueprint behavior: Enter triggers them immediately.
func TestCommitPopupSelectionBuiltinDispatches(t *testing.T) {
	r := NewSlashRegistry()
	invoked := false
	r.Register(&SlashCommand{Name: "clear", Handler: func(m *chatTUI, args string) (tea.Cmd, bool) {
		invoked = true
		return nil, false
	}})
	m := popupTestTUI(r)
	m.completion.Refresh("cl", r)
	sel := m.completion.Selected()
	if sel == nil || sel.Name != "clear" {
		t.Fatalf("selection = %v, want clear", sel)
	}
	cmd, quit := m.commitPopupSelection(sel)
	if quit {
		t.Fatal("clear must not quit")
	}
	if !invoked {
		t.Fatal("builtin candidate must dispatch immediately on Enter")
	}
	if cmd != nil {
		t.Fatalf("unexpected cmd: %v", cmd)
	}
	if m.completion.active {
		t.Error("popup must close after dispatch")
	}
	if got := m.input.Value(); got != "/" {
		t.Fatalf("input must stay untouched for immediate dispatch, got %q", got)
	}
}

// TestCommitPopupSelectionNilSafe verifies a nil selection is a no-op.
func TestCommitPopupSelectionNilSafe(t *testing.T) {
	m := popupTestTUI(NewSlashRegistry())
	cmd, quit := m.commitPopupSelection(nil)
	if quit || cmd != nil {
		t.Fatalf("nil selection must be a no-op: quit=%v cmd=%v", quit, cmd)
	}
}
