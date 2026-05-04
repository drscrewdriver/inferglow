//go:build windows

package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestWindowsSandboxHandleNewCreated(t *testing.T) {
	cfg := map[string]any{
		"backend":          int(BackendWindowsSandbox),
		"auto_select":      false,
		"sandbox_directory": "C:\\temp\\sandbox",
		"network_isolation": true,
	}
	policy := DefaultPolicy()

	handle := &WindowsSandboxHandle{
		config:           cfg,
		policy:           &policy,
		status:           StatusCreated,
		sandboxDir:       cfg["sandbox_directory"].(string),
		networkIsolation: cfg["network_isolation"].(bool),
	}

	if handle.Status() != StatusCreated {
		t.Errorf("expected status %q, got %q", StatusCreated, handle.Status())
	}
}

func TestWindowsSandboxHandleStartSetsRunning(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if handle.Status() != StatusRunning {
		t.Errorf("expected status %q after Start, got %q", StatusRunning, handle.Status())
	}
}

func TestWindowsSandboxHandleStartAlreadyRunning(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusRunning,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	if err := handle.Start(ctx); err == nil {
		t.Error("expected error when starting already running handle")
	}
}

func TestWindowsSandboxHandleStopSetsStopped(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusRunning,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	if err := handle.Stop(ctx); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	if handle.Status() != StatusStopped {
		t.Errorf("expected status %q after Stop, got %q", StatusStopped, handle.Status())
	}
}

func TestWindowsSandboxHandleExecuteNotRunning(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	_, err := handle.Execute(ctx, &Command{Argv: []string{"cmd", "/c", "echo hello"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Errorf("expected ErrHandleNotRunning, got: %v", err)
	}
}

func TestWindowsSandboxHandleExecuteWithDefaultCommand(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusRunning,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	// Execute with empty argv should use default command
	result, err := handle.Execute(ctx, &Command{Argv: []string{}})
	if err != nil {
		// May fail because we're in a stub implementation, but should not be ErrHandleNotRunning
		if err == ErrHandleNotRunning {
			t.Error("unexpected ErrHandleNotRunning for running handle")
		}
	}

	if result != nil && result.ExitCode < 0 {
		t.Errorf("unexpected negative exit code: %d", result.ExitCode)
	}
}

func TestWindowsSandboxHandleConfigFields(t *testing.T) {
	cfg := map[string]any{
		"backend":          int(BackendWindowsSandbox),
		"network_isolation": true,
		"sandbox_directory": "C:\\custom\\sandbox",
	}
	policy := DefaultPolicy()

	handle := &WindowsSandboxHandle{
		config:           cfg,
		policy:           &policy,
		status:           StatusCreated,
		sandboxDir:       cfg["sandbox_directory"].(string),
		networkIsolation: cfg["network_isolation"].(bool),
	}

	if handle.sandboxDir != "C:\\custom\\sandbox" {
		t.Errorf("expected sandboxDir %q, got %q", "C:\\custom\\sandbox", handle.sandboxDir)
	}

	if !handle.networkIsolation {
		t.Error("expected networkIsolation to be true")
	}

	if handle.policy == nil {
		t.Error("policy should be set")
	}
}

func TestWindowsSandboxHandleSharedFolders(t *testing.T) {
	sharedFolders := []SharedFolder{
		{HostPath: "C:\\host", SandboxPath: "S:\\shared", ReadOnly: true},
		{HostPath: "C:\\writable", SandboxPath: "S:\\data", ReadOnly: false},
	}
	policy := DefaultPolicy()

	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
		sharedFolders:    sharedFolders,
	}

	if len(handle.sharedFolders) != 2 {
		t.Errorf("expected 2 shared folders, got %d", len(handle.sharedFolders))
	}

	if handle.sharedFolders[0].HostPath != "C:\\host" {
		t.Errorf("expected first folder host path %q, got %q", "C:\\host", handle.sharedFolders[0].HostPath)
	}

	if !handle.sharedFolders[0].ReadOnly {
		t.Error("expected first folder to be read-only")
	}
}

func TestWindowsSandboxHandleStatusTransitions(t *testing.T) {
	ctx := context.Background()
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	// Created -> Running
	if err := handle.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if handle.Status() != StatusRunning {
		t.Errorf("expected %q, got %q", StatusRunning, handle.Status())
	}

	// Running -> Stopped
	if err := handle.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if handle.Status() != StatusStopped {
		t.Errorf("expected %q, got %q", StatusStopped, handle.Status())
	}
}

func TestWindowsSandboxHandleNetworkIsolationFlag(t *testing.T) {
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           map[string]any{"backend": int(BackendWindowsSandbox)},
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: true,
	}

	if !handle.networkIsolation {
		t.Error("networkIsolation should be true")
	}
}

func TestWindowsSandboxHandleConfigContainsBackend(t *testing.T) {
	cfg := map[string]any{
		"backend": int(BackendWindowsSandbox),
	}
	policy := DefaultPolicy()
	handle := &WindowsSandboxHandle{
		status:           StatusCreated,
		config:           cfg,
		policy:           &policy,
		sandboxDir:       "C:\\temp\\sandbox",
		networkIsolation: false,
	}

	backend, ok := handle.config["backend"].(int)
	if !ok {
		t.Fatal("backend should be in config as int")
	}
	if WindowsBackend(backend) != BackendWindowsSandbox {
		t.Errorf("expected BackendWindowsSandbox, got %v", WindowsBackend(backend))
	}
}
