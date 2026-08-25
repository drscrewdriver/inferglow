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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// cliColor holds a hex color and its xterm-256 fallback.
type cliColor struct {
	hex   string
	xterm int
}

// cliPalette defines the full color palette for the TUI.
type cliPalette struct {
	accent    cliColor
	muted     cliColor
	subtle    cliColor
	subtle2   cliColor
	success   cliColor
	warn      cliColor
	err       cliColor
	info      cliColor
	border    cliColor
	selection cliColor
	userBG    cliColor
	userFG    cliColor
	reasoning cliColor
}

var (
	// darkTheme is the default dark-mode palette (inspired by Reasonix graphite).
	darkTheme = cliPalette{
		accent:    cliColor{"#d97757", 173},
		muted:     cliColor{"#c0c4cc", 251},
		subtle:    cliColor{"#a4a9b3", 248},
		subtle2:   cliColor{"#858b96", 245},
		success:   cliColor{"#74b87a", 108},
		warn:      cliColor{"#d9a441", 179},
		err:       cliColor{"#e0696a", 167},
		info:      cliColor{"#56b6c2", 80},
		border:    cliColor{"#343945", 237},
		selection: cliColor{"#d97757", 173},
		userBG:    cliColor{"#222631", 235},
		userFG:    cliColor{"#e8e8e8", 255},
		reasoning: cliColor{"#858b96", 245},
	}

	// lightTheme is the light-mode palette.
	lightTheme = cliPalette{
		accent:    cliColor{"#b3552f", 131},
		muted:     cliColor{"#3d4149", 240},
		subtle:    cliColor{"#5c6069", 242},
		subtle2:   cliColor{"#7a7f88", 244},
		success:   cliColor{"#2f7d38", 64},
		warn:      cliColor{"#a6781f", 136},
		err:       cliColor{"#c0392b", 124},
		info:      cliColor{"#1d7f9c", 31},
		border:    cliColor{"#c9ccd2", 251},
		selection: cliColor{"#b3552f", 131},
		userBG:    cliColor{"#e6e7ea", 254},
		userFG:    cliColor{"#1f2329", 235},
		reasoning: cliColor{"#7a7f88", 244},
	}

	// activeTheme is the runtime theme (mutable via /theme).
	activeTheme = &darkTheme

	// Pre-built lipgloss styles for common elements.
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, true, false).
			BorderForeground(lipgloss.Color(darkTheme.accent.hex)).
			PaddingLeft(1)

	statusBlockStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(darkTheme.subtle2.hex))
	workingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(darkTheme.subtle2.hex))
)

// themeFg renders text with a specific palette color using xterm-256.
func themeFg(c cliColor, s string) string {
	if trueColorTerminal() {
		r, g, b := hexToRGB(c.hex)
		return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, s)
	}
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c.xterm, s)
}

// dim renders text in the subtle color.
func dim(s string) string { return themeFg(activeTheme.subtle, s) }

// accent renders text in the accent color.
func accent(s string) string { return themeFg(activeTheme.accent, s) }

// errorText renders text in the error color.
func errorText(s string) string { return themeFg(activeTheme.err, s) }

// successText renders text in the success color.
func successText(s string) string { return themeFg(activeTheme.success, s) }

// infoText renders text in the info color.
func infoText(s string) string { return themeFg(activeTheme.info, s) }

// warnText renders text in the warning color.
func warnText(s string) string { return themeFg(activeTheme.warn, s) }

// userBubble renders text with the user message background.
func userBubble(s string) string {
	return fmt.Sprintf("\033[48;5;%dm\033[38;5;%dm %s \033[0m",
		activeTheme.userBG.xterm, activeTheme.userFG.xterm, s)
}

// reasoningDim renders text in the reasoning/dim style.
func reasoningDim(s string) string { return themeFg(activeTheme.reasoning, s) }

// approvalText renders text in the approval/warning style (amber).
func approvalText(s string) string { return themeFg(activeTheme.warn, s) }

// sandboxBadge renders a sandbox mode badge.
func sandboxBadge(mode string) string {
	if mode == "" {
		return ""
	}
	return infoText("[" + mode + "]")
}

