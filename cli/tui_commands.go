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
	"os"
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

	// OT-14: try registry first, fallback to legacy switch.
	if m.cmdRegistry != nil {
		if c, q, found := m.cmdRegistry.Dispatch(m, cmdName, args); found {
			// SC-1: recognized-but-unimplemented commands get a friendly hint
			// instead of an "unknown command" error.
			if !m.cmdRegistry.IsImplemented(cmdName) {
				m.commitLine("")
				msg := fmt.Sprintf("  /%s 已识别", cmdName)
				if src := m.cmdRegistry.SourceOf(cmdName); src != "" {
					msg += fmt.Sprintf("（来自 %s）", src)
				}
				msg += "，但本版本未实现。"
				m.commitLine(warnText(msg))
				return nil, false
			}
			return c, q
		}
	}
	return m.dispatchLegacy(cmdName, args)
}

// dispatchLegacy runs the legacy slash-command switch (SC-1). Registry
// entries that mirror legacy commands (registerLegacyMirrors) dispatch here,
// so native semantics stay unchanged while the commands surface in the "/"
// popup and Tab completion.
func (m *chatTUI) dispatchLegacy(cmdName, args string) (cmd tea.Cmd, quit bool) {
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
		m.commitLine(dim("  /async-compress    Force async compression (bypass sweet-spot)"))
		m.commitLine(dim("  /clear             Clear the transcript"))
		m.commitLine(dim("  /verbose           Toggle reasoning display"))
		m.commitLine(dim("  /receipt           Show last turn receipt"))
		m.commitLine(dim("  /session           Show current session ID"))
		m.commitLine(dim("  /resume [id]       List/resume previous sessions"))
		m.commitLine(dim("  /vision <img> [q] Attach an image and ask the vision model about it"))
		m.commitLine(dim("  /sandbox [mode]    Show/switch sandbox mode"))
		m.commitLine(dim("  /config            Show config path and settings"))
		m.commitLine(dim("  /showbackground    Show current project background context"))
		m.commitLine(dim("  /rebackground      Have AI analyze and rewrite project background"))
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

	case "async-compress":
		m.tuiHandleAsyncCompress(args)
		return nil, false

	case "memory":
		m.tuiHandleMemory(args)
		return nil, false

	case "receipt":
		m.commitLine("")
		m.commitReceipt(fmt.Sprintf("Turn · %ds · %d rounds · %d tools",
			m.receipt.duration, m.receipt.llmRounds, m.receipt.toolCalls))
		// RF-6/8: extended per-turn stats.
		for _, ln := range m.renderReceiptDetail() {
			m.commitLine(dim("  " + ln))
		}
		return nil, false

	case "session":
		m.tuiHandleSession(args)
		return nil, false

	case "resume":
		if m.tuiHandleResume(args) {
			// Truthy: session file exists; quit so RunTUI relaunches with it.
			return tea.Quit, true
		}
		return nil, false

	case "vision", "see":
		// B4/B5: vision bridge / read-image agent. Format: <path> [question]
		fields := strings.Fields(args)
		if len(fields) == 0 {
			m.commitLine(dim("Usage: /vision <image-path> [question]"))
			return nil, false
		}
		m.runVision(fields[0], strings.TrimSpace(strings.TrimPrefix(args, fields[0])))
		return nil, false

	case "sandbox":
		m.tuiHandleSandbox(args)
		return nil, false

	case "config":
		m.tuiHandleConfig(args)
		return nil, false

	case "showbackground":
		m.tuiHandleShowBackground(args)
		return nil, false

	case "rebackground":
		m.tuiHandleRebackground(args)
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

// tuiHandleAsyncCompress triggers forced async compression,
// bypassing the sweet-spot threshold check.
func (m *chatTUI) tuiHandleAsyncCompress(args string) {
	m.commitLine("")
	m.commitLine(accent("Async compression started…"))
	m.commitLine(dim("  Forcing compression regardless of sweet-spot threshold."))

	go func() {
		result, err := m.bridge.ForceAsyncCompress(context.Background())
		if err != nil {
			m.commitLine(errorText(fmt.Sprintf("Async compress error: %v", err)))
			return
		}
		m.commitLine("")
		m.commitLine(successText(fmt.Sprintf("Async compression complete: %d steps compressed.",
			result.StepsCompressed)))
		if result.TokensSaved > 0 {
			m.commitLine(dim(fmt.Sprintf("  Estimated tokens saved: ~%d", result.TokensSaved)))
		}
		m.transcriptDirty = true
	}()
}

// buildSlashRegistry creates the OT-14 command registry with new commands
// that are not in the legacy switch. Legacy commands remain in the switch
// and are migrated incrementally.
func buildSlashRegistry(cfg CLIConfig) *SlashRegistry {
	r := NewSlashRegistry()

	// MC-3: /mode command for runtime context mode switching.
	if cfg.Features.RuntimeModeSwitch {
		r.Register(&SlashCommand{
			Name:        "mode",
			Description: "Show or switch context management mode",
			Usage:       "[hybrid|passthrough|three_zone|summary]",
			Handler:     tuiHandleMode,
		})
	}

	// SC-3: /tasks toggles the right-side task panel.
	if cfg.Features.TaskPanel {
		r.Register(&SlashCommand{
			Name:        "tasks",
			Description: "Toggle the task panel",
			Handler:     tuiHandleTasks,
		})
	}

	// SC-5: /workspace and /cd switch the working directory.
	if cfg.Features.WorkspaceSwitch {
		ws := &SlashCommand{
			Name:        "workspace",
			Description: "Show or switch the working directory",
			Usage:       "[path|list|~|-]",
			Handler:     tuiHandleWorkspace,
		}
		cd := *ws
		cd.Name = "cd"
		cd.Description = "Show or switch the working directory (alias of /workspace)"
		r.Register(ws)
		r.Register(&cd)
	}

	// RF-1: /model runtime multi-provider/model switching.
	if cfg.Features.ModelSwitch {
		r.Register(&SlashCommand{
			Name:        "model",
			Aliases:     []string{"models", "scoped-models"},
			Description: "Switch model / provider at runtime",
			Usage:       "[<provider> [<model>]|status|recents]",
			Handler:     tuiHandleModel,
		})
	}

	// RF-2: /effort reasoning-level control.
	if cfg.Features.EffortControl {
		r.Register(&SlashCommand{
			Name:        "effort",
			Description: "Adjust reasoning effort level",
			Usage:       "[low|medium|high|auto|status]",
			Handler:     tuiHandleEffort,
		})
	}

	// RF-3: /theme real theme switching.
	if cfg.Features.ThemeSwitch {
		r.Register(&SlashCommand{
			Name:        "theme",
			Description: "Switch UI theme",
			Usage:       "[dark|light|auto]",
			Handler:     tuiHandleTheme,
		})
	}

	// RF-9: /tips view usage tips; /welcome re-show the welcome page.
	if cfg.Features.Welcome {
		r.Register(&SlashCommand{
			Name:        "tips",
			Description: "Show usage tips (group filter)",
			Usage:       "[keys|commands|workflow|display|pitfalls]",
			Handler:     tuiHandleTips,
		})
		r.Register(&SlashCommand{
			Name:        "welcome",
			Description: "Re-show the welcome page",
			Handler:     tuiHandleWelcome,
		})
	}

	// RF-7: /tps output-efficiency stats.
	if cfg.Features.TPS {
		r.Register(&SlashCommand{
			Name:        "tps",
			Description: "Show tokens/sec output statistics",
			Handler:     tuiHandleTPS,
		})
	}

	// RF-10: /health API health check.
	if cfg.Features.HealthCheck {
		r.Register(&SlashCommand{
			Name:        "health",
			Description: "Probe API provider health",
			Usage:       "[<key>|interval <sec>]",
			Handler:     tuiHandleHealth,
		})
	}

	// SC-2: mirror the legacy switch commands into the registry so they
	// surface in Tab completion and the "/" popup. Dispatch still goes
	// through the original legacy handlers (native semantics unchanged).
	registerLegacyMirrors(r)

	// SC-1: overlay the compat catalog (claude/pi/opencode/codex aliases and
	// recognized-but-unimplemented stubs). Native commands are never
	// overridden — colliding names are skipped by RegisterOverlay.
	registerCompatCommands(r, cfg)

	return r
}

// legacyCommand describes a legacy-switch command mirrored into the registry.
type legacyCommand struct {
	name        string
	description string
	usage       string
}

// legacySlashCommands lists every legacy switch command (tuiDispatchCommand's
// dispatchLegacy) with its help metadata.
var legacySlashCommands = []legacyCommand{
	{"help", "Show this help message", ""},
	{"clear", "Clear the transcript", ""},
	{"verbose", "Toggle reasoning display", ""},
	{"compact", "Manually trigger context compression", ""},
	{"async-compress", "Force async compression (bypass sweet-spot)", ""},
	{"memory", "Search memory / show stats", "search <q> | stats"},
	{"receipt", "Show last turn receipt", ""},
	{"session", "Show current session ID", ""},
	{"resume", "List/resume previous sessions", "[id]"},
	{"vision", "Attach an image and ask the vision model", "<img> [q]"},
	{"see", "Attach an image and ask the vision model", "<img> [q]"},
	{"sandbox", "Show/switch sandbox mode", "[mode]"},
	{"config", "Show config path and settings", "[save]"},
	{"showbackground", "Show current project background context", ""},
	{"rebackground", "Have AI analyze and rewrite project background", ""},
	{"quit", "End session and exit", ""},
	{"exit", "End session and exit", ""},
}

// registerLegacyMirrors overlays registry entries for the legacy switch
// commands. Collisions (e.g. a native registry command with the same name)
// are skipped by RegisterOverlay.
func registerLegacyMirrors(r *SlashRegistry) {
	for _, l := range legacySlashCommands {
		name := l.name
		r.RegisterOverlay(&SlashCommand{
			Name:        name,
			Description: l.description,
			Usage:       l.usage,
			Source:      "inferglow",
			Implemented: true,
			Handler: func(m *chatTUI, args string) (tea.Cmd, bool) {
				return m.dispatchLegacy(name, args)
			},
		})
	}
}

// tuiHandleTasks toggles the right-side task panel (SC-3).
func tuiHandleTasks(m *chatTUI, args string) (tea.Cmd, bool) {
	m.taskPanel.Toggle()
	m.taskPanelUserSet = true
	if m.taskPanel.Active() {
		m.taskPanel.Sync(m.bridge.TaskStore())
		m.commitSystemNote(dim("Task panel shown."))
	} else {
		m.commitSystemNote(dim("Task panel hidden."))
	}
	m.transcriptDirty = true
	return nil, false
}

// tuiHandleWorkspace handles /workspace and /cd (SC-5):
//   - no args        → show current directory + usage
//   - <path>         → switch to the given directory
//   - list           → show the recent workspace history
//   - ~              → switch to the user home directory
//   - -              → switch back to the previous workspace
func tuiHandleWorkspace(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	m.commitLine("")
	switch {
	case args == "" || args == "show":
		m.commitLine(accent("Workspace:"))
		m.commitLine(dim("  Current: ") + footerInfo(m.workspace.GetCurrentDir()))
		m.commitLine(dim("  Usage: /workspace <path> | list | ~ | -"))

	case args == "list":
		hist := m.workspace.GetHistory()
		if len(hist) == 0 {
			m.commitLine(dim("  No workspace history yet."))
			return nil, false
		}
		m.commitLine(accent("Recent workspaces:"))
		cur := m.workspace.GetCurrentDir()
		for _, p := range hist {
			marker := "  "
			if p == cur {
				marker = "→ "
			}
			m.commitLine(dim(marker + p))
		}

	case args == "-":
		if err := m.workspace.SwitchToPrevious(); err != nil {
			m.commitLine(warnText("  No previous workspace."))
			return nil, false
		}
		m.commitLine(successText("  ✓ 已切换到 " + m.workspace.GetCurrentDir()))
		m.transcriptDirty = true

	case args == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			m.commitLine(errorText("  ✗ 无法获取用户主目录: " + err.Error()))
			return nil, false
		}
		m.workspaceSwitchTo(home)

	default:
		m.workspaceSwitchTo(args)
	}
	return nil, false
}

