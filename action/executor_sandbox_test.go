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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/approval"
	"github.com/inferglow/sandbox"
)

// --- Fakes ---

// fakeHandle implements sandbox.Handle for unit tests.
type fakeHandle struct {
	startErr   error
	execResult *sandbox.ExecutionResult
	execErr    error
	stopErr    error
	started    bool
	stopped    bool
	executeCmd *sandbox.Command
}

func (h *fakeHandle) Start(ctx context.Context) error {
	h.started = true
	return h.startErr
}

func (h *fakeHandle) Execute(ctx context.Context, cmd *sandbox.Command) (*sandbox.ExecutionResult, error) {
	h.executeCmd = cmd
	return h.execResult, h.execErr
}

func (h *fakeHandle) Stop(ctx context.Context) error {
	h.stopped = true
	return h.stopErr
}

func (h *fakeHandle) Status() sandbox.HandleStatus {
	if h.stopped {
		return sandbox.StatusStopped
	}
	if h.started {
		return sandbox.StatusRunning
	}
	return sandbox.StatusCreated
}

// fakeProvider implements sandbox.Provider for unit tests.
type fakeProvider struct {
	name   string
	handle sandbox.Handle
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Kind() string { return "fake" }
func (p *fakeProvider) InspectAvailability() (*sandbox.AvailabilityResult, error) {
	return &sandbox.AvailabilityResult{Available: true, Platform: "test"}, nil
}
func (p *fakeProvider) CreateHandle(cfg map[string]any, policy *sandbox.ExecutionPolicy) (sandbox.Handle, error) {
	return p.handle, nil
}

// newTestManager builds a Manager with a fakeProvider registered under "trusted_local".
func newTestManager(handle sandbox.Handle) *sandbox.Manager {
	m := sandbox.NewManager()
	_ = m.Register(&fakeProvider{name: "trusted_local", handle: handle})
	return m
}

// --- Tests ---

func TestSandboxExecutorSuccess(t *testing.T) {
	h := &fakeHandle{
		execResult: &sandbox.ExecutionResult{
			ExitCode: 0,
			Stdout:   "hello\n",
			Stderr:   "",
			Duration: 5 * time.Millisecond,
		},
	}
	m := newTestManager(h)
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:     m,
		DefaultMode: sandbox.ModeTrustedLocal,
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv": []any{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want %q", res.Status, "success")
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T (%v)", res.Result, res.Result)
	}
	if exitCode, _ := result["exit_code"].(int); exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", result["exit_code"])
	}
	if stdout, _ := result["stdout"].(string); stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", stdout, "hello\n")
	}
	if stderr, _ := result["stderr"].(string); stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if _, ok := result["duration_ms"].(int64); !ok {
		t.Errorf("duration_ms = %T (%v), want int64", result["duration_ms"], result["duration_ms"])
	}
	if !h.started {
		t.Errorf("expected handle.Start to be called")
	}
	if !h.stopped {
		t.Errorf("expected handle.Stop to be called")
	}
	if h.executeCmd == nil {
		t.Fatal("execute cmd is nil")
	}
	if len(h.executeCmd.Argv) != 2 || h.executeCmd.Argv[0] != "echo" || h.executeCmd.Argv[1] != "hello" {
		t.Errorf("Argv = %v, want [echo hello]", h.executeCmd.Argv)
	}
}

func TestSandboxExecutorFailure(t *testing.T) {
	h := &fakeHandle{
		execResult: &sandbox.ExecutionResult{
			ExitCode: 1,
			Stdout:   "",
			Stderr:   "boom",
			Duration: 2 * time.Millisecond,
		},
	}
	m := newTestManager(h)
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:     m,
		DefaultMode: sandbox.ModeTrustedLocal,
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv": []any{"false"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	wantErr := "exit code 1: boom"
	if res.Error != wantErr {
		t.Errorf("Error = %q, want %q", res.Error, wantErr)
	}
	if !h.stopped {
		t.Errorf("expected handle.Stop to be called even on failure")
	}
}