// sideEffectBadge renders a side-effect level badge with color coding.
func sideEffectBadge(level string) string {
	switch level {
	case "read":
		return successText("[read]")
	case "write":
		return warnText("[write]")
	case "exec":
		return errorText("[exec]")
	case "network":
		return infoText("[network]")
	default:
		return dim("[" + level + "]")
	}
}

// compressionNote renders a compression event note.
func compressionNote(s string) string { return infoText("⟳ " + s) }

// receiptDim renders a turn receipt line.
func receiptDim(s string) string { return dim("─── " + s + " ───") }

// visibleWidth returns the visible width of a string (stripping ANSI codes).
func visibleWidth(s string) int {
	return lipgloss.Width(stripAnsi(s))
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// compactMiddle truncates a string in the middle, keeping start and end.
func compactMiddle(s string, maxW int) string {
	if maxW <= 0 || visibleWidth(s) <= maxW {
		return s
	}
	if maxW < 5 {
		return s[:maxW]
	}
	keep := (maxW - 3) / 2
	return s[:keep] + "..." + s[len(s)-keep:]
}

// trueColorTerminal checks if the terminal supports true color.
func trueColorTerminal() bool {
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	return ct == "truecolor" || ct == "24bit"
}

// hexToRGB converts a hex color string to (r, g, b).
func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return int(r), int(g), int(b)
}

// ---------------------------------------------------------------------------
// RF-3: /theme real switching + persistence (~/.inferglow/theme.json).
// ---------------------------------------------------------------------------

const themePrefFile = "theme.json"

// themePref is the persisted theme preference.
type themePref struct {
	Theme string `json:"theme"`
}

// themeNames lists the supported themes ("" = auto).
var themeNames = []string{"dark", "light", "auto"}

// applyTheme switches activeTheme to the named theme (dark/light/auto).
// auto resolves to dark when the terminal background cannot be detected.
// Returns an error for unknown names. NOTE: does not persist — callers
// (tuiHandleTheme) persist via writeThemePref.
func applyTheme(name string) error {
	switch name {
	case "dark", "":
		activeTheme = &darkTheme
	case "light":
		activeTheme = &lightTheme
	case "auto":
		if termIsLight() {
			activeTheme = &lightTheme
		} else {
			activeTheme = &darkTheme
		}
	default:
		return fmt.Errorf("unknown theme: %s (available: dark, light, auto)", name)
	}
	return nil
}

// termIsLight heuristically detects a light terminal background.
// COLORFGBG (used by many terminals) reports "fg;bg" where bg=15 means light.
func termIsLight() bool {
	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if len(parts) >= 2 && strings.TrimSpace(parts[1]) == "15" {
			return true
		}
	}
	return false
}

// readThemePref loads the persisted theme preference ("" when unset/corrupt).
func readThemePref() string {
	return readThemePrefFrom(filepath.Join(prefsDir(), themePrefFile))
}

func readThemePrefFrom(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var p themePref
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.Theme
}

// writeThemePref persists the theme preference. Failures are silent.
func writeThemePref(name string) {
	writeThemePrefTo(filepath.Join(prefsDir(), themePrefFile), name)
}

func writeThemePrefTo(path, name string) {
	data, err := json.Marshal(themePref{Theme: name})
	if err != nil {
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(path, data, 0o644)
}

// tuiHandleTheme handles /theme (RF-3):
//
//	/theme        → list themes + current
//	/theme <name> → switch (dark|light|auto) + persist + redraw
func tuiHandleTheme(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	m.commitLine("")
	if args == "" {
		cur := activeThemeName()
		m.commitLine(accent("Theme: " + cur))
		for _, name := range themeNames {
			marker := "  "
			if name == cur {
				marker = "→ "
			}
			m.commitLine(dim(marker + name))
		}
		m.commitLine(dim("  Usage: /theme <dark|light|auto>"))
		return nil, false
	}
	if err := applyTheme(args); err != nil {
		m.commitLine(errorText("  ✗ " + err.Error()))
		return nil, false
	}
	writeThemePref(args) // persist only on explicit /theme (not on startup restore)
	applyTextareaTheme(&m.input)
	m.transcriptDirty = true
	m.commitLine(successText("  ✓ 主题已切换为 " + args))
	return nil, false
}

// activeThemeName returns the name of the currently active theme.
func activeThemeName() string {
	if activeTheme == &lightTheme {
		return "light"
	}
	return "dark"
}
