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

import tea "charm.land/bubbletea/v2"

// enterScrollbackMode switches the TUI into vim-like scrollback mode.
func (m *chatTUI) enterScrollbackMode() {
	m.scrollbackMode = true
	m.scrollbackOffset = len(m.transcript) - 1
}

// exitScrollbackMode returns the TUI to normal mode.
func (m *chatTUI) exitScrollbackMode() {
	m.scrollbackMode = false
	m.scrollbackOffset = 0
	m.viewport.GotoBottom()
}

// handleScrollbackKey processes key input during scrollback mode.
// Returns (handled, model, cmd).
func (m *chatTUI) handleScrollbackKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.exitScrollbackMode()
		return true, m, nil
	case "j", "down":
		m.viewport.ScrollDown(1)
		return true, m, nil
	case "k", "up":
		m.viewport.ScrollUp(1)
		return true, m, nil
	case "ctrl+d":
		m.viewport.HalfPageDown()
		return true, m, nil
	case "ctrl+u":
		m.viewport.HalfPageUp()
		return true, m, nil
	case "g", "home":
		m.viewport.GotoTop()
		return true, m, nil
	case "G", "end":
		m.viewport.GotoBottom()
		return true, m, nil
	}
	return false, m, nil
}

// scrollbackStatusLine returns the scrollback position indicator.
func (m *chatTUI) scrollbackStatusLine() string {
	total := len(m.transcript)
	if total == 0 {
		return dim("Scrollback (0/0)")
	}
	// Approximate current visible line from viewport offset.
	pos := m.viewport.YOffset()
	return dim("Scrollback (" + itoa(pos) + "/" + itoa(total) + ")")
}

// itoa is a simple int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
