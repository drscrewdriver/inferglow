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

package server

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/mcpserver"
)

// MCPToolRecord is the JSON-safe projection of an installed MCP tool exposed
// by the MCP Hub (spec C-9). The underlying action.Action is never serialized;
// only the MCP tool descriptor surface is surfaced.
type MCPToolRecord struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// MCPHubStore is the C-9 MCP Hub store. It manages installed MCP tools by
// reusing mcpserver's ActionRegistryAdapter: discovery and invocation flow
// through the mcpserver ToolProvider contract, while the underlying action
// registry stores the actual tools.
//
// The action package is owned by another lane (G1) and is treated as read-only
// here. Removal is a real unregister on the backing registry.
type MCPHubStore struct {
	reg     *action.ActionRegistry
	adapter *mcpserver.ActionRegistryAdapter
}

// NewMCPHubStore creates an empty MCP Hub store backed by a fresh action
// registry and an mcpserver adapter.
func NewMCPHubStore() *MCPHubStore {
	reg := action.NewRegistry()
	return &MCPHubStore{
		reg:     reg,
		adapter: mcpserver.NewActionRegistryAdapter(reg),
	}
}

// Install registers an action as an installable MCP tool.
func (m *MCPHubStore) Install(a *action.Action) error {
	return m.reg.Register(a)
}

// List returns the descriptors of installed MCP tools, sorted.
func (m *MCPHubStore) List() []MCPToolRecord {
	tools := m.adapter.ListTools()
	out := make([]MCPToolRecord, 0, len(tools))
	for _, t := range tools {
		out = append(out, MCPToolRecord{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out
}

// Get returns the descriptor for a single installed MCP tool.
func (m *MCPHubStore) Get(name string) (MCPToolRecord, error) {
	a, err := m.reg.Get(name)
	if err != nil {
		return MCPToolRecord{}, err
	}
	return MCPToolRecord{
		Name:        a.Name,
		Description: a.Description,
		InputSchema: a.Schema,
	}, nil
}

// Remove unregisters an MCP tool by name.
func (m *MCPHubStore) Remove(name string) error {
	if !m.reg.Unregister(name) {
		return fmt.Errorf("tool %q not found", name)
	}
	return nil
}

// Call invokes an installed MCP tool by name with the given arguments.
func (m *MCPHubStore) Call(ctx context.Context, name string, args map[string]any) (*mcpserver.ToolResult, error) {
	return m.adapter.CallTool(ctx, name, args)
}