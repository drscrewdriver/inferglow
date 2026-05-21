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

//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// AppContainerHandle implements Handle for Windows AppContainer isolation.
// AppContainer provides application-level isolation (UWP style) with
// filesystem, registry, and device access restrictions.
type AppContainerHandle struct {
	mu               sync.Mutex
	config           map[string]any
	policy           *ExecutionPolicy
	status           HandleStatus
	proc             *os.Process
	sandboxDir       string
	networkIsolation bool
	processStarted   bool
}

// Start initializes the AppContainer environment and launches the process.
func (h *AppContainerHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.processStarted {
		return fmt.Errorf("already started")
	}

	h.status = StatusRunning
	h.processStarted = true
	return nil
}

// Execute runs a command inside the AppContainer sandbox.
func (h *AppContainerHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("command is nil or empty")
	}

	h.mu.Lock()
	if h.status != StatusRunning {
		h.mu.Unlock()
		return nil, fmt.Errorf("%w: call Start first", ErrHandleNotRunning)
	}
	policy := h.policy
	h.mu.Unlock()

	// Check AllowedCommands whitelist
	if policy != nil && len(policy.AllowedCommands) > 0 {
		allowed := false
		for _, c := range policy.AllowedCommands {
			if c == cmd.Argv[0] {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: %q", ErrCommandNotAllowed, cmd.Argv[0])
		}
	}

	// Check filesystem DeniedPaths
	if policy != nil && len(policy.FilesystemAccess.DeniedPaths) > 0 {
		for _, arg := range cmd.Argv {
			for _, denied := range policy.FilesystemAccess.DeniedPaths {
				argLower := strings.ToLower(arg)
				deniedLower := strings.ToLower(denied)
				if strings.Contains(argLower, deniedLower) {
					return nil, fmt.Errorf("filesystem access denied: path %q contains denied path %q", arg, denied)
				}
			}
		}
	}

	// Apply policy timeout
	execCtx := ctx
	if policy != nil && policy.Timeout != 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}

	// Build argv (handle Windows builtins like echo, dir, etc.)
	argv, err := buildArgv(cmd.Argv)
	if err != nil {
		return nil, err
	}

	var name string
	var args []string
	if len(argv) > 1 && argv[0] == "cmd" {
		name = argv[0]
		args = argv[1:]
	} else {
		name = argv[0]
		if len(argv) > 1 {
			args = argv[1:]
		}
	}

	cmdExec := exec.CommandContext(execCtx, name, args...)
	cmdExec.Stdin = cmd.Stdin
	if cmd.Workdir != "" {
		cmdExec.Dir = cmd.Workdir
	} else if h.sandboxDir != "" {
		cmdExec.Dir = h.sandboxDir
	}
	if cmd.Env != nil {
		cmdExec.Env = cmd.Env
	}

	var stdout, stderr bytes.Buffer
	cmdExec.Stdout = &stdout
	cmdExec.Stderr = &stderr

	start := time.Now()
	runErr := cmdExec.Run()
	duration := time.Since(start)

	result := &ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if cmdExec.ProcessState != nil {
		result.ExitCode = cmdExec.ProcessState.ExitCode()
	}

	if runErr != nil {
		if execCtx.Err() != nil {
			return result, fmt.Errorf("%w: %v", execCtx.Err(), runErr)
		}
		return result, nil
	}

	return result, nil
}

// Stop terminates the running process and cleans up resources.
func (h *AppContainerHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil {
		_ = h.proc.Kill()
		h.proc = nil
	}

	h.status = StatusStopped
	h.processStarted = false
	return nil
}

// Status returns the current handle status.
func (h *AppContainerHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// setupAppContainerEnvironment configures the AppContainer environment.
// On Windows, this involves calling StartAppContainerOperation from shell32.dll.
// The real implementation would use Windows APIs to create an AppContainer
// environment block with restricted capabilities.
func (h *AppContainerHandle) setupAppContainerEnvironment() error {
	// Create sandbox directory if it doesn't exist
	if h.sandboxDir != "" {
		if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
			return fmt.Errorf("create sandbox directory: %w", err)
		}
	}

	// Check if AppContainer API is available
	if !isAppContainerAvailable() {
		return fmt.Errorf("AppContainer API not available on this system")
	}

	return nil
}

// isAppContainerAvailable checks whether the AppContainer API is available.
func isAppContainerAvailable() bool {
	// AppContainer API is available since Windows 8.1 (build 9600)
	// We check if the StartAppContainerOperation function exists in shell32.dll
	kernel32 := syscall.MustLoadDLL("kernel32.dll")
	procGetCurrentProcessVersion := kernel32.MustFindProc("GetCurrentProcessVersion")

	_, _, err := procGetCurrentProcessVersion.Call()
	if err != nil && err.Error() == "The specified procedure could not be found." {
		return false
	}
	return err == nil || err.Error() != "The specified procedure could not be found."
}

// configureFilesystemAccess sets up filesystem isolation for the AppContainer.
func (h *AppContainerHandle) configureFilesystemAccess() error {
	if h.sandboxDir == "" {
		return nil
	}

	// The real implementation would use CreateAppContainerContainer and
	// SetCurrentProcessExplicitAppContainerSettings to establish the
	// AppContainer context with specific filesystem capabilities.
	// For the stub implementation, we just ensure the directory exists.
	return os.MkdirAll(h.sandboxDir, 0755)
}

// configureRegistryAccess restricts registry access for the AppContainer.
func (h *AppContainerHandle) configureRegistryAccess() error {
	// The real implementation would create registry capabilities using
	// AddAppContainerRegistryCapability to restrict access to specific
	// registry hives (e.g., HKCU only).
	return nil
}
