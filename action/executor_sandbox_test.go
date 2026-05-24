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
	policy := &sandbox.ApprovalPolicy{
		BlocklistedProviders: []string{"trusted_local"},
	}
	approvalSvc := sandbox.NewApprovalService(policy)
	// Manager with no providers — approval should block before CreateHandle.
	m := sandbox.NewManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalService: approvalSvc,
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
	// Policy with no auto-approved modes and no blocklisted providers → pending.
	policy := &sandbox.ApprovalPolicy{}
	approvalSvc := sandbox.NewApprovalService(policy)
	m := sandbox.NewManager()
	exec := NewSandboxExecutor(SandboxExecutorConfig{
		Manager:         m,
		ApprovalService: approvalSvc,
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
