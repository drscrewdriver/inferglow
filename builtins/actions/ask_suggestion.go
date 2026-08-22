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

package actions

import (
	"context"

	"github.com/inferglow/action"
)

// AskSuggestionActionID is the registered Action name for the ask_suggestion
// built-in. It lets the model surface a clarification / next-step suggestion
// request to the operator mid-turn.
const AskSuggestionActionID = "ask_suggestion"

// AskSuggestionInput is the strongly-typed input for the ask_suggestion Action.
type AskSuggestionInput struct {
	// Question is the suggestion/clarification the model wants the operator
	// to confirm or answer (required).
	Question string `json:"question"`
	// Context supplies optional background for the operator.
	Context string `json:"context,omitempty"`
	// Options are optional suggested answers the operator can pick from.
	Options []string `json:"options,omitempty"`
}

// AskSuggestionResult is the outcome returned to the model. It is a
// non-destructive outcome: the question is surfaced for a human to answer,
// and the engine/host injects the answer back as the tool result.
type AskSuggestionResult struct {
	Status       string `json:"status"` // "posed" | "answered"
	Question     string `json:"question"`
	Answer       string `json:"answer,omitempty"`
	NeedsOperator bool   `json:"needs_operator"`
}

// AskSuggestionSpec is the ActionSpec declaring ask_suggestion's safety
// properties: no side effects, no approval gate (posing a question is benign),
// no sandbox; replay-safe.
var AskSuggestionSpec = &action.ActionSpec{
	ActionID:         AskSuggestionActionID,
	Name:             "Ask Suggestion",
	Description:      "Pose a clarification or next-step suggestion to the operator and wait for their answer. Use when you need human input on an ambiguous decision or before a consequential action.",
	SideEffectLevel:  action.SideEffectNone,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"hitl", "builtin"},
	Kwargs: map[string]any{
		"question": map[string]any{"type": "string", "required": true},
		"context":  map[string]any{"type": "string"},
		"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
}

// askSuggestionExecutor adapts the question-posing capability to the
// action.ActionExecutor interface. It validates the question and returns a
// "posed" result; the host (TUI/CLI) is responsible for answering via the
// HITL bridge and feeding the answer back to the model in the next round.
type askSuggestionExecutor struct{}

func (askSuggestionExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	q, _ := input["question"].(string)
	if q == "" {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "ask_suggestion: question is required",
		}, nil
	}
	return &action.ActionResult{
		OK:     true,
		Status: "posed",
		Result: AskSuggestionResult{
			Status:        "posed",
			Question:      q,
			NeedsOperator: true,
		},
	}, nil
}

// NewAskSuggestionAction builds a registered-ready Action for the
// ask_suggestion built-in.
func NewAskSuggestionAction() *action.Action {
	return &action.Action{
		Name:        AskSuggestionActionID,
		Description: "Pose a clarification or next-step suggestion to the operator and wait for their answer.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string"},
				"context":  map[string]any{"type": "string"},
				"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"question"},
		},
		Executor: askSuggestionExecutor{},
		Tags:     []string{"hitl", "builtin"},
	}
}