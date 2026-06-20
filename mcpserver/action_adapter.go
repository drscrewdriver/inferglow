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

package mcpserver

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// ActionRegistryAdapter wraps an action.ActionRegistry to satisfy the
// ToolProvider interface, bridging the Action Runtime to MCP.
type ActionRegistryAdapter struct {
	Registry *action.ActionRegistry
}

// NewActionRegistryAdapter creates an adapter for the given registry.
func NewActionRegistryAdapter(reg *action.ActionRegistry) *ActionRegistryAdapter {
	return &ActionRegistryAdapter{Registry: reg}
}

// ListTools converts all registered actions to MCP tool descriptors.
func (a *ActionRegistryAdapter) ListTools() []ToolDescriptor {
	names := a.Registry.List()
	tools := make([]ToolDescriptor, 0, len(names))
	for _, name := range names {
		act, err := a.Registry.Get(name)
		if err != nil {
			continue
		}
		tools = append(tools, ToolDescriptor{
			Name:        act.Name,
			Description: act.Description,
			InputSchema: act.Schema,
		})
	}
	return tools
}

// CallTool executes an action by name with the given arguments and
// converts the result to an MCP ToolResult.
func (a *ActionRegistryAdapter) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	result, err := a.Registry.Execute(ctx, name, args)
	if err != nil {
		return nil, fmt.Errorf("action execute %q: %w", name, err)
	}

	if !result.OK {
		return &ToolResult{
			Content: []ToolContent{{Type: "text", Text: result.Error}},
			IsError: true,
		}, nil
	}

	// Convert result to text content
	text := fmt.Sprintf("%v", result.Result)
	return &ToolResult{
		Content: []ToolContent{{Type: "text", Text: text}},
	}, nil
}
