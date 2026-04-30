package agent

import (
	"context"
	"strings"

	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/actionruntime"
)

// Engine orchestrates the PLAN → EXECUTE loop.
type Engine struct {
	session   *SessionExtension
	actionExt *ActionExtension
	modelReq  model.ModelRequester
}

// NewEngine creates an Engine with the given components.
func NewEngine(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester) *Engine {
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mr,
	}
}

// executeLoop runs the PLAN → EXECUTE loop until the LLM returns a response
// or maxRounds is reached.
func (e *Engine) executeLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (*actionruntime.Decision, error) {
	// Add user message to session
	e.session.AddUserMessage(userMessage)

	round := 0
	for {
		// Build ModelRequest
		tools := e.buildToolDefinitions()
		req := &model.ModelRequest{
			System:      systemPrompt,
			ChatHistory: e.session.PreparePrompt(),
			Tools:       tools,
			Output: &model.OutputSchema{
				Type: "object",
				Properties: map[string]any{
					"next_action": map[string]any{
						"type":        "string",
						"description": "\"execute\" or \"response\"",
					},
					"action_calls": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string"},
								"params": map[string]any{"type": "object"},
							},
						},
					},
					"final_response": map[string]any{
						"type":        "string",
						"description": "The response to return when next_action is \"response\"",
					},
				},
			},
		}

		// Call LLM
		data, err := e.modelReq.GenerateRequestData(ctx, req)
		if err != nil {
			return nil, err
		}

		stream, err := e.modelReq.RequestModel(ctx, data)
		if err != nil {
			return nil, err
		}

		// Collect response content
		var content strings.Builder
		for chunk := range stream {
			content.WriteString(chunk.Delta)
		}

		// Parse decision
		decision, err := actionruntime.ParseDecision(content.String())
		if err != nil {
			return nil, err
		}

		// Check if we should continue
		if !actionruntime.ShouldContinue(*decision, round, maxRounds) {
			return decision, nil
		}

		// Execute actions
		dispatcher := actionruntime.NewActionDispatcher(e.actionExt.GetRegistry())
		results := dispatcher.Execute(ctx, decision.ActionCalls)

		// Add results to session
		for i, call := range decision.ActionCalls {
			if i < len(results) {
				e.session.AddActionResult(call.Name, results[i])
			}
		}

		round++
	}
}

// buildToolDefinitions creates ToolDefinition list from registered actions.
func (e *Engine) buildToolDefinitions() []model.ToolDefinition {
	actions := e.actionExt.ListActions()
	tools := make([]model.ToolDefinition, 0, len(actions))
	for _, a := range actions {
		tools = append(tools, model.ToolDefinition{
			Name:        a["name"].(string),
			Description: a["description"].(string),
			Parameters:  a["schema"],
		})
	}
	return tools
}


