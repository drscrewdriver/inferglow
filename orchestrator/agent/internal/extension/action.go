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

package extension

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// ActionExtension wraps the action.ActionRegistry and provides
// a simplified interface for the orchestrator to manage and execute actions.
type ActionExtension struct {
	registry *action.ActionRegistry
}

// NewActionExtension creates an empty ActionExtension.
func NewActionExtension() *ActionExtension {
	return &ActionExtension{
		registry: action.NewRegistry(),
	}
}

// SetRegistry replaces the internal registry.
func (e *ActionExtension) SetRegistry(r *action.ActionRegistry) {
	e.registry = r
}

// Register adds an action to the registry.
func (e *ActionExtension) Register(a *action.Action) error {
	return e.registry.Register(a)
}

// ListActions returns all registered actions as tool definitions.
func (e *ActionExtension) ListActions() []map[string]any {
	names := e.registry.List()
	actions := make([]map[string]any, 0, len(names))
	for _, name := range names {
		a := e.registry.GetAction(name)
		if a == nil {
			continue
		}
		actions = append(actions, map[string]any{
			"name":        a.Name,
			"description": a.Description,
			"schema":      a.Schema,
		})
	}
	return actions
}

// GetRegistry returns the internal ActionRegistry.
func (e *ActionExtension) GetRegistry() *action.ActionRegistry {
	return e.registry
}

// Execute dispatches an action by name with the given input.
func (e *ActionExtension) Execute(ctx context.Context, name string, input map[string]any) (*action.ActionResult, error) {
	result, err := e.registry.Execute(ctx, name, input)
	if err != nil {
		return nil, fmt.Errorf("action execution failed: %w", err)
	}
	return result, nil
}
