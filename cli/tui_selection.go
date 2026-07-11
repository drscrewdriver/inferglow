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
	"os/exec"
	"runtime"
	"strings"
)

// enterSelectionMode starts visual selection mode (vim-like).
func (m *chatTUI) enterSelectionMode() {
	m.selectionMode = true
	m.selectionStart = m.viewport.YOffset()
	m.selectionEnd = m.selectionStart
}

// exitSelectionMode clears the current selection.
func (m *chatTUI) exitSelectionMode() {
	m.selectionMode = false
	m.selectionStart = 0
	m.selectionEnd = 0
}

// copySelection copies the selected transcript range to the system clipboard.
// Uses OSC 52 escape sequence for cross-SSH compatibility, with fallback
// to xclip/xsel/pbcopy.
func (m *chatTUI) copySelection() string {
	if !m.selectionMode || m.selectionStart == m.selectionEnd {
		return ""
	}
	start, end := m.selectionStart, m.selectionEnd
	if start > end {
		start, end = end, start
	}
	// Extract plain text from transcript blocks.
	var lines []string
	for i := start; i <= end && i < len(m.transcript); i++ {
		lines = append(lines, m.transcript[i].Source)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		return ""
	}
	// Try OSC 52 first (works over SSH).
	osc52Copy(text)
	// Fallback to system clipboard.
	systemCopy(text)
	return text
}

// osc52Copy writes an OSC 52 escape sequence to copy text to clipboard.
func osc52Copy(text string) {
	// OSC 52 format: \033]52;c;<base64-encoded-text>\007
	// Simplified: most terminals support this for the system clipboard.
	encoded := base64Encode(text)
	fmt.Printf("\033]52;c;%s\007", encoded)
}

// systemCopy copies text to the system clipboard using platform tools.
func systemCopy(text string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return // no clipboard tool available
		}
	default:
		return
	}
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Start()
	_ = cmd.Wait()
}

// base64Encode is a minimal base64 encoder (no import needed).
func base64Encode(s string) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var b strings.Builder
	data := []byte(s)
	for i := 0; i < len(data); i += 3 {
		var n uint32
		remaining := len(data) - i
		switch {
		case remaining >= 3:
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			b.WriteByte(table[n>>18&0x3F])
			b.WriteByte(table[n>>12&0x3F])
			b.WriteByte(table[n>>6&0x3F])
			b.WriteByte(table[n&0x3F])
		case remaining == 2:
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8
			b.WriteByte(table[n>>18&0x3F])
			b.WriteByte(table[n>>12&0x3F])
			b.WriteByte(table[n>>6&0x3F])
			b.WriteByte('=')
		case remaining == 1:
			n = uint32(data[i]) << 16
			b.WriteByte(table[n>>18&0x3F])
			b.WriteByte(table[n>>12&0x3F])
			b.WriteByte('=')
			b.WriteByte('=')
		}
	}
	return b.String()
}
