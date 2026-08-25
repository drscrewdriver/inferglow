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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// SlashCommand defines a TUI slash command with metadata for discovery
// and auto-completion (OT-14).
type SlashCommand struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Source      string // "inferglow" | "claude" | "pi" | "opencode" | "codex" | "compat"
	Implemented bool   // false = recognized but not implemented (friendly hint)
	Handler     func(m *chatTUI, args string) (tea.Cmd, bool)
}

// SlashRegistry is an ordered registry of slash commands supporting
// dispatch, help generation, and prefix-based auto-completion.
type SlashRegistry struct {
	commands []*SlashCommand
	index    map[string]*SlashCommand // name/alias → command
}

// NewSlashRegistry creates an empty command registry.
func NewSlashRegistry() *SlashRegistry {
	return &SlashRegistry{
		index: make(map[string]*SlashCommand),
	}
}

// Register adds a command to the registry. Panics on duplicate names.
// Native commands are always fully implemented (Implemented is forced to
// true); compat stubs use RegisterOverlay with an explicit flag instead.
func (r *SlashRegistry) Register(cmd *SlashCommand) {
	if _, exists := r.index[cmd.Name]; exists {
		panic(fmt.Sprintf("slash command already registered: %s", cmd.Name))
	}
	cmd.Implemented = true
	r.commands = append(r.commands, cmd)
	r.index[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.index[alias] = cmd
	}
}

// RegisterOverlay registers a command without panicking on conflicts
// (SC-1): if the name is already taken by an *implemented* command, the new
// entry is skipped and only its free aliases are merged onto the existing
// command (native commands are never overridden). If the existing entry is
// an unimplemented stub, it is replaced by the new command — e.g. a user
// skill that finally implements a previously recognized-but-unimplemented
// name (SC-6). The stub's aliases are kept and repointed at the replacement.
func (r *SlashRegistry) RegisterOverlay(cmd *SlashCommand) {
	if cmd == nil {
		return
	}
	if existing, ok := r.index[cmd.Name]; ok {
		if existing.Implemented {
			for _, alias := range cmd.Aliases {
				if _, taken := r.index[alias]; !taken {
					existing.Aliases = append(existing.Aliases, alias)
					r.index[alias] = existing
				}
			}
			return
		}
		// Replace the stub in registration order.
		for i, c := range r.commands {
			if c == existing {
				r.commands[i] = cmd
				break
			}
		}
		// Keep the stub's aliases and repoint them at the replacement.
		for _, alias := range existing.Aliases {
			r.index[alias] = cmd
		}
		cmd.Aliases = append(append([]string{}, existing.Aliases...), cmd.Aliases...)
		r.index[cmd.Name] = cmd
		for _, alias := range cmd.Aliases {
			if _, taken := r.index[alias]; !taken {
				r.index[alias] = cmd
			}
		}
		return
	}
	r.commands = append(r.commands, cmd)
	r.index[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		if _, taken := r.index[alias]; !taken {
			r.index[alias] = cmd
		}
	}
}

// All returns all registered commands in registration order.
func (r *SlashRegistry) All() []*SlashCommand {
	out := make([]*SlashCommand, len(r.commands))
	copy(out, r.commands)
	return out
}

// IsImplemented reports whether the named command (or alias) is registered
// AND fully implemented. Returns false for unknown names and for
// recognized-but-unimplemented stubs (SC-1).
func (r *SlashRegistry) IsImplemented(name string) bool {
	cmd, ok := r.index[name]
	return ok && cmd.Implemented
}

// SourceOf returns the source label of a registered command, or "" when the
// name is unknown.
func (r *SlashRegistry) SourceOf(name string) string {
	if cmd, ok := r.index[name]; ok {
		return cmd.Source
	}
	return ""
}

