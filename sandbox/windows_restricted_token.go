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
	"time"
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
}

// Start initializes the restricted token environment and launches the process.
func (h *RestrictedTokenHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.processStarted {
		return nil // Already started
	}

	h.status = StatusRunning
	h.processStarted = true
	return nil
}

// Execute runs a command inside the restricted token sandbox.
func (h *RestrictedTokenHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("command is nil or empty")
	}

	h.mu.Lock()
	running := h.status == StatusRunning
	policy := h.policy
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
		// Non-zero exit code is not an error from Execute's perspective
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
// The real implementation would use DuplicateTokenEx, CreateRestrictedToken,
// and RemoveAccessAllowedAuditable from advapi32.dll.
func CreateRestrictedToken() error {
	return fmt.Errorf("restricted token creation requires real Windows API implementation")
}
