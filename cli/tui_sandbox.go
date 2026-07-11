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

// validSandboxModes lists all recognized sandbox mode values.
var validSandboxModes = []string{
	"trusted_local",
	"local",
	"docker",
	"gvisor",
	"auto",
}

// tuiHandleSandbox handles the /sandbox command.
// No args: show current mode and available modes.
// With arg: switch to the specified mode (runtime only, not persisted).
func (m *chatTUI) tuiHandleSandbox(args string) {
	m.commitLine("")
	mode := strings.TrimSpace(args)

	// Switch mode.
	if mode != "" {
		if !isValidSandboxMode(mode) {
			m.commitLine(errorText(fmt.Sprintf("Unknown sandbox mode: %q", mode)))
			m.commitLine(dim("Valid modes: " + strings.Join(validSandboxModes, ", ")))
			return
		}
		m.cfg.SandboxMode = mode
		m.commitLine(successText("Sandbox mode switched to: ") + infoText(mode))
		m.commitLine(dim("Note: runtime only. Use /config save to persist."))
		return
	}

	// Show current mode.
	m.commitLine(accent("Sandbox configuration:"))
	m.commitLine(dim("  Current mode: ") + infoText(m.cfg.SandboxMode))
	m.commitLine(dim("  Available modes:"))
	for _, mode := range validSandboxModes {
		marker := "  "
		if mode == m.cfg.SandboxMode {
			marker = "→ "
		}
		m.commitLine(dim(fmt.Sprintf("    %s%s", marker, mode)))
	}
	m.commitLine(dim("  Usage: /sandbox <mode> to switch"))
}

// tuiHandleConfig handles the /config command.
// Shows config path and key settings. With "save" arg, persists current config.
func (m *chatTUI) tuiHandleConfig(args string) {
	m.commitLine("")
	sub := strings.TrimSpace(args)

	switch sub {
	case "save":
		path := DefaultConfigPath()
		if err := SaveConfig(m.cfg, path); err != nil {
			m.commitLine(errorText(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}
		m.commitLine(successText("Config saved to: ") + footerInfo(path))
		return
	case "":
		// Show current config summary.
	default:
		m.commitLine(errorText(fmt.Sprintf("Unknown config subcommand: %s", sub)))
		m.commitLine(dim("Usage: /config [save]"))
		return
	}

	path := DefaultConfigPath()
	m.commitLine(accent("Configuration:"))
	m.commitLine(dim("  Config path:  ") + footerInfo(path))
	m.commitLine(dim("  Data dir:     ") + footerInfo(m.cfg.DataDir))
	m.commitLine(dim("  Workspace:    ") + footerInfo(m.cfg.WorkspaceDir))
	m.commitLine(dim("  Model:        ") + footerInfo(m.cfg.LLM.Model))
	m.commitLine(dim("  Provider:     ") + footerInfo(m.cfg.LLM.Provider))
	m.commitLine(dim("  Endpoint:     ") + footerInfo(m.cfg.LLM.Endpoint))
	m.commitLine(dim("  Sandbox:      ") + infoText(m.cfg.SandboxMode))
	m.commitLine(dim("  Window:       ") + footerInfo(shortTokens(m.cfg.WindowTokens)))
	m.commitLine(dim("  Unsafe mode:  ") + footerInfo(fmt.Sprintf("%v", m.cfg.UnsafeMode)))
	m.commitLine(dim("  TUI mode:     ") + footerInfo(fmt.Sprintf("%v", m.cfg.Features.TUIMode)))
	m.commitLine(dim("  Compression:  ") + footerInfo(fmt.Sprintf("%v", m.cfg.Features.Compression)))
	m.commitLine(dim("  Memory inject:") + footerInfo(fmt.Sprintf("%v", m.cfg.Features.MemoryInjection)))
	m.commitLine(dim("  Memory store: ") + footerInfo(fmt.Sprintf("%v", m.cfg.Features.MemoryStorage)))
	m.commitLine(dim("  Use /config save to persist current settings."))
}

// isValidSandboxMode checks if a mode string is recognized.
func isValidSandboxMode(mode string) bool {
	for _, m := range validSandboxModes {
		if m == mode {
			return true
		}
	}
	return false
}
