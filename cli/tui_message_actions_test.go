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

func testMessages(n int) []selectableMessage {
	out := make([]selectableMessage, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, selectableMessage{index: i, text: "msg"})
	}
	return out
}

func TestMessageActionsSelection(t *testing.T) {
	var m MessageActionsMenu
	m.EnterSelectionMode(testMessages(3))
	if !m.Active() {
		t.Fatal("selection mode should be active with messages")
	}
	if m.SelectedMessage() == nil || m.SelectedMessage().index != 0 {
		t.Fatal("first message should be selected")
	}
	m.Move(+1)
	if m.SelectedMessage().index != 1 {
		t.Fatalf("Move(+1) = %d, want 1", m.SelectedMessage().index)
	}
	m.Move(+1)
	if m.SelectedMessage().index != 2 {
		t.Fatalf("Move(+1) = %d, want 2", m.SelectedMessage().index)
	}
	m.Move(+1) // cyclic
	if m.SelectedMessage().index != 0 {
		t.Fatalf("Move(+1) cyclic = %d, want 0", m.SelectedMessage().index)
	}
	m.Move(-1)
	if m.SelectedMessage().index != 2 {
		t.Fatalf("Move(-1) = %d, want 2", m.SelectedMessage().index)
	}
	m.Exit()
	if m.Active() {
		t.Fatal("Exit() should leave selection mode")
	}
}

func TestMessageActionsEmpty(t *testing.T) {
	var m MessageActionsMenu
	m.EnterSelectionMode(nil)
	if m.Active() {
		t.Fatal("empty message list must not activate selection mode")
	}
}

func TestMessageActionsMenuOpenAndMove(t *testing.T) {
	var m MessageActionsMenu
	m.EnterSelectionMode(testMessages(1))
	m.OpenMenu()
	if !m.MenuVisible() {
		t.Fatal("OpenMenu() should open the menu")
	}
	// Default cursor: first item (Revert).
	if m.Current() != ActionRevert {
		t.Fatalf("Current() = %v, want Revert", m.Current())
	}
	m.MoveMenu(+1)
	if m.Current() != ActionCopy {
		t.Fatalf("MoveMenu(+1) = %v, want Copy", m.Current())
	}
	m.MoveMenu(+1)
	if m.Current() != ActionFork {
		t.Fatalf("MoveMenu(+1) = %v, want Fork", m.Current())
	}
	m.MoveMenu(+1) // cyclic
	if m.Current() != ActionRevert {
		t.Fatalf("MoveMenu(+1) cyclic = %v, want Revert", m.Current())
	}
	m.CloseMenu()
	if m.MenuVisible() {
		t.Fatal("CloseMenu() should hide the menu")
	}
	if !m.Active() {
		t.Fatal("CloseMenu() should stay in selection mode")
	}
}

func TestMessageActionsIsSelected(t *testing.T) {
	var m MessageActionsMenu
	m.EnterSelectionMode([]selectableMessage{{index: 5, text: "x"}, {index: 9, text: "y"}})
	if !m.isSelected(5) || m.isSelected(9) {
		t.Fatal("isSelected should track the selected block index")
	}
	m.Move(+1)
	if m.isSelected(5) || !m.isSelected(9) {
		t.Fatal("isSelected should follow Move")
	}
}

func TestMessageActionsRender(t *testing.T) {
	var m MessageActionsMenu
	m.EnterSelectionMode(testMessages(1))
	m.OpenMenu()
	out := m.Render(60)
	for _, want := range []string{"Message Actions", "Revert", "Copy", "Fork"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Render missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\033[48;5;") {
		t.Fatalf("Render should highlight the cursor row:\n%s", out)
	}
	if m.Render(60) == "" {
		t.Fatal("menu render should be non-empty while visible")
	}
	m.CloseMenu()
	if m.Render(60) != "" {
		t.Fatal("closed menu should render empty")
	}
}

func TestCollectUserMessages(t *testing.T) {
	m := &chatTUI{}
	m.commitUserBubble("hello")
	m.commitLine("assistant reply")
	m.commitUserBubble("world")
	msgs := m.collectUserMessages()
	if len(msgs) != 2 {
		t.Fatalf("collectUserMessages = %d, want 2", len(msgs))
	}
	if msgs[0].text != "hello" || msgs[1].text != "world" {
		t.Fatalf("collected texts wrong: %q, %q", msgs[0].text, msgs[1].text)
	}
}
