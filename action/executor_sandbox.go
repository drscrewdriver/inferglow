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

//go:build with_sandbox

package action

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/inferglow/sandbox"
)

// SandboxExecutorConfig configures a SandboxExecutor.
type SandboxExecutorConfig struct {
	Manager         *sandbox.Manager
	ApprovalService *sandbox.ApprovalService
	DefaultMode     sandbox.SandboxMode
}

// SandboxExecutor bridges sandbox.Manager to the action.ActionExecutor
// contract, exposing sandboxed command execution as an Action.
type SandboxExecutor struct {
	cfg SandboxExecutorConfig
}

// NewSandboxExecutor constructs a new SandboxExecutor.
//
// If cfg.DefaultMode is empty it defaults to sandbox.ModeTrustedLocal.
func NewSandboxExecutor(cfg SandboxExecutorConfig) *SandboxExecutor {
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = sandbox.ModeTrustedLocal
	}
	return &SandboxExecutor{cfg: cfg}
}

// Execute implements ActionExecutor. It parses the input map into a
// sandbox.Command, optionally consults the ApprovalService, runs the command
// inside a sandbox Handle, and maps the ExecutionResult into an ActionResult.
func (e *SandboxExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	argv, err := toStringSlice(input["argv"])
	if err != nil {
		return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("invalid argv: %s", err.Error())}, nil
	}
	if len(argv) == 0 {
		return &ActionResult{OK: false, Status: "error", Error: "argv must be non-empty"}, nil
	}

	env, err := toStringSlice(input["env"])
	if err != nil {
		return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("invalid env: %s", err.Error())}, nil
	}
	workdir, _ := input["workdir"].(string)
	stdin, _ := input["stdin"].(string)

	cmd := &sandbox.Command{
		Argv:    argv,
		Env:     env,
		Workdir: workdir,
		Stdin:   strings.NewReader(stdin),
	}

	mode := e.cfg.DefaultMode
	if sr, ok := input["sandbox_required"].(bool); ok && sr {
		mode = sandbox.ModeDocker
	}
	if mode == sandbox.ModeDocker {
		if _, err := e.cfg.Manager.SelectSandbox(mode); err != nil {
			mode = sandbox.ModeAuto
		}
	}

	policy := buildPolicyFromInput(input)

	if e.cfg.ApprovalService != nil {
		if ar, ok := input["approval_required"].(bool); ok && ar {
			req := &sandbox.ApprovalRequest{
				ProviderName: string(mode),
				Mode:         mode,
				Policy:       policy,
				Requester:    "sandbox_executor",
				Reason:       "execute sandbox command",
			}
			record, err := e.cfg.ApprovalService.Submit(req)
			if err != nil {
				return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("approval submit: %s", err.Error())}, nil
			}
			switch record.Status {
			case sandbox.ApprovalPending:
				return &ActionResult{OK: false, Status: "blocked", Error: "pending approval: " + record.ID}, nil
			case sandbox.ApprovalRejected:
				return &ActionResult{OK: false, Status: "blocked", Error: "approval rejected"}, nil
			case sandbox.ApprovalApproved:
				// fall through to execute
			}
		}
	}

	handle, err := e.cfg.Manager.CreateHandle(mode, nil, policy)
	if err != nil {
		return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("create handle: %s", err.Error())}, nil
	}

	if err := handle.Start(ctx); err != nil {
		_ = handle.Stop(ctx)
		return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("handle start: %s", err.Error())}, nil
	}

	result, execErr := handle.Execute(ctx, cmd)
	_ = handle.Stop(ctx)
	if execErr != nil {
		return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("handle execute: %s", execErr.Error())}, nil
	}

	if result.ExitCode == 0 {
		return &ActionResult{
			OK:     true,
			Status: "success",
			Result: map[string]any{
				"exit_code":   result.ExitCode,
				"stdout":      result.Stdout,
				"stderr":      result.Stderr,
				"duration_ms": result.Duration.Milliseconds(),
			},
		}, nil
	}
	return &ActionResult{
		OK:     false,
		Status: "error",
		Error:  fmt.Sprintf("exit code %d: %s", result.ExitCode, result.Stderr),
	}, nil
}

// PolicyFromActionSpec maps an ActionSpec's DefaultPolicy into a
// sandbox.ExecutionPolicy. Returns nil if spec or spec.DefaultPolicy is nil.
func PolicyFromActionSpec(spec *ActionSpec) *sandbox.ExecutionPolicy {
	if spec == nil || spec.DefaultPolicy == nil {
		return nil
	}
	p := spec.DefaultPolicy
	policy := &sandbox.ExecutionPolicy{
		Timeout: p.Timeout,
	}
	if p.NetworkAccess == "disabled" {
		policy.NetworkAccess.AllowInternet = false
	} else if p.NetworkAccess == "enabled" {
		policy.NetworkAccess.AllowInternet = true
	}
	if len(p.PathAllowlist) > 0 {
		policy.FilesystemAccess.AllowedPaths = p.PathAllowlist
	}
	if len(p.PathDenylist) > 0 {
		policy.FilesystemAccess.DeniedPaths = p.PathDenylist
	}
	policy.ResourceLimit.DiskBytes = int64(p.MaxOutputBytes)
	return policy
}

// buildPolicyFromInput constructs an ExecutionPolicy from input map fields:
// timeout (float64/int/string), network_access ("disabled"/"enabled"),
// path_allowlist ([]string/[]any), and max_output_bytes (int/float64).
func buildPolicyFromInput(input map[string]any) *sandbox.ExecutionPolicy {
	policy := &sandbox.ExecutionPolicy{}
	if v, ok := input["timeout"]; ok {
		switch t := v.(type) {
		case float64:
			policy.Timeout = time.Duration(t) * time.Second
		case int:
			policy.Timeout = time.Duration(t) * time.Second
		case int64:
			policy.Timeout = time.Duration(t) * time.Second
		case string:
			if d, err := time.ParseDuration(t); err == nil {
				policy.Timeout = d
			}
		}
	}
	if v, ok := input["network_access"]; ok {
		if s, ok := v.(string); ok {
			if s == "disabled" {
				policy.NetworkAccess.AllowInternet = false
			} else if s == "enabled" {
				policy.NetworkAccess.AllowInternet = true
			}
		}
	}
	if v, ok := input["path_allowlist"]; ok {
		if paths, err := toStringSlice(v); err == nil {
			policy.FilesystemAccess.AllowedPaths = paths
		}
	}
	if v, ok := input["max_output_bytes"]; ok {
		switch n := v.(type) {
		case float64:
			policy.ResourceLimit.DiskBytes = int64(n)
		case int:
			policy.ResourceLimit.DiskBytes = int64(n)
		case int64:
			policy.ResourceLimit.DiskBytes = n
		}
	}
	return policy
}

// toStringSlice converts an any value (expected to be []string, []any, or nil)
// into a []string. Returns nil for nil input.
func toStringSlice(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []string or []any, got %T", v)
	}
}
