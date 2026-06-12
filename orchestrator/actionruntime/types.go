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
	NextAction    string       `json:"next_action"` // "execute" or "response"
	ActionCalls   []ActionCall `json:"action_calls,omitempty"`
	FinalResponse string       `json:"final_response,omitempty"`
	// Reasoning carries the model's reasoning content for the current round
	// (G1-02). It is populated by the engine from StreamChunk.Reasoning and
	// forwarded to the session so the next round's ChatHistory includes
	// reasoning_content in the assistant message — required by DeepSeek, MiMo
	// and other providers that mandate reasoning passback in multi-turn calls.
	Reasoning string `json:"-"`
}

// ToolDefinitionBuilder converts an action.Action to a model.ToolDefinition.
func ToolDefinitionBuilder(a *action.Action) map[string]any {
	return map[string]any{
		"name":        a.Name,
		"description": a.Description,
		"parameters":  a.Schema,
	}
}
