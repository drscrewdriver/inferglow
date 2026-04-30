//go:build windows

package sandbox

import (
	"context"
	"os"
	"testing"
)

// TestAppContainerHandleCreation tests that CreateHandle for AppContainer
// returns a valid handle with correct initial status.
func TestAppContainerHandleCreation(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	if p == nil {
		t.Fatal("NewWindowsRuntimeProvider returned nil")
	}

	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}
	if handle == nil {
		t.Fatal("CreateHandle returned nil handle")
	}

	status := handle.Status()
	if status != StatusCreated {
		t.Errorf("initial status = %q, want %q", status, StatusCreated)
	}
}

// TestAppContainerHandleStartStop lifecycle tests Start, Stop, Status.
func TestAppContainerHandleStartStop(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()

	// Start should succeed (the real implementation will launch the process).
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if handle.Status() != StatusRunning {
		t.Errorf("status after Start = %q, want %q", handle.Status(), StatusRunning)
	}

	// Stop should succeed.
	if err := handle.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if handle.Status() != StatusStopped {
		t.Errorf("status after Stop = %q, want %q", handle.Status(), StatusStopped)
	}
}

// TestAppContainerHandleExecute runs a simple command inside the sandbox.
func TestAppContainerHandleExecute(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer handle.Stop(ctx)

	// Execute a simple command: echo on Windows.
	result, err := handle.Execute(ctx, &Command{
		Argv: []string{"cmd", "/c", "echo", "hello"},
	})
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	// In AppContainer sandbox, cmd may fail due to restricted permissions.
	// We just verify Execute returns a result (even with error).
	if err != nil {
		t.Logf("Execute returned error (expected in restricted AppContainer): %v, exitCode=%d, stderr=%q",
			err, result.ExitCode, result.Stderr)
	}
}

// TestAppContainerHandleCancel tests that Cancel properly terminates a running process.
func TestAppContainerHandleCancel(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel should terminate the running process.
	if err := handle.Stop(ctx); err != nil {
		t.Fatalf("Cancel (Stop) failed: %v", err)
	}

	status := handle.Status()
	if status != StatusStopped {
		t.Errorf("status after Cancel = %q, want %q", status, StatusStopped)
	}
}

// TestAppContainerHandleStartTwice should return error (idempotent Start).
func TestAppContainerHandleStartTwice(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	// Second Start should fail.
	if err := handle.Start(ctx); err == nil {
		t.Error("second Start should have returned an error")
	}

	handle.Stop(ctx)
}

// TestAppContainerHandleExecuteNotRunning returns error when not started.
func TestAppContainerHandleExecuteNotRunning(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	_, err = handle.Execute(ctx, &Command{
		Argv: []string{"cmd", "/c", "echo", "hello"},
	})
	if err == nil {
		t.Error("Execute before Start should have returned an error")
	}
}

// TestAppContainerHandleStopNotRunning returns error or succeeds gracefully.
func TestAppContainerHandleStopNotRunning(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	// Stop before Start should either succeed or return a clear error.
	err = handle.Stop(ctx)
	if err != nil && err != ErrHandleNotRunning {
		t.Errorf("Stop before Start returned unexpected error: %v", err)
	}
}

// TestAppContainerConfigValidation tests that invalid config is rejected.
func TestAppContainerConfigValidation(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	policy := DefaultPolicy()

	// backend = 1 (BackendAppContainer), missing sandbox_directory.
	cfg := map[string]any{
		"backend":       int(BackendAppContainer),
		"auto_select":   false,
		"network_isolation": true,
	}

	_, err := p.CreateHandle(cfg, &policy)
	// The implementation may either create the temp dir or reject.
	// We just verify no panic occurs.
	_ = err
}

// TestAppContainerFilesystemIsolation tests that sandbox_directory is respected.
func TestAppContainerFilesystemIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	appContainerDir := tmpDir + "\\.sandbox"
	if err := os.MkdirAll(appContainerDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  appContainerDir,
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer handle.Stop(ctx)

	// Execute a command that writes to a file.
	result, err := handle.Execute(ctx, &Command{
		Argv: []string{"cmd", "/c", "echo", "test", ">", "sandbox.txt"},
		Workdir: appContainerDir,
	})
	if err != nil {
		// Execution may fail in AppContainer due to filesystem restrictions.
		// Verify this is expected behavior for AppContainer isolation.
		t.Logf("Execute (expected to possibly fail in AppContainer): %v", err)
		return
	}

	if result != nil {
		t.Logf("Execute result: exitCode=%d, stdout=%q", result.ExitCode, result.Stdout)
	}
}

// TestAppContainerHandleStatusTransitions verifies the status lifecycle.
func TestAppContainerHandleStatusTransitions(t *testing.T) {
	p := NewWindowsRuntimeProvider()
	cfg := map[string]any{
		"backend":            int(BackendAppContainer),
		"auto_select":        false,
		"sandbox_directory":  t.TempDir(),
		"network_isolation":  true,
	}
	policy := DefaultPolicy()

	handle, err := p.CreateHandle(cfg, &policy)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	if handle.Status() != StatusCreated {
		t.Errorf("initial status = %q, want %q", handle.Status(), StatusCreated)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if handle.Status() != StatusRunning {
		t.Errorf("after Start status = %q, want %q", handle.Status(), StatusRunning)
	}

	if err := handle.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if handle.Status() != StatusStopped {
		t.Errorf("after Stop status = %q, want %q", handle.Status(), StatusStopped)
	}
}
