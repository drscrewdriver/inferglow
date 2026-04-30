package agent

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
