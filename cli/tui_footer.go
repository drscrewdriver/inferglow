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

const (
	statusFooterIndent   = "  "
	statusFooterGroupGap = 2
)

// renderStatusBar renders the complete status bar area at the bottom.
// Note: the working spinner line is rendered separately in View() above the
// status bar, so this function only produces the persistent status row(s).
func (m *chatTUI) renderStatusBar() string {
	primary := m.primaryStatusLine()
	model := m.statusModelGroup()
	return layoutStatusSides(primary, model, m.width)
}

// primaryStatusLine renders the left side: mode tag + interaction state.
func (m *chatTUI) primaryStatusLine() string {
	modeTag := accent("Chat")
	status := statusFooterIndent + modeTag
	switch {
	case m.state == tuiRunning:
		status += " · " + infoText("running")
	default:
		status += " · " + footerValue("Idle")
	}
	// RF-10: API health indicator (● online / ○ offline).
	if h := m.renderHealth(); h != "" {
		status += " · " + h
	}
	// SC-5: workspace indicator in the status bar.
	if m.cfg.Features.WorkspaceSwitch {
		if ws := m.workspace.RenderStatus(); ws != "" {
			status += " · " + footerHint(ws)
		}
	}
	status += " · " + footerHint("Ctrl+C quit")
	return status
}

// statusModelGroup renders the right side: model route + effort + tps +
// cache + context info.
func (m *chatTUI) statusModelGroup() string {
	var parts []string
	if m.modelLabel != "" {
		parts = append(parts, footerMetric("model:", footerInfo(m.modelLabel)))
	}
	// RF-2: effort level in the status bar.
	if m.cfg.Features.EffortControl {
		parts = append(parts, footerMetric("effort:", footerInfo(m.effortStatus())))
	}
	// RF-7: live TPS gauge while streaming, history sparkline when idle.
	if m.state == tuiRunning {
		if tps := m.renderLiveTPS(); tps != "" {
			parts = append(parts, tps)
		}
	} else if tps := m.renderHistoryTPS(); tps != "" {
		parts = append(parts, tps)
	}
	// RF-8: cache hit rate.
	if cache := m.renderCacheHit(); cache != "" {
		parts = append(parts, cache)
	}
	// Context usage.
	stats := m.bridge.Stats()
	if stats.TotalTokens > 0 {
		window := m.cfg.WindowTokens
		if window <= 0 {
			window = 32000
		}
		pct := stats.TotalTokens * 100 / window
		ctxValue := fmt.Sprintf("%s / %s (%d%%)",
			shortTokens(stats.TotalTokens), shortTokens(window), pct)
		parts = append(parts, footerMetric("ctx:", ctxValue))
	}
	// RF-4: narrow terminals (<100 cols) degrade to model · effort only.
	if m.width > 0 && m.width < 100 {
		parts = parts[:min(len(parts), 2)]
	}
	return strings.Join(parts, "  ")
}

// layoutStatusSides places left and right status groups, handling overflow.
func layoutStatusSides(left, right string, width int) string {
	if right == "" {
		return left
	}
	if left == "" {
		return rightAlign(right, width)
	}
	lw := visibleWidth(left)
	rw := visibleWidth(right)
	if lw+statusFooterGroupGap+rw <= width {
		return left + strings.Repeat(" ", width-lw-rw) + right
	}
	// Wrap: left on first line, right on second.
	return left + "\n" + statusFooterIndent + right
}

// rightAlign pads the left side with spaces to right-align text.
func rightAlign(s string, width int) string {
	sw := visibleWidth(s)
	if sw >= width {
		return s
	}
	return strings.Repeat(" ", width-sw) + s
}

// footerValue renders a footer value in muted color.
func footerValue(s string) string {
	return themeFg(activeTheme.muted, s)
}

// footerHint renders a footer hint in subtle color.
func footerHint(s string) string {
	return themeFg(activeTheme.subtle, s)
}

// footerInfo renders a footer info value in info color.
func footerInfo(s string) string {
	return themeFg(activeTheme.info, s)
}

// footerMetric renders a label+value pair.
func footerMetric(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return dim(label) + " " + value
}

// shortTokens formats a token count in human-readable form.
func shortTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
