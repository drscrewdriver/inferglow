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
	// maxTranscriptBlocks is the upper bound for transcript entries.
	// Beyond this, oldest blocks are evicted to prevent unbounded memory growth.
	maxTranscriptBlocks = 5000
)

// blockKind identifies the type of transcript block.
type blockKind int

const (
	blockText blockKind = iota
	blockUser
	blockAssistant
	blockTool
	blockError
	blockApproval
	blockReceipt
	blockSystem
)

// transcriptBlock is a typed transcript entry with both rendered and source text.
type transcriptBlock struct {
	Kind   blockKind
	Raw    string // rendered ANSI string (current behavior)
	Source string // plain text source (for copy/search/reflow)
}

// commitLine appends a raw text line to the transcript and marks it dirty.
func (m *chatTUI) commitLine(text string) {
	m.commitBlock(blockText, text, stripAnsi(text))
}

// commitBlock appends a typed block to the transcript.
func (m *chatTUI) commitBlock(kind blockKind, raw, source string) {
	m.transcript = append(m.transcript, transcriptBlock{Kind: kind, Raw: raw, Source: source})
	m.transcriptDirty = true
	if len(m.transcript) > maxTranscriptBlocks {
		excess := len(m.transcript) - maxTranscriptBlocks
		m.transcript = m.transcript[excess:]
	}
}

// commitBlockRaw appends a pre-rendered line (backward-compatible wrapper).
func (m *chatTUI) commitBlockRaw(raw string) {
	m.commitBlock(blockText, raw, stripAnsi(raw))
}

// commitUserBubble renders a user message as a visually distinct bubble.
func (m *chatTUI) commitUserBubble(text string) {
	m.commitBlock(blockText, "", "")
	m.commitBlock(blockUser, userBubble("› "+text), text)
}

// commitToolCard renders a tool invocation as a styled card.
func (m *chatTUI) commitToolCard(name, status, output string) {
	m.commitToolCardEx(name, status, "", "", output)
}

// commitToolCardEx renders a tool card with sandbox/side-effect badges.
func (m *chatTUI) commitToolCardEx(name, status, sandboxMode, sideEffect, output string) {
	var icon string
	var kind blockKind
	switch status {
	case "running":
		icon = "⎿"
		kind = blockTool
	case "done":
		icon = "✓"
		kind = blockTool
	case "error":
		icon = "✗"
		kind = blockError
	case "blocked":
		icon = "⚠"
		kind = blockApproval
	default:
		icon = "·"
		kind = blockTool
	}
	line := fmt.Sprintf("%s [%s] %s", icon, name, status)
	// Append sandbox/side-effect badges.
	var badges string
	if sandboxMode != "" {
		badges += " │ " + sandboxBadge(sandboxMode)
	}
	if sideEffect != "" {
		badges += " " + sideEffectBadge(sideEffect)
	}
	if status == "blocked" {
		badges += " " + approvalText("[Y/N]")
	}
	line += badges
	if status == "running" {
		line = infoText(line)
	} else if status == "error" {
		line = errorText(line)
	} else if status == "blocked" {
		line = approvalText(line)
	} else {
		line = dim(line)
	}
	source := fmt.Sprintf("[%s] %s%s", name, status, badges)
	m.commitBlock(kind, line, stripAnsi(source))
	if output != "" {
		for _, l := range strings.Split(output, "\n") {
			m.commitBlock(blockText, dim("  "+l), "  "+l)
		}
	}
}

// commitApprovalCard renders an approval request card.
func (m *chatTUI) commitApprovalCard(toolName, recordID string) {
	raw := approvalText(fmt.Sprintf(
		"┌─ ⚠ Approval Required ─────────────────┐\n"+
			"│ Tool: %-30s │\n"+
			"│ [Y] Approve  [N] Deny                  │\n"+
			"└────────────────────────────────────────┘", toolName))
	source := fmt.Sprintf("Approval required for tool: %s (record: %s)", toolName, recordID)
	m.commitBlock(blockApproval, raw, source)
}

// commitReceipt renders a turn receipt line.
func (m *chatTUI) commitReceipt(summary string) {
	m.commitBlock(blockReceipt, receiptDim(summary), summary)
}

// commitSystemNote renders a system note line.
func (m *chatTUI) commitSystemNote(text string) {
	m.commitBlock(blockSystem, dim(text), text)
}

// renderTranscript assembles the full viewport content from transcript blocks.
// rich mode renders each block's ANSI/markdown Raw; raw mode renders the plain
// Source (falling back to the stripped Raw when no Source is stored).
func (m *chatTUI) renderTranscript() string {
	parts := make([]string, len(m.transcript))
	for i, b := range m.transcript {
		raw := b.Raw
		// SC-4: highlight the user message selected in message-action mode.
		if m.messageActions.Active() && b.Kind == blockUser && m.messageActions.isSelected(i) {
			raw = selectionRow(stripAnsi(raw))
		}
		if m.renderRaw {
			src := b.Source
			if src == "" {
				src = stripAnsi(b.Raw)
			}
			parts[i] = src
		} else {
			parts[i] = raw
		}
	}
	return strings.Join(parts, "\n")
}

