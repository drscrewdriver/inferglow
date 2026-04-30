package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// ErrCommandNotAllowed returned when policy.AllowedCommands rejects Argv[0].
var ErrCommandNotAllowed = errors.New("command not allowed by policy")

// TrustedLocalProvider is a no-isolation Provider that runs commands
// directly on the host. Suitable for development and trusted code only.
type TrustedLocalProvider struct{}

// NewTrustedLocalProvider constructs a new TrustedLocalProvider.
func NewTrustedLocalProvider() *TrustedLocalProvider {
	return &TrustedLocalProvider{}
}

// Name returns "trusted_local".
func (p *TrustedLocalProvider) Name() string { return "trusted_local" }

// Kind returns "local".
func (p *TrustedLocalProvider) Kind() string { return "local" }

// InspectAvailability always returns Available=true with the current OS as Platform.
func (p *TrustedLocalProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available: true,
		Platform:  string(DetectOS()),
	}, nil
}

// CreateHandle returns a new TrustedLocalHandle bound to the given policy.
func (p *TrustedLocalProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	if policy == nil {
		def := DefaultPolicy()
		policy = &def
	}
	return &TrustedLocalHandle{
		policy: policy,
		status: StatusCreated,
	}, nil
}

// TrustedLocalHandle is a Handle that runs commands directly on the host.
type TrustedLocalHandle struct {
	mu     sync.Mutex
	policy *ExecutionPolicy
	status HandleStatus
}

// Start transitions the handle to StatusRunning.
func (h *TrustedLocalHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = StatusRunning
	return nil
}

// Stop transitions the handle to StatusStopped.
func (h *TrustedLocalHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = StatusStopped
	return nil
}

// Status returns the current handle status.
func (h *TrustedLocalHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// Execute runs the given command on the host.
//
// Returns ErrHandleNotRunning if Start was not called.
// Returns ErrCommandNotAllowed if policy.AllowedCommands is non-empty and
// Argv[0] is not in the whitelist.
// Applies policy.Timeout if non-zero.
func (h *TrustedLocalHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
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
	// Apply Timeout
	execCtx := ctx
	var cancel context.CancelFunc
	if policy != nil && policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}
	// Build exec.Cmd
	argv, err := buildArgv(cmd.Argv)
	if err != nil {
		return nil, err
	}
	var name string
	var args []string
	if runtime.GOOS == "windows" && len(argv) > 1 && argv[0] == "cmd" {
		// Already wrapped
		name = argv[0]
		args = argv[1:]
	} else {
		name = argv[0]
		if len(argv) > 1 {
			args = argv[1:]
		}
	}
	c := exec.CommandContext(execCtx, name, args...)
	if cmd.Workdir != "" {
		c.Dir = cmd.Workdir
	}
	if len(cmd.Env) > 0 {
		c.Env = cmd.Env
	}
	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	start := time.Now()
	runErr := c.Run()
	duration := time.Since(start)
	result := &ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}
	if c.ProcessState != nil {
		result.ExitCode = c.ProcessState.ExitCode()
	}
	if runErr != nil {
		if execCtx.Err() != nil {
			return result, fmt.Errorf("%w: %v", execCtx.Err(), runErr)
		}
		// Non-zero exit code is not an error from Execute's perspective;
		// caller can inspect result.ExitCode. But propagate context errors.
		return result, nil
	}
	return result, nil
}

// buildArgv preprocesses the argv to handle Windows builtins (echo, type, cd, etc.)
// by wrapping them with "cmd /c" so they actually execute.
func buildArgv(argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty argv")
	}
	if runtime.GOOS == "windows" {
		// List of Windows cmd.exe builtins that need wrapping
		builtins := map[string]bool{
			"echo": true, "type": true, "cd": true, "dir": true,
			"set": true, "cls": true, "copy": true, "del": true,
			"mkdir": true, "rmdir": true, "ren": true, "rem": true,
			"exit": true, "ver": true, "vol": true, "path": true,
			"prompt": true, "title": true, "color": true, "date": true,
			"time": true, "start": true, "call": true, "shift": true,
			"for": true, "if": true, "goto": true, "md": true, "rd": true,
		}
		first := argv[0]
		if builtins[first] {
			// Wrap with cmd /c
			wrapped := append([]string{"cmd", "/c"}, argv...)
			return wrapped, nil
		}
	}
	return argv, nil
}
