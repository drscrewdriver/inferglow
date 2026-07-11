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
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// tuiDispatchCommand handles slash commands within the TUI.
// Returns a tea.Cmd if the command produces one, and whether to quit.
func (m *chatTUI) tuiDispatchCommand(input string) (cmd tea.Cmd, quit bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, false
	}
	cmdName := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	switch cmdName {
	case "quit", "exit":
		return tea.Quit, true

	case "help":
		m.commitLine("")
		m.commitLine(accent("Available commands:"))
		m.commitLine(dim("  /help              Show this help message"))
		m.commitLine(dim("  /memory search <q> Search memory for a query"))
		m.commitLine(dim("  /memory stats      Show memory statistics"))
		m.commitLine(dim("  /compact           Manually trigger context compression"))
		m.commitLine(dim("  /clear             Clear the transcript"))
		m.commitLine(dim("  /verbose           Toggle reasoning display"))
		m.commitLine(dim("  /receipt           Show last turn receipt"))
		m.commitLine(dim("  /session           Show current session ID"))
		m.commitLine(dim("  /resume [id]       List/resume previous sessions"))
		m.commitLine(dim("  /sandbox [mode]    Show/switch sandbox mode"))
		m.commitLine(dim("  /config            Show config path and settings"))
		m.commitLine(dim("  /quit              End session and exit"))
		return nil, false

	case "clear":
		m.transcript = nil
		m.transcriptDirty = true
		return nil, false

	case "verbose":
		m.showReasoning = !m.showReasoning
		state := "off"
		if m.showReasoning {
			state = "on"
		}
		m.commitLine("")
		m.commitLine(dim("Reasoning display: " + state))
		return nil, false

	case "compact":
		m.commitLine("")
		m.commitLine(dim("Triggering compression…"))
		go func() {
			err := m.bridge.Compact(context.Background())
			if err != nil {
				m.commitLine(errorText(fmt.Sprintf("Compression error: %v", err)))
			} else {
				m.commitLine(successText("Compression complete."))
			}
			m.transcriptDirty = true
		}()
		return nil, false

	case "memory":
		m.tuiHandleMemory(args)
		return nil, false

	case "receipt":
		m.commitLine("")
		m.commitReceipt(fmt.Sprintf("Turn · %ds · %d rounds · %d tools",
			m.receipt.duration, m.receipt.llmRounds, m.receipt.toolCalls))
		return nil, false

	case "session":
		m.tuiHandleSession(args)
		return nil, false

	case "resume":
		m.tuiHandleResume(args)
		return nil, false

	case "sandbox":
		m.tuiHandleSandbox(args)
		return nil, false

	case "config":
		m.tuiHandleConfig(args)
		return nil, false

	default:
		m.commitLine("")
		m.commitLine(errorText(fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName)))
		return nil, false
	}
}

// tuiHandleMemory handles /memory subcommands in the TUI.
func (m *chatTUI) tuiHandleMemory(args string) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		m.commitLine("")
		m.commitLine(dim("Usage: /memory search <query> | /memory stats"))
		return
	}

	switch parts[0] {
	case "stats":
		stats := m.bridge.Stats()
		m.commitLine("")
		m.commitLine(accent("Memory stats:"))
		m.commitLine(dim(fmt.Sprintf("  Total steps:   %d", stats.TotalSteps)))
		m.commitLine(dim(fmt.Sprintf("  Active steps:  %d", stats.ActiveSteps)))
		m.commitLine(dim(fmt.Sprintf("  Total tokens:  %d", stats.TotalTokens)))

	case "search":
		if len(parts) < 2 || parts[1] == "" {
			m.commitLine("")
			m.commitLine(dim("Usage: /memory search <query>"))
			return
		}
		results, err := m.bridge.SearchMemory(context.Background(), parts[1])
		if err != nil {
			m.commitLine("")
			m.commitLine(errorText(fmt.Sprintf("Memory search error: %v", err)))
			return
		}
		m.commitLine("")
		if len(results) == 0 {
			m.commitLine(dim("No memories found."))
			return
		}
		m.commitLine(accent(fmt.Sprintf("Found %d memories:", len(results))))
		for _, r := range results {
			facts := strings.Join(r.Facts, "; ")
			m.commitLine(dim(fmt.Sprintf("  [%s] (%s, %.2f) %s",
				r.MemID, r.Category, r.Confidence, facts)))
		}

	default:
		m.commitLine("")
		m.commitLine(errorText(fmt.Sprintf("Unknown memory subcommand: %s", parts[0])))
	}
}
