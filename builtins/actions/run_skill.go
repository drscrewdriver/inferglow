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
	"github.com/inferglow/skill"
)

// RunSkillConfig configures the run_skill action.
type RunSkillConfig struct {
	// Store is the skill store to load skills from.
	Store *skill.Store
	// MaxRounds is the maximum iteration rounds for subagent mode. Default 15.
	MaxRounds int
}

// NewRunSkillAction creates the "run_skill" action that loads and executes
// a skill playbook. Supports two modes:
//   - inline: skill body is injected as tool result into current turn
//   - subagent: skill runs in an isolated sub-agent via FlowContext.RunAgent
func NewRunSkillAction(cfg RunSkillConfig) *action.Action {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 15
	}
	return &action.Action{
		Name:        "run_skill",
		Description: "Invoke a skill playbook. Use 'inline' for simple playbooks that inject instructions into the current turn, or 'subagent' for complex playbooks that run in an isolated sub-agent.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string", "description": "Skill name (e.g. 'go-test-fix', 'code-review')."},
				"arguments": map[string]any{"type": "string", "description": "Task-specific arguments or context for the skill."},
			},
			"required": []string{"name"},
		},
		Executor: &runSkillExecutor{cfg: cfg},
		Tags:     []string{"skill", "playbook", "builtin"},
	}
}

type runSkillExecutor struct {
	cfg RunSkillConfig
}

func (e *runSkillExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "run_skill: name is required"}, nil
	}
	arguments, _ := input["arguments"].(string)

	// Load skill from store
	sk, ok := e.cfg.Store.Read(name)
	if !ok {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("run_skill: skill '%s' not found. Available skills: list from skill store.", name),
		}, nil
	}

	// Build the full prompt: skill body + arguments
	prompt := sk.Body
	if arguments != "" {
		prompt = fmt.Sprintf("%s\n\n## Task Arguments\n%s", sk.Body, arguments)
	}

	// Inline mode: return body as tool result
	if sk.RunAs == "inline" || sk.RunAs == "" {
		return &action.ActionResult{
			OK:     true,
			Status: "ok",
			Result: prompt,
		}, nil
	}

	// Subagent mode: run in isolated sub-agent
	if sk.RunAs == "subagent" {
		fc, ok := flow.FlowContextFrom(ctx)
		if !ok || fc == nil {
			return &action.ActionResult{
				OK:     false,
				Status: "error",
				Error:  "run_skill: subagent mode requires flow context (not running inside a flow)",
			}, nil
		}

		systemPrompt := fmt.Sprintf("You are executing the '%s' skill.\n\n%s", sk.Name, sk.Body)
		task := arguments
		if task == "" {
			task = fmt.Sprintf("Execute the %s skill", sk.Name)
		}

		opts := &flow.AgentRunOptions{
			MaxRounds: e.cfg.MaxRounds,
			MaxDepth:  3,
		}
		resp, err := fc.RunAgent(ctx, task, systemPrompt, opts)
		if err != nil {
			return &action.ActionResult{
				OK:     false,
				Status: "error",
				Error:  fmt.Sprintf("run_skill: subagent error: %v", err),
			}, nil
		}

		return &action.ActionResult{OK: true, Status: "ok", Result: resp}, nil
	}

	return &action.ActionResult{
		OK:     false,
		Status: "error",
		Error:  fmt.Sprintf("run_skill: unknown runas mode '%s'", sk.RunAs),
	}, nil
}
