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
	profileName      string
	sid              *syscall.SID
	appToken         syscall.Token
}

// Start initializes the AppContainer environment and launches the process.
// It creates an AppContainer profile, configures filesystem/registry ACLs,
// derives an AppContainer token, and stores the token for Execute to start
// processes under the container identity.
func (h *AppContainerHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.processStarted {
		return fmt.Errorf("already started")
	}

	// Generate a unique profile name from the sandbox directory.
	h.profileName = "inferglow-sandbox-" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Set up the AppContainer environment (profile + ACLs).
	if err := h.setupAppContainerEnvironment(); err != nil {
		h.rollbackProfile()
		return fmt.Errorf("setup AppContainer environment: %w", err)
	}

	// Derive the AppContainer token used to start child processes under the
	// container identity. A failure here must roll the profile back so a
	// half-initialized container is never left behind.
	token, err := createAppContainerToken(h.sid)
	if err != nil {
		h.rollbackProfile()
		return fmt.Errorf("create AppContainer token: %w", err)
	}
	h.appToken = token

	h.status = StatusRunning
	h.processStarted = true
	return nil
}

// rollbackProfile deletes the AppContainer profile and frees the SID after a
// failed initialization, so no persistent system state is left behind.
func (h *AppContainerHandle) rollbackProfile() {
	if h.sid != nil {
		freeSID(h.sid)
		h.sid = nil
	}
	if h.profileName != "" {
		_ = deleteAppContainerProfile(h.profileName)
		h.profileName = ""
	}
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

	// Launch under the AppContainer token via the shared launcher. Fail
	// closed: without a valid container token the process would run with the
	// full caller identity, which defeats the AppContainer isolation.
	if h.appToken == 0 {
		return nil, fmt.Errorf("%w: AppContainer token not initialized (call Start first)", ErrHandleNotRunning)
	}
	return launchProcessWithIO(execCtx, h.appToken, argv, cmd, h.sandboxDir)
}

// Stop terminates the running process and cleans up resources.
// It kills the process, deletes the AppContainer profile, closes the
// AppContainer token, and frees the SID.
func (h *AppContainerHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil {
		_ = h.proc.Kill()
		h.proc = nil
	}

	// Close the AppContainer token if it was created.
	if h.appToken != 0 {
		h.appToken.Close()
		h.appToken = 0
	}

	// Clean up the AppContainer profile.
	if h.profileName != "" {
		_ = deleteAppContainerProfile(h.profileName)
		h.profileName = ""
	}

	// Free the SID allocated by CreateAppContainerProfile (LocalAlloc-backed).
	freeSID(h.sid)
	h.sid = nil

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

// setupAppContainerEnvironment prepares the AppContainer environment.
// It creates an AppContainer profile via userenv.dll, configures filesystem
// ACLs, and sets up registry restrictions.
func (h *AppContainerHandle) setupAppContainerEnvironment() error {
	// Create sandbox directory if it doesn't exist.
	if h.sandboxDir != "" {
		if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
			return fmt.Errorf("create sandbox directory: %w", err)
		}
	}

	// Check if AppContainer API is available.
	if !isAppContainerAvailable() {
		return fmt.Errorf("AppContainer API not available on this system")
	}

	// Create the AppContainer profile via userenv.dll.
	sid, err := createAppContainerProfile(h.profileName)
	if err != nil {
		return fmt.Errorf("create AppContainer profile: %w", err)
	}
	h.sid = sid

	// Configure filesystem access ACLs.
	if err := h.configureFilesystemAccess(); err != nil {
		return fmt.Errorf("configure filesystem access: %w", err)
	}

	// Configure registry access restrictions.
	if err := h.configureRegistryAccess(); err != nil {
		return fmt.Errorf("configure registry access: %w", err)
	}

	return nil
}

// isAppContainerAvailable checks whether the AppContainer API is available.
// It uses feature detection to verify userenv.dll exports exist.
func isAppContainerAvailable() bool {
	loadUserenv()
	// Check if CreateAppContainerProfile exists in userenv.dll.
	return featureDetection(userenv, "CreateAppContainerProfile")
}

// configureFilesystemAccess sets up filesystem isolation for the AppContainer.
// It grants the AppContainer SID GENERIC_ALL access to the sandbox directory
// (merged into the existing DACL) so container processes can work inside it
// while everything else stays deny-by-default.
//
// This is fail-closed: any failure returns an error and the caller rolls the
// AppContainer profile back, so a half-configured container is never left
// behind.
func (h *AppContainerHandle) configureFilesystemAccess() error {
	if h.sandboxDir == "" {
		return nil
	}

	// Ensure the sandbox directory exists.
	if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
		return fmt.Errorf("create sandbox directory: %w", err)
	}

	// Grant the AppContainer SID full access to the sandbox directory,
	// preserving the pre-existing DACL entries.
	if err := grantDirectoryAccess(h.sandboxDir, h.sid); err != nil {
		return fmt.Errorf("grant sandbox directory access: %w", err)
	}
	return nil
}

// configureRegistryAccess documents the registry isolation posture for v1.
//
// AppContainer processes are inherently denied access to the host registry
// hives unless a capability-gated key is explicitly granted, so no explicit
// API call is required for the deny-by-default baseline. A future iteration
// may use AddAppContainerRegistryCapability to whitelist specific hives
// (e.g. HKCU\Software\InferGlow); the current version intentionally ships
// with the default full isolation.
func (h *AppContainerHandle) configureRegistryAccess() error {
	return nil
}
