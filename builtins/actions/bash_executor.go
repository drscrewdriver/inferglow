package actions

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// BashExecutorActionID is the registered Action name for bash execution.
const BashExecutorActionID = "bash_executor"

// BashExecutionRequest is the strongly-typed payload handed to a
// BashExecutor. The runtime does not interpret Command — the injected
// executor is responsible for both dispatch and sandboxing.
type BashExecutionRequest struct {
	Command string `json:"command"`            // shell command line
	Workdir string `json:"workdir,omitempty"`  // optional working directory
	Stdin   string `json:"stdin,omitempty"`    // optional stdin
	Timeout string `json:"timeout,omitempty"`  // optional duration string
	Env     map[string]string `json:"env,omitempty"` // optional environment overrides
}

// BashExecutionResult is the structured outcome returned by a
// BashExecutor.
type BashExecutionResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration,omitempty"`
}

// BashExecutor is the abstraction that actually runs shell commands.
// Concrete implementations live outside this package (typically in the
// sandbox module) and are injected by the caller — this package
// deliberately does NOT execute commands itself, in keeping with the
// high-risk classification (SideEffectExec, ApprovalRequired,
// SandboxRequired).
type BashExecutor interface {
	// Execute runs req.Command inside whatever sandbox the
	// implementation provides.
	Execute(ctx context.Context, req BashExecutionRequest) (*BashExecutionResult, error)
}

// BashExecutorSpec is the ActionSpec for bash_executor: highest risk
// tier — exec side-effect, approval required, sandbox required.
var BashExecutorSpec = &action.ActionSpec{
	ActionID:         BashExecutorActionID,
	Name:             "BashExecutor",
	Description:      "Execute a bash command inside a sandbox. Execution is delegated to an injected BashExecutor.",
	SideEffectLevel:  action.SideEffectExec,
	ApprovalRequired: true,
	SandboxRequired:  true,
	ReplaySafe:       false,
	ExposeToModel:    true,
	Tags:             []string{"exec", "shell", "builtin"},
	Kwargs: map[string]any{
		"command": map[string]any{"type": "string", "required": true},
		"workdir": map[string]any{"type": "string", "required": false},
		"stdin":   map[string]any{"type": "string", "required": false},
		"timeout": map[string]any{"type": "string", "required": false},
		"env":     map[string]any{"type": "object", "required": false},
	},
	Returns: map[string]any{"type": "object"},
}

// bashExecutorAction wraps an injected BashExecutor behind the
// ActionExecutor contract.
type bashExecutorAction struct {
	executor BashExecutor
}

func (a *bashExecutorAction) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if a == nil || a.executor == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "bash_executor: no executor injected",
		}, nil
	}
	req, err := decodeBashRequest(input)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: err.Error()}, nil
	}
	if req.Command == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "bash_executor: command is required"}, nil
	}
	result, err := a.executor.Execute(ctx, req)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("bash_executor: %s", err),
		}, nil
	}
	ok := result != nil && result.ExitCode == 0
	status := "success"
	if !ok {
		status = "error"
	}
	return &action.ActionResult{
		OK:     ok,
		Status: status,
		Result: result,
	}, nil
}

// decodeBashRequest converts the loose input map into a typed request.
func decodeBashRequest(input map[string]any) (BashExecutionRequest, error) {
	req := BashExecutionRequest{}
	if v, ok := input["command"]; ok {
		if s, ok := v.(string); ok {
			req.Command = s
		} else {
			return req, fmt.Errorf("bash_executor: command must be string, got %T", v)
		}
	}
	if v, ok := input["workdir"]; ok {
		if s, ok := v.(string); ok {
			req.Workdir = s
		}
	}
	if v, ok := input["stdin"]; ok {
		if s, ok := v.(string); ok {
			req.Stdin = s
		}
	}
	if v, ok := input["timeout"]; ok {
		if s, ok := v.(string); ok {
			req.Timeout = s
		}
	}
	if v, ok := input["env"]; ok {
		if m, ok := v.(map[string]any); ok {
			env := make(map[string]string, len(m))
			for k, val := range m {
				if s, ok := val.(string); ok {
					env[k] = s
				}
			}
			req.Env = env
		}
	}
	return req, nil
}

// NewBashExecutorAction builds an Action that delegates execution to
// executor. The executor is typically backed by sandbox.Manager. The
// returned Action carries ApprovalRequired=true and SandboxRequired=true
// so the runtime gates it appropriately before dispatching.
func NewBashExecutorAction(executor BashExecutor) *action.Action {
	return &action.Action{
		Name:        BashExecutorActionID,
		Description: "Execute a bash command inside a sandbox.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"workdir": map[string]any{"type": "string"},
				"stdin":   map[string]any{"type": "string"},
				"timeout": map[string]any{"type": "string"},
				"env":     map[string]any{"type": "object"},
			},
			"required": []string{"command"},
		},
		Executor: &bashExecutorAction{executor: executor},
		Tags:     []string{"exec", "shell", "builtin"},
	}
}
