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

// compatEntry is one row of the external slash-command catalog (SC-1).
// A compat command either maps onto an existing native command (mapTo), runs
// its own handler, or is registered as a recognized-but-unimplemented stub.
type compatEntry struct {
	name        string
	aliases     []string
	description string
	source      string // "claude" | "pi" | "opencode" | "codex" | "compat"
	mapTo       string // native command name to re-dispatch ("" = none)
	handler     func(m *chatTUI, args string) (tea.Cmd, bool)
}

// compatCatalog is the static compat command directory. Names/aliases that
// collide with native inferglow commands are skipped at registration time by
// RegisterOverlay — native commands are never overridden.
var compatCatalog = []compatEntry{
	// --- Implemented: mapped onto native handlers -------------------------
	{name: "reset", description: "Clear the transcript (claude/pi/opencode)", source: "compat", mapTo: "clear"},
	{name: "new", description: "Clear the transcript (claude/pi/opencode)", source: "compat", mapTo: "clear"},
	{name: "summarize", description: "Compress the conversation context (opencode)", source: "compat", mapTo: "compact"},
	{name: "continue", description: "Resume the previous session (claude/opencode)", source: "compat", mapTo: "resume"},
	{name: "sessions", description: "List/resume previous sessions (opencode)", source: "compat", mapTo: "resume"},
	{name: "settings", description: "Show configuration (pi/opencode)", source: "compat", mapTo: "config"},
	{name: "title", description: "Show current session info (codex)", source: "compat", mapTo: "session"},
	{name: "status", description: "Show usage/receipt (claude/codex)", source: "compat", mapTo: "receipt"},
	{name: "usage", description: "Show usage/receipt (opencode/pi)", source: "compat", mapTo: "receipt"},
	{name: "cost", description: "Show usage/receipt (claude)", source: "compat", mapTo: "receipt"},
	{name: "hotkeys", description: "Show help (pi)", source: "compat", mapTo: "help"},
	{name: "keybindings", description: "Show help (claude/codex)", source: "compat", mapTo: "help"},
	{name: "q", description: "Quit (opencode)", source: "compat", mapTo: "quit"},
	{name: "logout", description: "Quit (pi/codex)", source: "compat", mapTo: "quit"},
	{name: "cd", description: "Switch workspace directory (codex)", source: "compat", mapTo: "workspace"},
	{name: "pwd", description: "Show current workspace directory (codex)", source: "compat", mapTo: "workspace"},
	{name: "model", aliases: []string{"models", "scoped-models"}, description: "Show model configuration (pi/opencode/codex)", source: "compat", handler: tuiHandleModelCompat},
	{name: "thinking", description: "Toggle extended thinking (opencode)", source: "opencode", handler: tuiHandleEffort},
	{name: "theme", description: "Theme switching (pi/opencode/codex)", source: "compat", handler: tuiHandleTheme},

	// --- Recognized but not implemented (friendly hint only) --------------
	{name: "undo", description: "Undo last message (opencode)", source: "compat"},
	{name: "redo", description: "Redo last message (opencode)", source: "compat"},
	{name: "rewind", description: "Rewind to a checkpoint (claude)", source: "claude"},
	{name: "checkpoint", description: "Rewind to a checkpoint (claude)", source: "claude"},
	{name: "fork", description: "Fork the session (claude/codex/pi)", source: "compat"},
	{name: "clone", description: "Clone the session (pi)", source: "pi"},
	{name: "init", description: "Generate AGENTS.md (claude/opencode/codex)", source: "compat"},
	{name: "import", description: "Import a session (codex)", source: "codex"},
	{name: "mcp", description: "Manage MCP servers (claude/codex)", source: "compat"},
	{name: "hooks", description: "Manage hooks (claude/codex)", source: "compat"},
	{name: "agents", description: "Manage agents (claude/codex)", source: "compat"},
	{name: "plugins", description: "Manage plugins (codex)", source: "codex"},
	{name: "apps", description: "Manage apps (codex)", source: "codex"},
	{name: "skills", description: "Manage skills (claude/codex)", source: "compat"},
	{name: "memories", description: "Manage memories (codex)", source: "codex"},
	{name: "vim", description: "Vim keymap (codex)", source: "codex"},
	{name: "keymap", description: "Keymap configuration (codex)", source: "codex"},
	{name: "editor", description: "Editor settings (codex)", source: "codex"},
	{name: "details", description: "Show details (codex)", source: "codex"},
	{name: "ide", description: "IDE integration (codex)", source: "codex"},
	{name: "permissions", description: "Permissions management (codex)", source: "codex"},
	{name: "elevatesandbox", description: "Elevate sandbox (codex)", source: "codex"},
	{name: "sandboxreadroot", description: "Sandbox read root (codex)", source: "codex"},
	{name: "experimental", description: "Experimental features (codex)", source: "codex"},
	{name: "autoreview", description: "Auto-review (codex)", source: "codex"},
	{name: "export", description: "Export conversation (opencode/codex/pi)", source: "compat"},
	{name: "share", description: "Share conversation (opencode/pi)", source: "compat"},
	{name: "copy", description: "Copy conversation (pi)", source: "pi"},
	{name: "raw", description: "Raw output (codex)", source: "codex"},
	{name: "diff", description: "Show diff (codex)", source: "codex"},
	{name: "review", description: "Code review (codex)", source: "codex"},
	{name: "plan", description: "Plan mode (codex)", source: "codex"},
	{name: "goal", description: "Goal mode (codex)", source: "codex"},
	{name: "side", description: "Side conversation (codex)", source: "codex"},
	{name: "btw", description: "Open btw (codex)", source: "codex"},
	{name: "mention", description: "Mention a file (codex)", source: "codex"},
	{name: "rename", description: "Rename session (codex)", source: "codex"},
	{name: "archive", description: "Archive session (codex)", source: "codex"},
	{name: "delete", description: "Delete session (codex)", source: "codex"},
	{name: "debugconfig", description: "Debug config (codex)", source: "codex"},
	{name: "statusline", description: "Status line config (codex)", source: "codex"},
	{name: "pets", description: "Terminal pets (codex)", source: "codex"},
	{name: "feedback", description: "Send feedback (codex)", source: "codex"},
	{name: "rollout", description: "Rollout info (codex)", source: "codex"},
	{name: "ps", description: "Show processes (codex)", source: "codex"},
	{name: "stop", description: "Stop the agent (codex)", source: "codex"},
	{name: "personality", description: "Personality settings (codex)", source: "codex"},
	{name: "testapproval", description: "Test approval flow (codex)", source: "codex"},
	{name: "multiagents", description: "Multi-agent mode (codex)", source: "codex"},
	{name: "memorydrop", description: "Drop memory (codex)", source: "codex"},
	{name: "memoryupdate", description: "Update memory (codex)", source: "codex"},
	{name: "name", description: "Rename session (pi)", source: "pi"},
	{name: "tree", description: "Show project tree (pi)", source: "pi"},
	{name: "reload", description: "Reload config (pi)", source: "pi"},
	{name: "changelog", description: "Show changelog (pi)", source: "pi"},
	{name: "login", description: "Log in (pi/codex)", source: "compat"},
}

