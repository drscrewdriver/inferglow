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
func (r *SlashRegistry) Register(cmd *SlashCommand) {
	if _, exists := r.index[cmd.Name]; exists {
		panic(fmt.Sprintf("slash command already registered: %s", cmd.Name))
	}
	r.commands = append(r.commands, cmd)
	r.index[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.index[alias] = cmd
	}
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