func TestSandboxExecutorApprovalRejected(t *testing.T) {
	approvalMgr := approval.NewPolicyApprovalManager()
	approvalMgr.SetPolicy(&approval.AccessPolicy{
		DeniedCapabilities: []string{"trusted_local"},
	})
	// Manager with no providers — approval should block before CreateHandle.
	m := sandbox.NewManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalManager: approvalMgr,
		DefaultMode:     sandbox.ModeTrustedLocal,
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv":              []any{"echo", "hello"},
		"approval_required": true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "blocked" {
		t.Errorf("Status = %q, want %q", res.Status, "blocked")
	}
	if res.Error != "approval rejected" {
		t.Errorf("Error = %q, want %q", res.Error, "approval rejected")
	}
}

func TestSandboxExecutorApprovalPending(t *testing.T) {
	// No policy and no handler → pending record.
	approvalMgr := approval.NewPolicyApprovalManager()
	m := sandbox.NewManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalManager: approvalMgr,
		DefaultMode:     sandbox.ModeTrustedLocal,
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv":              []any{"echo", "hello"},
		"approval_required": true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "blocked" {
		t.Errorf("Status = %q, want %q", res.Status, "blocked")
	}
	if !strings.HasPrefix(res.Error, "pending approval: ") {
		t.Errorf("Error = %q, want prefix %q", res.Error, "pending approval: ")
	}
}

func TestPolicyFromActionSpec(t *testing.T) {
	spec := &ActionSpec{
		DefaultPolicy: &ActionPolicy{
			Timeout:        30 * time.Second,
			NetworkAccess:  "disabled",
			PathAllowlist:  []string{"/tmp", "/var"},
			MaxOutputBytes: 4096,
		},
	}
	policy := PolicyFromActionSpec(spec)
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}
	if policy.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", policy.Timeout, 30*time.Second)
	}
	if policy.NetworkAccess.AllowInternet {
		t.Errorf("AllowInternet = true, want false")
	}
	if len(policy.FilesystemAccess.AllowedPaths) != 2 ||
		policy.FilesystemAccess.AllowedPaths[0] != "/tmp" ||
		policy.FilesystemAccess.AllowedPaths[1] != "/var" {
		t.Errorf("AllowedPaths = %v, want [/tmp /var]", policy.FilesystemAccess.AllowedPaths)
	}
	if policy.ResourceLimit.DiskBytes != 4096 {
		t.Errorf("DiskBytes = %d, want 4096", policy.ResourceLimit.DiskBytes)
	}
}

func TestSandboxExecutorIntegrationTrustedLocalEcho(t *testing.T) {
	// Integration test: real TrustedLocalProvider + Manager executing echo.
	m := sandbox.NewManager()
	if err := m.Register(sandbox.NewTrustedLocalProvider()); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:     m,
		DefaultMode: sandbox.ModeTrustedLocal,
	})

	var argv []any
	if runtime.GOOS == "windows" {
		argv = []any{"cmd", "/c", "echo", "hello"}
	} else {
		argv = []any{"echo", "hello"}
	}

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv": argv,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want %q", res.Status, "success")
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any: %T", res.Result)
	}
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, want contains %q", stdout, "hello")
	}
}

// TestNewSandboxExecutor_DefaultDenyBaseline verifies that when no Baseline is
// supplied the executor substitutes sandbox.DefaultDenyBaseline() so that
// security is the default rather than an opt-in.
func TestNewSandboxExecutor_DefaultDenyBaseline(t *testing.T) {
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:     sandbox.NewManager(),
		DefaultMode: sandbox.ModeTrustedLocal,
	})
	if !exec.cfg.Baseline.ApprovalRequired {
		t.Errorf("default baseline ApprovalRequired = false, want true")
	}
	if exec.cfg.Baseline.NetworkAccess != sandbox.NetworkAccessNone {
		t.Errorf("default baseline NetworkAccess = %q, want %q", exec.cfg.Baseline.NetworkAccess, sandbox.NetworkAccessNone)
	}
	if exec.cfg.Baseline.Timeout != 30*time.Second {
		t.Errorf("default baseline Timeout = %v, want 30s", exec.cfg.Baseline.Timeout)
	}
	if exec.cfg.Baseline.MaxOutputBytes != 1<<20 {
		t.Errorf("default baseline MaxOutputBytes = %d, want %d", exec.cfg.Baseline.MaxOutputBytes, 1<<20)
	}
}