// registerCompatCommands overlays the compat catalog onto the registry
// (SC-1). No-op when features.slash_compat is disabled. Names colliding with
// native commands are skipped by RegisterOverlay; only free aliases merge.
func registerCompatCommands(r *SlashRegistry, cfg CLIConfig) {
	if !cfg.Features.SlashCompat {
		return
	}
	for _, e := range compatCatalog {
		// SC-5: cd/pwd map onto /workspace; skip them when workspace
		// switching is disabled so the alias never dangles.
		if (e.name == "cd" || e.name == "pwd") && !cfg.Features.WorkspaceSwitch {
			continue
		}
		// RF-2: /thinking maps onto /effort; skip when effort control is off.
		if e.name == "thinking" && !cfg.Features.EffortControl {
			continue
		}
		// RF-3: /theme maps onto the real theme switcher; skip when disabled.
		if e.name == "theme" && !cfg.Features.ThemeSwitch {
			continue
		}
		if e.mapTo != "" {
			mapTo := e.mapTo
			r.RegisterOverlay(&SlashCommand{
				Name:        e.name,
				Aliases:     e.aliases,
				Description: e.description,
				Source:      e.source,
				Implemented: true,
				Handler: func(m *chatTUI, args string) (tea.Cmd, bool) {
					return m.tuiDispatchCommand("/" + mapTo + " " + args)
				},
			})
			continue
		}
		if e.handler != nil {
			r.RegisterOverlay(&SlashCommand{
				Name:        e.name,
				Aliases:     e.aliases,
				Description: e.description,
				Source:      e.source,
				Implemented: true,
				Handler:     e.handler,
			})
			continue
		}
		r.RegisterOverlay(&SlashCommand{
			Name:        e.name,
			Aliases:     e.aliases,
			Description: e.description,
			Source:      e.source,
			Implemented: false,
		})
	}
}

// tuiHandleModelCompat handles the compat /model command: reports the active
// model route. Used only when features.model_switch is disabled (the native
// /model picker is registered otherwise and wins via RegisterOverlay).
func tuiHandleModelCompat(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	m.commitLine("")
	if args != "" {
		m.commitLine(warnText(fmt.Sprintf("  Runtime model switching is disabled (features.model_switch=false); edit llm.model in %s and restart.", DefaultConfigPath())))
		return nil, false
	}
	m.commitLine(accent("Model route:"))
	m.commitLine(dim("  Current:  ") + footerInfo(m.modelLabel))
	if m.route.Endpoint != "" {
		m.commitLine(dim("  Endpoint: ") + footerInfo(m.route.Endpoint))
	}
	m.commitLine(dim("  Enable runtime switching with features.model_switch=true."))
	return nil, false
}