// workspaceSwitchTo attempts to switch the working directory and reports the
// result in the transcript.
func (m *chatTUI) workspaceSwitchTo(path string) {
	if err := m.workspace.SetCurrentDir(path); err != nil {
		m.commitLine(errorText(fmt.Sprintf("  ✗ 无法切换到 %s: %v", path, err)))
		return
	}
	m.commitLine(successText("  ✓ 已切换到 " + m.workspace.GetCurrentDir()))
	m.transcriptDirty = true
}

// tuiHandleMode handles the /mode slash command (MC-3).
func tuiHandleMode(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	if args == "" {
		// Show current mode.
		current := m.bridge.CurrentMode()
		m.commitLine("")
		m.commitLine(accent("Context mode: " + current))
		m.commitLine(dim("  Available: hybrid, passthrough, three_zone, summary"))
		m.commitLine(dim("  Usage: /mode <mode>"))
		return nil, false
	}

	// Validate mode.
	valid := map[string]bool{"hybrid": true, "passthrough": true, "three_zone": true, "summary": true}
	if !valid[args] {
		m.commitLine("")
		m.commitLine(errorText(fmt.Sprintf("Unknown mode: %s. Available: hybrid, passthrough, three_zone, summary", args)))
		return nil, false
	}

	// Switch mode.
	if err := m.bridge.SwitchMode(args); err != nil {
		m.commitLine("")
		m.commitLine(errorText(fmt.Sprintf("Mode switch failed: %v", err)))
		return nil, false
	}

	m.commitLine("")
	m.commitLine(successText("Context mode switched to: " + args))
	m.transcriptDirty = true
	return nil, false
}
