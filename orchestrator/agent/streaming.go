package agent

import (
	"context"
	"fmt"

	"github.com/inferglow/model"
)

// buildToolsFromActions converts action maps (as returned by
// ActionExtension.ListActions) into model.ToolDefinition values.
//
// BUG-NEW-2 (GREEN): All type assertions use the comma-ok pattern so a
// missing key or a value of the wrong type returns an error containing
// the offending field name and the actual underlying type, rather than
// panicking and aborting the stream. The loop variable is named `act`
// to avoid shadowing the Agent receiver `a` in StreamRun's scope.
func buildToolsFromActions(actions []map[string]any) ([]model.ToolDefinition, error) {
	tools := make([]model.ToolDefinition, 0, len(actions))
	for _, act := range actions {
		name, ok := act["name"].(string)
		if !ok {
			return nil, fmt.Errorf("action field 'name' missing or not string, got %T", act["name"])
		}
		description, ok := act["description"].(string)
		if !ok {
			return nil, fmt.Errorf("action field 'description' missing or not string, got %T", act["description"])
		}
		schema, ok := act["schema"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("action field 'schema' missing or not map[string]any, got %T", act["schema"])
		}
		tools = append(tools, model.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  schema,
		})
	}
	return tools, nil
}

// StreamRun returns a channel that emits StreamChunks as the LLM generates output.
// The stream does not wait for action execution — it only delivers LLM output in real-time.
func (a *Agent) StreamRun(ctx context.Context, userMessage string, opts ...RunOption) (<-chan *model.StreamChunk, error) {
	c := &runConfig{maxRounds: 10}
	for _, opt := range opts {
		opt(c)
	}

	// Add user message
	a.session.AddUserMessage(userMessage)

	// Build tool definitions (with schema from action)
	actions := a.actionExt.ListActions()
	tools, err := buildToolsFromActions(actions)
	if err != nil {
		return nil, err
	}

	req := &model.ModelRequest{
		System:      c.systemPrompt,
		ChatHistory: a.session.PreparePrompt(),
		Tools:       tools,
	}

	data, err := a.engine.modelReq.GenerateRequestData(ctx, req)
	if err != nil {
		return nil, err
	}

	stream, err := a.engine.modelReq.RequestModel(ctx, data)
	if err != nil {
		return nil, err
	}

	return stream, nil
}
