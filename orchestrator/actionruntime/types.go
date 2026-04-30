// Package actionruntime defines core data structures for the orchestrator.
//
// It provides the types needed to coordinate between LLM decisions and action execution:
//   - ActionCall: represents a single action invocation
//   - Decision: the LLM's planning decision (execute actions or respond)
//   - ToolDefinitionBuilder: converts Action structs to model.ToolDefinition
package actionruntime

import "github.com/inferglow/action"

// ActionCall represents a single invocation of an Action.
type ActionCall struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
}

// Decision represents the LLM's planning decision for the next step.
type Decision struct {
	NextAction    string       `json:"next_action"`    // "execute" or "response"
	ActionCalls   []ActionCall `json:"action_calls,omitempty"`
	FinalResponse string       `json:"final_response,omitempty"`
}

// ToolDefinitionBuilder converts an action.Action to a model.ToolDefinition.
func ToolDefinitionBuilder(a *action.Action) map[string]any {
	return map[string]any{
		"name":        a.Name,
		"description": a.Description,
		"parameters":  a.Schema,
	}
}
