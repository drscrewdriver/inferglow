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
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/flow"
)

// SubAgentConfig configures the spawn_agent action.
type SubAgentConfig struct {
	// MaxDepth is the maximum nesting depth for sub-agents. Default 3.
	MaxDepth int
	// MaxRounds is the maximum iteration rounds per sub-agent. Default 15.
	MaxRounds int
}

// NewSubAgentAction creates the "spawn_agent" action that allows the LLM
// to spawn a child agent for delegated tasks. The child agent runs via
// FlowContext.RunAgent with depth control to prevent infinite recursion.
func NewSubAgentAction(cfg SubAgentConfig) *action.Action {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 3
	}
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 15
	}
	return &action.Action{
		Name:        "spawn_agent",
		Description: "Spawn a sub-agent to handle a delegated task. The sub-agent runs independently with its own tool loop and returns a final response. Use for complex multi-step tasks that benefit from focused attention.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":          map[string]any{"type": "string", "description": "The task description for the sub-agent to accomplish."},
				"system_prompt": map[string]any{"type": "string", "description": "Optional system prompt to guide the sub-agent's behavior."},
				"max_rounds":    map[string]any{"type": "number", "description": fmt.Sprintf("Maximum iteration rounds for the sub-agent (default %d).", cfg.MaxRounds)},
			},
			"required": []string{"task"},
		},
		Executor: &subAgentExecutor{cfg: cfg},
		Tags:     []string{"agent", "delegation", "builtin"},
	}
}

type subAgentExecutor struct {
	cfg SubAgentConfig
}

func (e *subAgentExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	task, _ := input["task"].(string)
	if task == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "spawn_agent: task is required"}, nil
	}
	systemPrompt, _ := input["system_prompt"].(string)
	maxRounds := e.cfg.MaxRounds
	if f, ok := input["max_rounds"].(float64); ok && int(f) > 0 {
		maxRounds = int(f)
	}

	// Get FlowContext from ctx to access RunAgent.
	fc, ok := flow.FlowContextFrom(ctx)
	if !ok || fc == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "spawn_agent: flow context not available (not running inside a flow)",
		}, nil
	}

	// Run the sub-agent via FlowContext.RunAgent with depth control.
	opts := &flow.AgentRunOptions{
		MaxRounds: maxRounds,
		MaxDepth:  e.cfg.MaxDepth,
	}
	resp, err := fc.RunAgent(ctx, task, systemPrompt, opts)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("spawn_agent: %v", err),
		}, nil
	}

	return &action.ActionResult{OK: true, Status: "ok", Result: resp}, nil
}
