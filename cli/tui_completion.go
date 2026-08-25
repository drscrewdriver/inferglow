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

	tea "charm.land/bubbletea/v2"
)

const (
	// popupMaxCandidates caps the number of suggestion rows rendered.
	popupMaxCandidates = 10
	// popupMaxQueryLen caps the command-name query length for popup activation.
	popupMaxQueryLen = 32
)

// completionPopup is the IME-style "/" prefix autocomplete popup (SC-2).
// It renders above the input box while the user edits a slash-command name.
type completionPopup struct {
	active   bool
	query    string          // query without the leading "/"
	items    []*SlashCommand // current candidates
	selected int             // index into items; -1 = nothing selected
}

// Active reports whether the popup is currently open.
func (p *completionPopup) Active() bool { return p.active }

// wantsOpen reports whether the popup context applies: input starts with "/",
// the cursor is on the first line (no newline), the query contains no space
// (still editing the command name), state is idle, and the query length is
// within the limit.
func (p *completionPopup) wantsOpen(input string, state tuiState) bool {
	if state != tuiIdle || !strings.HasPrefix(input, "/") {
		return false
	}
	if strings.ContainsAny(input, "\n ") {
		return false
	}
	q := strings.TrimPrefix(input, "/")
	if len([]rune(q)) > popupMaxQueryLen {
		return false
	}
	return true
}

// Refresh re-computes candidates for query and opens the popup. The
// selection is kept when it still points at a valid candidate; otherwise the
// first candidate is selected (or none when the list is empty).
func (p *completionPopup) Refresh(query string, registry *SlashRegistry) {
	p.query = query
	p.active = true
	if registry == nil {
		p.items = nil
	} else {
		p.items = registry.Suggest(query, popupMaxCandidates)
	}
	if len(p.items) == 0 {
		p.selected = -1
	} else if p.selected < 0 || p.selected >= len(p.items) {
		p.selected = 0
	}
}

// Move moves the selection by delta, cycling within the candidate list.
func (p *completionPopup) Move(delta int) {
	n := len(p.items)
	if n == 0 {
		p.selected = -1
		return
	}
	if p.selected < 0 {
		if delta < 0 {
			p.selected = n - 1
		} else {
			p.selected = 0
		}
		return
	}
	p.selected = (p.selected + delta + n) % n
}

// Selected returns the currently selected candidate, if any.
func (p *completionPopup) Selected() *SlashCommand {
	if p.active && p.selected >= 0 && p.selected < len(p.items) {
		return p.items[p.selected]
	}
	return nil
}

// Cycle implements the Tab key (SC-2): first it aligns the input's command
// name to the longest common prefix of all prefix-matching candidates —
// expanding e.g. "/mod" to "/mode " when the candidates are mode/model.
// Once the input is already aligned (no further common prefix), it cycles
// the selection to the next candidate and commits its full name into the
// input, wrapping around. The popup stays open throughout, so the user
// keeps choosing among the candidates "in the middle of the prefix".
// Returns the new input text, or "" to keep the current input.
func (p *completionPopup) Cycle(input string) string {
	sel := p.Selected()
	if sel == nil {
		return ""
	}
	cur := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if lcp := p.commonPrefix(cur); len(lcp) > len(cur) {
		// Alignment: expand to the maximum common prefix, keep the popup
		// and the current selection (the user may keep typing or Tab).
		return "/" + lcp + " "
	}
	// Already aligned (or nothing more in common): cycle to the next
	// candidate and commit its full name as a placeholder.
	p.Move(+1)
	if s := p.Selected(); s != nil {
		return "/" + s.Name + " "
	}
	return ""
}

// commonPrefix returns the longest common prefix of the candidates that
// strictly extend cur (i.e. whose names start with cur). When no candidate
// has cur as a prefix, cur itself is returned (no alignment possible).
func (p *completionPopup) commonPrefix(cur string) string {
	var prefixed []string
	for _, c := range p.items {
		if strings.HasPrefix(c.Name, cur) {
			prefixed = append(prefixed, c.Name)
		}
	}
	if len(prefixed) == 0 {
		return cur
	}
	lcp := prefixed[0]
	for _, n := range prefixed[1:] {
		for !strings.HasPrefix(n, lcp) && len(lcp) > 0 {
			lcp = lcp[:len(lcp)-1]
		}
		if lcp == "" {
			break
		}
	}
	return lcp
}

// isCycling reports whether the input currently holds a command name that
// is a prefix of (or equal to) one of the popup candidates — i.e. a
// Tab-cycle / alignment commit. The popup must stay open in that state so
// the user can keep cycling with Tab; any other edit closes it through the
// regular wantsOpen/Close sync.
func (p *completionPopup) isCycling(input string) bool {
	if !p.active || len(p.items) == 0 {
		return false
	}
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return false
	}
	name := strings.TrimPrefix(trimmed, "/")
	for _, c := range p.items {
		if c.Name == name || strings.HasPrefix(c.Name, name) {
			return true
		}
	}
	return false
}

// Close closes the popup and drops its state.
func (p *completionPopup) Close() {
	p.active = false
	p.query = ""
	p.items = nil
	p.selected = -1
}

// commitPopupSelection handles the popup Enter action. Skill candidates are
// placeholders (SC-6): selecting one only commits the command name into the
// input box — the skill content is activated later, when the user confirms
// the merged input with Enter (dispatch then goes through the normal submit
// path). All other candidates dispatch immediately. Returns the dispatch
// command and quit flag (nil/false for placeholder commits).
func (m *chatTUI) commitPopupSelection(sel *SlashCommand) (tea.Cmd, bool) {
	m.completion.Close()
	if sel != nil && sel.Source == "skill" {
		m.input.SetValue("/" + sel.Name + " ")
		return nil, false
	}
	if sel == nil {
		return nil, false
	}
	return m.tuiDispatchCommand("/" + sel.Name)
}

// Render produces the popup's candidate lines (one per row), or "" when the
// popup is closed or has no candidates. Markers: ● implemented built-in,
// ◇ skill (placeholder — selecting only commits the name, activation on the
// user's confirm Enter), ○ recognized-but-unimplemented stub.
func (p *completionPopup) Render(width int) string {
	if !p.active || len(p.items) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range p.items {
		marker := "●"
		switch {
		case !c.Implemented:
			marker = "○"
		case c.Source == "skill":
			marker = "◇"
		}
		line := fmt.Sprintf("  %s /%s  %s", marker, c.Name, c.Description)
		line = truncateToWidth(line, width)
		if i == p.selected {
			line = selectionRow(line)
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// selectionRow renders a row with the selection color as background (reverse
// of the normal foreground accent).
func selectionRow(s string) string {
	return fmt.Sprintf("\033[48;5;%dm\033[38;5;%dm %s \033[0m",
		activeTheme.selection.xterm, activeTheme.userBG.xterm, s)
}

// truncateToWidth truncates s to at most width visible runes (approximate:
// ANSI codes are stripped for the count).
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	runes := []rune(stripAnsi(s))
	if len(runes) <= width {
		return s
	}
	if width < 3 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