// TestBuildPolicyFromInput_BaselineClampsLLM verifies that LLM-generated input
// attempting to LOOSEN every policy field is clamped back to the server-side
// baseline. The LLM must never be able to widen the policy surface.
func TestBuildPolicyFromInput_BaselineClampsLLM(t *testing.T) {
	baseline := sandbox.ServerPolicyBaseline{
		Timeout:        10 * time.Second,
		NetworkAccess:  sandbox.NetworkAccessNone,
		PathAllowlist:  []string{"/tmp", "/var"},
		MaxOutputBytes: 4096,
	}
	// LLM tries to loosen every field.
	input := map[string]any{
		"timeout":          600, // 600s > 10s baseline
		"network_access":   "full",
		"path_allowlist":   []any{"/tmp", "/var", "/etc"}, // /etc not in baseline
		"max_output_bytes": 99999,                          // > 4096 baseline
	}

	p := buildPolicyFromInput(input, baseline)

	if p.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s (clamped to baseline)", p.Timeout)
	}
	if p.NetworkAccess.Level != sandbox.NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q (more restrictive wins)", p.NetworkAccess.Level, sandbox.NetworkAccessNone)
	}
	if p.NetworkAccess.AllowInternet {
		t.Errorf("AllowInternet = true, want false for none level")
	}
	wantPaths := []string{"/tmp", "/var"}
	if len(p.FilesystemAccess.AllowedPaths) != len(wantPaths) {
		t.Errorf("AllowedPaths = %v, want %v (/etc dropped, not in baseline)", p.FilesystemAccess.AllowedPaths, wantPaths)
	} else {
		for i, want := range wantPaths {
			if p.FilesystemAccess.AllowedPaths[i] != want {
				t.Errorf("AllowedPaths[%d] = %q, want %q", i, p.FilesystemAccess.AllowedPaths[i], want)
			}
		}
	}
	if p.ResourceLimit.DiskBytes != 4096 {
		t.Errorf("DiskBytes = %d, want 4096 (clamped to baseline)", p.ResourceLimit.DiskBytes)
	}
}

// TestBuildPolicyFromInput_LLMTightensAccepted verifies that LLM-generated
// input that TIGHTENS every field (relative to a permissive baseline) is
// honoured. The LLM may always make the policy stricter.
func TestBuildPolicyFromInput_LLMTightensAccepted(t *testing.T) {
	baseline := sandbox.ServerPolicyBaseline{
		Timeout:        60 * time.Second,
		NetworkAccess:  sandbox.NetworkAccessFull,
		PathAllowlist:  []string{"/tmp", "/var", "/etc"},
		MaxOutputBytes: 1 << 20,
	}
	// LLM tightens every field.
	input := map[string]any{
		"timeout":          5, // 5s < 60s baseline
		"network_access":   "none",
		"path_allowlist":   []any{"/tmp"}, // subset of baseline
		"max_output_bytes": 1024,          // < 1MiB baseline
	}

	p := buildPolicyFromInput(input, baseline)

	if p.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s (LLM tightening accepted)", p.Timeout)
	}
	if p.NetworkAccess.Level != sandbox.NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q (LLM tightening accepted)", p.NetworkAccess.Level, sandbox.NetworkAccessNone)
	}
	if len(p.FilesystemAccess.AllowedPaths) != 1 || p.FilesystemAccess.AllowedPaths[0] != "/tmp" {
		t.Errorf("AllowedPaths = %v, want [/tmp] (LLM subset accepted)", p.FilesystemAccess.AllowedPaths)
	}
	if p.ResourceLimit.DiskBytes != 1024 {
		t.Errorf("DiskBytes = %d, want 1024 (LLM tightening accepted)", p.ResourceLimit.DiskBytes)
	}
}

