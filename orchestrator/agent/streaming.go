package agent

import (
	"context"

	"github.com/inferglow/model"
)

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
	tools := make([]model.ToolDefinition, 0, len(actions))
	for _, a := range actions {
		tools = append(tools, model.ToolDefinition{
			Name:        a["name"].(string),
			Description: a["description"].(string),
			Parameters:  a["schema"].(map[string]any),
		})
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
