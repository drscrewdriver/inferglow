package actions

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// CodeExecutorActionID is the registered Action name for code execution.
const CodeExecutorActionID = "code_executor"

// CodeExecutionRequest is the strongly-typed payload handed to a
// CodeExecutor. The runtime does not interpret Language or Source —
// the injected executor is responsible for both dispatch and sandboxing.
type CodeExecutionRequest struct {
	Language string `json:"language"`            // e.g. "python", "go", "javascript"
	Source   string `json:"source"`              // source code to execute
	Stdin    string `json:"stdin,omitempty"`     // optional stdin
	Timeout  string `json:"timeout,omitempty"`   // optional duration string
}

// CodeExecutionResult is the structured outcome returned by a
// CodeExecutor. The runtime surfaces ExitCode, Stdout and Stderr to
// the calling agent.
type CodeExecutionResult struct {
	Language string `json:"language"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration,omitempty"` // executor-defined format
}

// CodeExecutor is the abstraction that actually runs code. Concrete
// implementations live outside this package (typically in the sandbox
// module) and are injected by the caller — this package deliberately
// does NOT execute code itself, in keeping with the high-risk
// classification (SideEffectExec, ApprovalRequired, SandboxRequired).
type CodeExecutor interface {
	// Execute runs req.Source as req.Language code inside whatever
	// sandbox the implementation provides.
	Execute(ctx context.Context, req CodeExecutionRequest) (*CodeExecutionResult, error)
}

// CodeExecutorSpec is the ActionSpec for code_executor: highest risk
// tier — exec side-effect, approval required, sandbox required.
var CodeExecutorSpec = &action.ActionSpec{
	ActionID:         CodeExecutorActionID,
	Name:             "CodeExecutor",
	Description:      "Execute arbitrary source code inside a sandbox. Execution is delegated to an injected CodeExecutor.",
	SideEffectLevel:  action.SideEffectExec,
	ApprovalRequired: true,
	SandboxRequired:  true,
	ReplaySafe:       false,
	ExposeToModel:    true,
	Tags:             []string{"exec", "code", "builtin"},
	Kwargs: map[string]any{
		"language": map[string]any{"type": "string", "required": true},
		"source":   map[string]any{"type": "string", "required": true},
		"stdin":    map[string]any{"type": "string", "required": false},
		"timeout":  map[string]any{"type": "string", "required": false},
	},
	Returns: map[string]any{"type": "object"},
}

// codeExecutorAction wraps an injected CodeExecutor behind the
// ActionExecutor contract.
type codeExecutorAction struct {
	executor CodeExecutor
}

func (a *codeExecutorAction) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if a == nil || a.executor == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "code_executor: no executor injected",
		}, nil
	}
	req, err := decodeCodeRequest(input)
	if err != nil {
		return &action.ActionResult{OK: false, Status: "error", Error: err.Error()}, nil
	}
	if req.Language == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "code_executor: language is required"}, nil
	}
	if req.Source == "" {
		return &action.ActionResult{OK: false, Status: "error", Error: "code_executor: source is required"}, nil
	}
	result, err := a.executor.Execute(ctx, req)
	if err != nil {
		return &action.ActionResult{
			OK: false, Status: "error",
			Error: fmt.Sprintf("code_executor: %s", err),
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

// decodeCodeRequest converts the loose input map into a typed request.
func decodeCodeRequest(input map[string]any) (CodeExecutionRequest, error) {
	req := CodeExecutionRequest{}
	if v, ok := input["language"]; ok {
		if s, ok := v.(string); ok {
			req.Language = s
		} else {
			return req, fmt.Errorf("code_executor: language must be string, got %T", v)
		}
	}
	if v, ok := input["source"]; ok {
		if s, ok := v.(string); ok {
			req.Source = s
		} else {
			return req, fmt.Errorf("code_executor: source must be string, got %T", v)
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
	return req, nil
}

// NewCodeExecutorAction builds an Action that delegates execution to
// executor. The executor is typically backed by sandbox.Manager. The
// returned Action carries ApprovalRequired=true and SandboxRequired=true
// so the runtime gates it appropriately before dispatching.
func NewCodeExecutorAction(executor CodeExecutor) *action.Action {
	return &action.Action{
		Name:        CodeExecutorActionID,
		Description: "Execute source code inside a sandbox.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"language": map[string]any{"type": "string"},
				"source":   map[string]any{"type": "string"},
				"stdin":    map[string]any{"type": "string"},
				"timeout":  map[string]any{"type": "string"},
			},
			"required": []string{"language", "source"},
		},
		Executor: &codeExecutorAction{executor: executor},
		Tags:     []string{"exec", "code", "builtin"},
	}
}
