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
	"log"
	"strings"
	"time"

	"github.com/inferglow/approval"
	"github.com/inferglow/sandbox"
)

// SandboxExecutorConfig configures a SandboxExecutor.
type SandboxExecutorConfig struct {
	Manager         *sandbox.Manager
	ApprovalManager *approval.PolicyApprovalManager
	DefaultMode     sandbox.SandboxMode
	// Baseline is the server-side, deny-by-default policy cap. LLM-generated
	// input may only tighten policy fields relative to this baseline. When
	// left unset (zero value), NewSandboxExecutor substitutes
	// sandbox.DefaultDenyBaseline() so that security is the default.
	Baseline sandbox.ServerPolicyBaseline
}

// SandboxExecutor bridges sandbox.Manager to the action.ActionExecutor
// contract, exposing sandboxed command execution as an Action.
type SandboxExecutor struct {
	cfg SandboxExecutorConfig
}

// NewSandboxExecutor constructs a new SandboxExecutor.
//
// If cfg.DefaultMode is empty it defaults to sandbox.ModeTrustedLocal.
// If cfg.Baseline is the zero value it defaults to
// sandbox.DefaultDenyBaseline() (deny-by-default).
func NewSandboxExecutor(cfg SandboxExecutorConfig) *SandboxExecutor {
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = sandbox.ModeTrustedLocal
	}
	if cfg.Baseline.IsZero() {
		cfg.Baseline = sandbox.DefaultDenyBaseline()
	}
	return &SandboxExecutor{cfg: cfg}
}

