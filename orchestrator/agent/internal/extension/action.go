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
// It also exposes a derived action.GroupRegistry for organizing tools into
// named groups (see action/group.go).
type ActionExtension struct {
	registry *action.ActionRegistry
	groups   *action.GroupRegistry
}

// NewActionExtension creates an empty ActionExtension.
func NewActionExtension() *ActionExtension {
	e := &ActionExtension{
		registry: action.NewRegistry(),
	}
	e.groups = action.NewGroupRegistry(e.registry)
	return e
}

// SetRegistry replaces the internal registry and re-binds the derived group
// view to the new registry.
func (e *ActionExtension) SetRegistry(r *action.ActionRegistry) {
	e.registry = r
	e.groups = action.NewGroupRegistry(r)
}

// Register adds an action to the registry.
func (e *ActionExtension) Register(a *action.Action) error {
	return e.registry.Register(a)
}

// RegisterGroup registers a named tool group against the derived group view.
func (e *ActionExtension) RegisterGroup(g *action.ToolGroup) error {
	return e.groups.Register(g)
}

// GetGroupRegistry returns the derived group view.
func (e *ActionExtension) GetGroupRegistry() *action.GroupRegistry {
	return e.groups
}

// ListActions returns all registered actions as tool definitions.
func (e *ActionExtension) ListActions() []map[string]any {
	return e.toToolDefinitions(e.registry.List())
}

// ListActionsByGroup returns the tool definitions of actions belonging to the
// named group. A missing group returns an error wrapping ErrGroupNotFound.
func (e *ActionExtension) ListActionsByGroup(group string) ([]map[string]any, error) {
	names, err := e.groups.ListActionNames(group)
	if err != nil {
		return nil, err
	}
	return e.toToolDefinitions(names), nil
}

// ListActionsFiltered returns tool definitions of actions that belong to the
// named group AND pass the request-time ToolFilter. A missing group returns an
// error wrapping ErrGroupNotFound. A nil filter passes everything in the group.
func (e *ActionExtension) ListActionsFiltered(group string, filter *action.ToolFilter, specs map[string]*action.ActionSpec) ([]map[string]any, error) {
	names, err := e.groups.ListActionNames(group)
	if err != nil {
		return nil, err
	}
	if filter != nil {
		names = filterNames(filter, names, specs)
	}
	return e.toToolDefinitions(names), nil
}

// filterNames returns the subset of names that pass the filter, using the
// optional specs map for side-effect level resolution. It operates on the
// given names only (not the whole registry) so filtered results stay within
// the group's members.
func filterNames(filter *action.ToolFilter, names []string, specs map[string]*action.ActionSpec) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		var spec *action.ActionSpec
		if specs != nil {
			spec = specs[name]
		}
		if filter.IsAllowed(name, spec) {
			result = append(result, name)
		}
	}
	return result
}

// toToolDefinitions converts a list of action names into tool definition maps.
func (e *ActionExtension) toToolDefinitions(names []string) []map[string]any {
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