// TestBuildPolicyFromInput_EmptyInputUsesBaseline ensures that when the LLM
// supplies no policy fields the server-side baseline is used verbatim.
func TestBuildPolicyFromInput_EmptyInputUsesBaseline(t *testing.T) {
	baseline := sandbox.ServerPolicyBaseline{
		Timeout:        30 * time.Second,
		NetworkAccess:  sandbox.NetworkAccessEgressOnly,
		PathAllowlist:  []string{"/data"},
		MaxOutputBytes: 8192,
	}
	p := buildPolicyFromInput(map[string]any{}, baseline)

	if p.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s (baseline)", p.Timeout)
	}
	if p.NetworkAccess.Level != sandbox.NetworkAccessEgressOnly {
		t.Errorf("NetworkAccess.Level = %q, want %q (baseline)", p.NetworkAccess.Level, sandbox.NetworkAccessEgressOnly)
	}
	if len(p.FilesystemAccess.AllowedPaths) != 1 || p.FilesystemAccess.AllowedPaths[0] != "/data" {
		t.Errorf("AllowedPaths = %v, want [/data] (baseline)", p.FilesystemAccess.AllowedPaths)
	}
	if p.ResourceLimit.DiskBytes != 8192 {
		t.Errorf("DiskBytes = %d, want 8192 (baseline)", p.ResourceLimit.DiskBytes)
	}
}

// TestSandboxExecutorApprovalServerSideDecision_LLMFalseIgnored verifies
// (Task 4) that when the server-side baseline requires approval, an LLM input
// of approval_required=false cannot disable the approval flow.
func TestSandboxExecutorApprovalServerSideDecision_LLMFalseIgnored(t *testing.T) {
	h := &fakeHandle{
		execResult: &sandbox.ExecutionResult{ExitCode: 0, Stdout: "ok\n"},
	}
	m := newTestManager(h)
	// No policy and no handler → any submitted request ends up "pending".
	approvalMgr := approval.NewPolicyApprovalManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalManager: approvalMgr,
		DefaultMode:     sandbox.ModeTrustedLocal,
		Baseline: sandbox.ServerPolicyBaseline{
			ApprovalRequired: true,
			NetworkAccess:    sandbox.NetworkAccessFull,
			Timeout:          60 * time.Second,
		},
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv":              []any{"echo", "ok"},
		"approval_required": false, // must be ignored — server requires approval
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Status != "blocked" {
		t.Fatalf("expected Status=%q (approval triggered despite LLM false), got %+v", "blocked", res)
	}
	if !strings.HasPrefix(res.Error, "pending approval: ") {
		t.Errorf("Error = %q, want prefix %q", res.Error, "pending approval: ")
	}
	if h.started {
		t.Errorf("handle.Start was called despite approval being required — executor must not run unapproved commands")
	}
}

// TestSandboxExecutorApprovalServerSideDecision_LLMTrueIgnored verifies
// (Task 4) that when the server-side baseline does NOT require approval, an
// LLM input of approval_required=true cannot force the approval flow. The
// command executes directly.
func TestSandboxExecutorApprovalServerSideDecision_LLMTrueIgnored(t *testing.T) {
	h := &fakeHandle{
		execResult: &sandbox.ExecutionResult{ExitCode: 0, Stdout: "ok\n"},
	}
	m := newTestManager(h)
	approvalMgr := approval.NewPolicyApprovalManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalManager: approvalMgr,
		DefaultMode:     sandbox.ModeTrustedLocal,
		Baseline: sandbox.ServerPolicyBaseline{
			ApprovalRequired: false,
			NetworkAccess:    sandbox.NetworkAccessFull,
			Timeout:          60 * time.Second,
		},
	})

	res, err := exec.Execute(context.Background(), map[string]any{
		"argv":              []any{"echo", "ok"},
		"approval_required": true, // must be ignored — server does not require approval
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Status == "blocked" {
		t.Errorf("approval was triggered despite baseline.ApprovalRequired=false; got %+v", res)
	}
	if !res.OK {
		t.Errorf("expected OK=true (approval bypassed, command executed), got %+v", res)
	}
	if !h.started {
		t.Errorf("expected handle.Start to be called (command should execute without approval)")
	}
}