// Execute implements ActionExecutor. It parses the input map into a
// sandbox.Command, optionally consults the ApprovalManager, runs the command
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

	policy := buildPolicyFromInput(input, e.cfg.Baseline)

	// approval_required is a server-side decision driven by the policy
	// baseline; the LLM-generated input map has no say in whether approval
	// is triggered.
	if e.cfg.ApprovalManager != nil && e.cfg.Baseline.ApprovalRequired {
		req := &approval.Request{
			RequestID:  "",
			Source:     "sandbox_executor",
			Capability: string(mode),
			Subject:    string(mode),
			Payload: map[string]any{
				"provider": string(mode),
			},
		}
		record, err := e.cfg.ApprovalManager.Submit(req)
		if err != nil {
			return &ActionResult{OK: false, Status: "error", Error: fmt.Sprintf("approval submit: %s", err.Error())}, nil
		}
		switch record.Status {
		case approval.DecisionPending:
			return &ActionResult{OK: false, Status: "blocked", Error: "pending approval: " + record.ID}, nil
		case approval.DecisionDenied:
			return &ActionResult{OK: false, Status: "blocked", Error: "approval rejected"}, nil
		case approval.DecisionApproved, approval.DecisionAllowed:
			// fall through to execute
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

// buildPolicyFromInput constructs an ExecutionPolicy by merging LLM-generated
// input map fields against the server-side ServerPolicyBaseline. The LLM input
// may only TIGHTEN the policy (make it more restrictive) relative to the
// baseline; any attempt to loosen a field is clamped back to the baseline and
// logged.
//
// Recognised input fields:
//
//	timeout          (float64/int/int64/string duration)
//	network_access   ("none"/"egress_only"/"full"; legacy "disabled"/"enabled")
//	path_allowlist   ([]string / []any)
//	max_output_bytes (int/float64/int64)
func buildPolicyFromInput(input map[string]any, baseline sandbox.ServerPolicyBaseline) *sandbox.ExecutionPolicy {
	policy := &sandbox.ExecutionPolicy{}

	// --- Timeout: effective = min(baseline, llm). 0 means "not specified". ---
	llmTimeout := parseTimeout(input["timeout"])
	switch {
	case baseline.Timeout > 0 && llmTimeout > 0:
		if llmTimeout > baseline.Timeout {
			log.Printf("sandbox: LLM requested timeout %v exceeds baseline %v; clamping to baseline", llmTimeout, baseline.Timeout)
			policy.Timeout = baseline.Timeout
		} else {
			policy.Timeout = llmTimeout
		}
	case baseline.Timeout > 0:
		policy.Timeout = baseline.Timeout
	case llmTimeout > 0:
		policy.Timeout = llmTimeout
	}

	// --- NetworkAccess: take the more restrictive level. ---
	baseLevel := baseline.NetworkAccess
	llmLevel := parseNetworkLevel(input["network_access"])
	effectiveLevel := sandbox.MoreRestrictiveNetwork(baseLevel, llmLevel)
	policy.NetworkAccess.Level = effectiveLevel
	// AllowInternet is derived for backends that still consult the bool. none
	// → no internet; egress_only / full → internet (egress is a superset of
	// none but still needs the stack up; isolation is expressed via Level).
	policy.NetworkAccess.AllowInternet = effectiveLevel != sandbox.NetworkAccessNone
	if llmLevel != "" && llmLevel.Rank() > effectiveLevel.Rank() {
		log.Printf("sandbox: LLM requested network_access %q exceeds baseline %q; clamping to %q", llmLevel, baseLevel, effectiveLevel)
	}

	// --- PathAllowlist: intersection of baseline and LLM allowlist. ---
	llmPaths, _ := toStringSlice(input["path_allowlist"])
	policy.FilesystemAccess.AllowedPaths = intersectAllowlist(baseline.PathAllowlist, llmPaths)

	// --- MaxOutputBytes: effective = min(baseline, llm). 0 means "not specified". ---
	llmMax := parseMaxOutputBytes(input["max_output_bytes"])
	switch {
	case baseline.MaxOutputBytes > 0 && llmMax > 0:
		if llmMax > baseline.MaxOutputBytes {
			log.Printf("sandbox: LLM requested max_output_bytes %d exceeds baseline %d; clamping to baseline", llmMax, baseline.MaxOutputBytes)
			policy.ResourceLimit.DiskBytes = baseline.MaxOutputBytes
		} else {
			policy.ResourceLimit.DiskBytes = llmMax
		}
	case baseline.MaxOutputBytes > 0:
		policy.ResourceLimit.DiskBytes = baseline.MaxOutputBytes
	case llmMax > 0:
		policy.ResourceLimit.DiskBytes = llmMax
	}

	return policy
}

// parseTimeout parses the timeout input field into a time.Duration.
// Returns 0 if the value is absent or unrecognised.
func parseTimeout(v any) time.Duration {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return time.Duration(t) * time.Second
	case int:
		return time.Duration(t) * time.Second
	case int64:
		return time.Duration(t) * time.Second
	case string:
		if d, err := time.ParseDuration(t); err == nil {
			return d
		}
	}
	return 0
}

// parseNetworkLevel parses the network_access input field into a
// NetworkAccessLevel. It accepts the canonical "none"/"egress_only"/"full"
// values as well as the legacy "disabled"/"enabled" aliases. Returns the
// empty string when the value is absent or unrecognised.
func parseNetworkLevel(v any) sandbox.NetworkAccessLevel {
	s, ok := v.(string)
	if !ok || s == "" {
		return ""
	}
	switch s {
	case "none", "disabled":
		return sandbox.NetworkAccessNone
	case "egress_only":
		return sandbox.NetworkAccessEgressOnly
	case "full", "enabled":
		return sandbox.NetworkAccessFull
	}
	return ""
}

// parseMaxOutputBytes parses the max_output_bytes input field into an int64.
// Returns 0 if the value is absent or unrecognised.
func parseMaxOutputBytes(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

// intersectAllowlist returns the effective path allowlist: the intersection of
// the baseline and LLM-provided lists. A nil/empty baseline means "no
// server-side restriction" (the LLM list is used as-is). A nil/empty LLM list
// means "LLM did not constrain paths" (the baseline list is used). Paths the
// LLM requests that are not in the baseline are dropped and logged.
func intersectAllowlist(baselinePaths, llmPaths []string) []string {
	if len(baselinePaths) == 0 {
		return llmPaths
	}
	if len(llmPaths) == 0 {
		return baselinePaths
	}
	allowed := make(map[string]struct{}, len(baselinePaths))
	for _, p := range baselinePaths {
		allowed[p] = struct{}{}
	}
	result := make([]string, 0, len(llmPaths))
	for _, p := range llmPaths {
		if _, ok := allowed[p]; ok {
			result = append(result, p)
		} else {
			log.Printf("sandbox: LLM requested path %q not in baseline allowlist; dropping", p)
		}
	}
	return result
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
