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
	"fmt"
	"strings"
)

// MessageAction identifies an operation on a historical user message (SC-4).
type MessageAction int

const (
	ActionRevert MessageAction = iota // undo message + file changes (stub)
	ActionCopy                        // copy message text to clipboard
	ActionFork                        // fork a new session (stub)
	ActionEdit                        // edit & resend (not offered yet)
)

// String returns the display name of the action.
func (a MessageAction) String() string {
	switch a {
	case ActionRevert:
		return "Revert"
	case ActionCopy:
		return "Copy"
	case ActionFork:
		return "Fork"
	case ActionEdit:
		return "Edit"
	}
	return "?"
}

// Description returns the one-line hint shown next to the action name.
func (a MessageAction) Description() string {
	switch a {
	case ActionRevert:
		return "undo messages and file changes"
	case ActionCopy:
		return "message text to clipboard"
	case ActionFork:
		return "create a new session"
	case ActionEdit:
		return "edit and resend the message"
	}
	return ""
}

// actionIcon returns the menu icon for the action.
func actionIcon(a MessageAction) string {
	switch a {
	case ActionRevert:
		return "↩"
	case ActionCopy:
		return "⧉"
	case ActionFork:
		return "⑂"
	case ActionEdit:
		return "✎"
	}
	return "•"
}

// selectableMessage is a user message the action menu can target.
type selectableMessage struct {
	index int    // transcript block index
	text  string // plain text source
}

// MessageActionsMenu is the history message action menu state (SC-4).
// Selection mode highlights a user message; [o]/[Enter] opens the action
// menu (Revert / Copy / Fork); Esc backs out one level.
type MessageActionsMenu struct {
	active      bool
	messages    []selectableMessage
	selected    int
	menuVisible bool
	menuItems   []MessageAction
	menuCursor  int
}

// EnterSelectionMode starts message selection mode over the given messages.
// No-op (stays inactive) when the list is empty.
func (m *MessageActionsMenu) EnterSelectionMode(messages []selectableMessage) {
	m.messages = messages
	m.selected = 0
	m.menuVisible = false
	m.menuCursor = 0
	m.menuItems = []MessageAction{ActionRevert, ActionCopy, ActionFork}
	m.active = len(messages) > 0
}

// Exit leaves selection mode entirely.
func (m *MessageActionsMenu) Exit() {
	m.active = false
	m.messages = nil
	m.selected = 0
	m.menuVisible = false
	m.menuCursor = 0
}

// Active reports whether selection mode is on.
func (m *MessageActionsMenu) Active() bool { return m.active }

// MenuVisible reports whether the action menu is open.
func (m *MessageActionsMenu) MenuVisible() bool { return m.menuVisible }

// Move moves the message selection cursor (cyclic).
func (m *MessageActionsMenu) Move(delta int) {
	if len(m.messages) == 0 {
		m.selected = 0
		return
	}
	m.selected = (m.selected + delta + len(m.messages)) % len(m.messages)
}

// MoveMenu moves the menu cursor (cyclic).
func (m *MessageActionsMenu) MoveMenu(delta int) {
	if len(m.menuItems) == 0 {
		m.menuCursor = 0
		return
	}
	m.menuCursor = (m.menuCursor + delta + len(m.menuItems)) % len(m.menuItems)
}

// OpenMenu opens the action menu for the selected message.
func (m *MessageActionsMenu) OpenMenu() {
	if len(m.messages) == 0 {
		return
	}
	m.menuVisible = true
	m.menuCursor = 0
}

// CloseMenu closes the menu, staying in selection mode.
func (m *MessageActionsMenu) CloseMenu() {
	m.menuVisible = false
}

// SelectedMessage returns the currently selected message, if any.
func (m *MessageActionsMenu) SelectedMessage() *selectableMessage {
	if m.selected >= 0 && m.selected < len(m.messages) {
		return &m.messages[m.selected]
	}
	return nil
}

// Current returns the menu item under the cursor.
func (m *MessageActionsMenu) Current() MessageAction {
	if m.menuCursor >= 0 && m.menuCursor < len(m.menuItems) {
		return m.menuItems[m.menuCursor]
	}
	return ActionCopy
}

// isSelected reports whether transcript block index i is the selected user
// message (used by the transcript renderer for highlighting).
func (m *MessageActionsMenu) isSelected(i int) bool {
	sel := m.SelectedMessage()
	return sel != nil && sel.index == i
}

// Render produces the menu overlay rows ("" when closed): a header, a
// divider, and one row per menu item with the cursor row highlighted.
func (m *MessageActionsMenu) Render(width int) string {
	if !m.active || !m.menuVisible {
		return ""
	}
	var sb strings.Builder
	header := "  Message Actions    [Esc]"
	if width > 0 {
		header = truncateToWidth(header, width)
	}
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(dim(strings.Repeat("─", max(width-2, 1))))
	for i, a := range m.menuItems {
		row := fmt.Sprintf("  %s %-8s %s", actionIcon(a), a.String(), a.Description())
		if width > 0 {
			row = truncateToWidth(row, width)
		}
		if i == m.menuCursor {
			row = selectionRow(row)
		}
		sb.WriteString("\n")
		sb.WriteString(row)
	}
	return sb.String()
}

// collectUserMessages extracts selectable user-message blocks from the
// transcript (SC-4).
func (m *chatTUI) collectUserMessages() []selectableMessage {
	var out []selectableMessage
	for i, b := range m.transcript {
		if b.Kind != blockUser {
			continue
		}
		text := b.Source
		if text == "" {
			text = stripAnsi(b.Raw)
		}
		out = append(out, selectableMessage{index: i, text: text})
	}
	return out
}

// executeMessageAction runs the menu item under the cursor (SC-4).
// Copy is implemented; Revert/Fork/Edit are recognized stubs for this
// release (they print a friendly "in development" note).
func (m *chatTUI) executeMessageAction() {
	action := m.messageActions.Current()
	switch action {
	case ActionCopy:
		if msg := m.messageActions.SelectedMessage(); msg != nil {
			if err := WriteClipboardText(msg.text); err == nil {
				m.commitSystemNote(successText("Copied message to clipboard."))
			} else {
				m.commitSystemNote(warnText("Clipboard unavailable"))
			}
		}
	default:
		m.commitSystemNote(dim(action.String() + " 功能开发中"))
	}
	m.messageActions.Exit()
	m.transcriptDirty = true
}