// Dispatch looks up a command by name (or alias) and invokes its handler.
// Returns (cmd, quit, found). If not found, found=false and the caller
// should fall back to legacy handling.
func (r *SlashRegistry) Dispatch(m *chatTUI, name, args string) (tea.Cmd, bool, bool) {
	cmd, ok := r.index[name]
	if !ok {
		return nil, false, false
	}
	if cmd.Handler == nil {
		return nil, false, true
	}
	c, quit := cmd.Handler(m, args)
	return c, quit, true
}

// Complete returns command names matching the given prefix (without leading /).
// Results are sorted alphabetically.
func (r *SlashRegistry) Complete(prefix string) []string {
	prefix = strings.ToLower(prefix)
	var matches []string
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd.Name)
		}
	}
	sort.Strings(matches)
	return matches
}

// Suggest returns commands matching prefix for the IME-style popup (SC-2):
// strict prefix matches first (name or alias), then subsequence fuzzy matches
// (e.g. "bt" → "btw"), deduplicated across aliases, sorted by name length
// then alphabetically, truncated to limit. limit <= 0 means no truncation.
func (r *SlashRegistry) Suggest(prefix string, limit int) []*SlashCommand {
	var prefixMatches, subMatches []*SlashCommand
	seen := map[*SlashCommand]bool{}
	for _, cmd := range r.commands {
		if seen[cmd] {
			continue
		}
		matched, isPrefix := cmd.matchPrefix(prefix)
		if !matched {
			continue
		}
		seen[cmd] = true
		if isPrefix {
			prefixMatches = append(prefixMatches, cmd)
		} else {
			subMatches = append(subMatches, cmd)
		}
	}
	byName := func(a, b *SlashCommand) bool {
		if len(a.Name) != len(b.Name) {
			return len(a.Name) < len(b.Name)
		}
		return a.Name < b.Name
	}
	sort.Slice(prefixMatches, func(i, j int) bool { return byName(prefixMatches[i], prefixMatches[j]) })
	sort.Slice(subMatches, func(i, j int) bool { return byName(subMatches[i], subMatches[j]) })
	out := append(prefixMatches, subMatches...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Match reports whether name or any alias matches prefix via strict prefix or
// subsequence matching (case-insensitive). Empty prefix matches everything.
func (c *SlashCommand) Match(prefix string) bool {
	matched, _ := c.matchPrefix(prefix)
	return matched
}

// matchPrefix reports whether the command matches prefix and whether the
// match is a strict prefix match (preferred over subsequence).
func (c *SlashCommand) matchPrefix(prefix string) (matched, isPrefix bool) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return true, true
	}
	if strings.HasPrefix(strings.ToLower(c.Name), prefix) {
		return true, true
	}
	for _, a := range c.Aliases {
		if strings.HasPrefix(strings.ToLower(a), prefix) {
			return true, true
		}
	}
	if isSubsequence(prefix, strings.ToLower(c.Name)) {
		return true, false
	}
	for _, a := range c.Aliases {
		if isSubsequence(prefix, strings.ToLower(a)) {
			return true, false
		}
	}
	return false, false
}

// isSubsequence reports whether prefix appears as a case-sensitive
// subsequence of s (both arguments must already be lowercased). Example:
// "bt" is a subsequence of "btw"; "mo" of "model"/"memory".
func isSubsequence(prefix, s string) bool {
	if prefix == "" {
		return true
	}
	pi := 0
	for i := 0; i < len(s) && pi < len(prefix); i++ {
		if s[i] == prefix[pi] {
			pi++
		}
	}
	return pi == len(prefix)
}

// HelpText generates a formatted help listing from all registered commands.
func (r *SlashRegistry) HelpText() string {
	var sb strings.Builder
	for _, cmd := range r.commands {
		name := "/" + cmd.Name
		if cmd.Usage != "" {
			name += " " + cmd.Usage
		}
		sb.WriteString(fmt.Sprintf("  %-28s %s\n", name, cmd.Description))
	}
	return sb.String()
}

// Names returns all primary command names (sorted).
func (r *SlashRegistry) Names() []string {
	names := make([]string, len(r.commands))
	for i, cmd := range r.commands {
		names[i] = cmd.Name
	}
	sort.Strings(names)
	return names
}
