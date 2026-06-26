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
	"unsafe"
)

// RestrictedTokenHandle implements Handle for restricted token process isolation.
// It creates a restricted access token and launches processes under that token.
type RestrictedTokenHandle struct {
	mu               sync.Mutex
	config           map[string]any
	policy           *ExecutionPolicy
	status           HandleStatus
	proc             *os.Process
	sandboxDir       string
	networkIsolation bool
	processStarted   bool
	restrictedToken  syscall.Token
}

// Start initializes the restricted token environment by creating a restricted
// access token from the current process token with high-privilege privileges
// removed. The token is stored for use in Execute via CreateProcessAsUser.
func (h *RestrictedTokenHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.processStarted {
		return nil // Already started
	}

	// Create the restricted token from the current process token.
	restrictedToken, err := createRestrictedTokenFromCurrent()
	if err != nil {
		return fmt.Errorf("create restricted token: %w", err)
	}
	h.restrictedToken = restrictedToken

	// Create sandbox directory if configured.
	if h.sandboxDir != "" {
		if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
			restrictedToken.Close()
			return fmt.Errorf("create sandbox directory: %w", err)
		}
	}

	h.status = StatusRunning
	h.processStarted = true
	return nil
}

// restrictedToken is the token created by Start for use in Execute.
// It is a syscall.Token (uintptr underneath) to avoid import cycles.
type restrictedTokenHandle struct {
	token syscall.Token
}

// Execute runs a command inside the restricted token sandbox.
// When a restricted token is available, it uses CreateProcessAsUser to
// launch the process under the restricted identity.
func (h *RestrictedTokenHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("command is nil or empty")
	}

	h.mu.Lock()
	running := h.status == StatusRunning
	policy := h.policy
	token := h.restrictedToken
	h.mu.Unlock()

	if !running {
		return nil, fmt.Errorf("%w: call Start first", ErrHandleNotRunning)
	}

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
				// Case-insensitive path containment check (Windows paths are case-insensitive)
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

	// When a restricted token is available, use CreateProcessAsUser.
	if token != 0 {
		return h.executeWithToken(execCtx, token, argv, cmd)
	}

	// Fallback: execute without token restriction (should not happen after Start).
	return h.executeDirect(execCtx, argv, cmd)
}

// executeWithToken runs a command using CreateProcessAsUser with the restricted token.
func (h *RestrictedTokenHandle) executeWithToken(ctx context.Context, token syscall.Token, argv []string, cmd *Command) (*ExecutionResult, error) {
	loadKernel32Procs()

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

	// Resolve the executable path.
	exePath, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("lookpath %q: %w", name, err)
	}

	// Build the command line string.
	cmdLine := exePath
	for _, a := range args {
		cmdLine += " " + syscall.EscapeArg(a)
	}
	cmdLinePtr, _ := syscall.UTF16PtrFromString(cmdLine)

	// Set up working directory.
	workDir := cmd.Workdir
	if workDir == "" {
		workDir = h.sandboxDir
	}
	var workDirPtr *uint16
	if workDir != "" {
		workDirPtr, _ = syscall.UTF16PtrFromString(workDir)
	}

	// Set up environment block.
	var envPtr *uint16
	if len(cmd.Env) > 0 {
		envStr := strings.Join(cmd.Env, "\x00") + "\x00\x00"
		envPtr, _ = syscall.UTF16PtrFromString(envStr)
	}

	// PROCESS_INFORMATION and STARTUPINFO structures.
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	si.Cb = uint32(unsafe.Sizeof(si))

	// CreateProcessAsUserW call.
	r1, _, callErr := procCreateProcessAsUser.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(exePath))),
		uintptr(unsafe.Pointer(cmdLinePtr)),
		0, // lpProcessAttributes
		0, // lpThreadAttributes
		0, // bInheritHandles
		0, // dwCreationFlags
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(unsafe.Pointer(workDirPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("CreateProcessAsUser: %w", callErr)
	}

	// Wait for the process to complete.
	procHandle := pi.Process
	defer syscall.CloseHandle(procHandle)
	defer syscall.CloseHandle(pi.Thread)

	// Use a goroutine to wait for process completion with context support.
	done := make(chan error, 1)
	go func() {
		_, waitErr := syscall.WaitForSingleObject(procHandle, syscall.INFINITE)
		done <- waitErr
	}()

	select {
	case <-ctx.Done():
		syscall.TerminateProcess(procHandle, 1)
		return nil, ctx.Err()
	case waitErr := <-done:
		if waitErr != nil {
			return nil, fmt.Errorf("WaitForSingleObject: %w", waitErr)
		}
	}

	// Get exit code.
	var exitCode uint32
	syscall.GetExitCodeProcess(procHandle, &exitCode)

	// Read stdout/stderr is not directly possible with CreateProcessAsUser
	// without pipe setup. For now, return exit code only.
	return &ExecutionResult{
		ExitCode: int(exitCode),
		Duration: 0, // Duration tracked by caller.
	}, nil
}

// executeDirect runs a command using exec.CommandContext (fallback path).
func (h *RestrictedTokenHandle) executeDirect(ctx context.Context, argv []string, cmd *Command) (*ExecutionResult, error) {
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

	cmdExec := exec.CommandContext(ctx, name, args...)
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
		if ctx.Err() != nil {
			return result, fmt.Errorf("%w: %v", ctx.Err(), runErr)
		}
		return result, nil
	}

	return result, nil
}

// Stop terminates the running process and cleans up resources.
func (h *RestrictedTokenHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil {
		_ = h.proc.Kill()
		h.proc = nil
	}

	// Close the restricted token if it was created.
	if h.restrictedToken != 0 {
		h.restrictedToken.Close()
		h.restrictedToken = 0
	}

	h.status = StatusStopped
	h.processStarted = false
	return nil
}

// Status returns the current handle status.
func (h *RestrictedTokenHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// CreateRestrictedToken creates a restricted access token by removing
// high-privilege privileges from the current process token.
// Uses advapi32.dll: OpenProcessToken → DuplicateTokenEx → CreateRestrictedToken.
func CreateRestrictedToken() (syscall.Token, error) {
	return createRestrictedTokenFromCurrent()
}
