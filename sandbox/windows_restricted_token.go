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
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
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
	// Duplicate the token under the lock so a concurrent Stop (which closes
	// the original) cannot invalidate the handle while Execute is running.
	var token syscall.Token
	if running {
		dup, err := h.duplicateTokenLocked()
		if err != nil {
			h.mu.Unlock()
			return nil, err
		}
		token = dup
	}
	h.mu.Unlock()
	if token != 0 {
		defer token.Close()
	}

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

	// Fail closed: without the restricted token the process would run with
	// the full caller identity, which is never acceptable for this handle.
	return nil, fmt.Errorf("%w: restricted token not initialized (call Start first)", ErrHandleNotRunning)
}

// duplicateTokenLocked returns a duplicated handle to the restricted token
// that survives a concurrent Stop (which closes the original handle).
// Callers must hold h.mu and must Close the returned handle.
func (h *RestrictedTokenHandle) duplicateTokenLocked() (syscall.Token, error) {
	if h.restrictedToken == 0 {
		return 0, fmt.Errorf("%w: restricted token not initialized (call Start first)", ErrHandleNotRunning)
	}
	p, _ := syscall.GetCurrentProcess()
	var dupHandle syscall.Handle
	if err := syscall.DuplicateHandle(
		syscall.Handle(p),
		syscall.Handle(h.restrictedToken),
		syscall.Handle(p),
		&dupHandle,
		0,  // dwDesiredAccess: ignored with DUPLICATE_SAME_ACCESS
		false, // bInheritHandle
		0x2, // dwOptions: DUPLICATE_SAME_ACCESS keeps the source handle's access rights
	); err != nil {
		return 0, fmt.Errorf("duplicate restricted token: %w", err)
	}
	return syscall.Token(dupHandle), nil
}

// executeWithToken runs a command using CreateProcessAsUser with the
// restricted token, capturing stdout/stderr through anonymous pipes wired
// via STARTF_USESTDHANDLES (see launchProcessWithIO).
func (h *RestrictedTokenHandle) executeWithToken(ctx context.Context, token syscall.Token, argv []string, cmd *Command) (*ExecutionResult, error) {
	return launchProcessWithIO(ctx, token, argv, cmd, h.sandboxDir)
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
