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

//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// SeatbeltProvider provides sandbox handles using macOS Seatbelt (sandbox-exec).
type SeatbeltProvider struct {
	sandboxExecPath string
	available       bool
}

// NewSeatbeltProvider creates a new SeatbeltProvider after verifying sandbox-exec is available.
func NewSeatbeltProvider() *SeatbeltProvider {
	path, err := exec.LookPath("sandbox-exec")
	return &SeatbeltProvider{
		sandboxExecPath: path,
		available:       err == nil,
	}
}

// Name returns the provider name.
func (p *SeatbeltProvider) Name() string { return "seatbelt" }

// Kind returns the provider kind.
func (p *SeatbeltProvider) Kind() string { return "local" }

// InspectAvailability reports whether the Seatbelt provider can be used.
func (p *SeatbeltProvider) InspectAvailability() (*AvailabilityResult, error) {
	return &AvailabilityResult{
		Available:  p.available,
		Platform:   "darwin",
		BinaryPath: p.sandboxExecPath,
	}, nil
}

// CreateHandle creates a new SeatbeltHandle from the given config and policy.
func (p *SeatbeltProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	if !p.available {
		return nil, ErrProviderUnavailable
	}

	seCfg := parseSeatbeltConfig(cfg)
	profile := buildSBPLProfile(seCfg, policy)

	return &SeatbeltHandle{
		config:  seCfg,
		policy:  policy,
		status:  StatusCreated,
		profile: profile,
	}, nil
}

var _ Provider = (*SeatbeltProvider)(nil)

// SeatbeltHandle manages a sandbox-exec process lifecycle.
type SeatbeltHandle struct {
	config     SeatbeltConfig
	policy     *ExecutionPolicy
	profile    string
	status     HandleStatus
	pid        int
	proc       *os.Process
	mu         sync.Mutex
	policyFile string
}

// Start generates the SBPL policy file and prepares the sandbox.
// Note: the actual process is started on first Execute() call.
func (h *SeatbeltHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status == StatusRunning {
		return nil // Already started
	}

	// Write policy to a temp file
	policyPath, cleanup, err := writeSBPLProfile(h.profile)
	if err != nil {
		h.status = StatusError
		return fmt.Errorf("seatbelt handle start: %w", err)
	}

	h.policyFile = policyPath
	h.status = StatusRunning
	_ = cleanup // Cleanup is called in Stop()

	return nil
}

// Execute runs a command inside the sandbox using sandbox-exec.
func (h *SeatbeltHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	h.mu.Lock()
	if h.status != StatusRunning {
		h.mu.Unlock()
		return nil, ErrHandleNotRunning
	}
	policyFile := h.policyFile
	h.mu.Unlock()

	if policyFile == "" {
		return nil, fmt.Errorf("seatbelt handle: policy file not set")
	}

	// Build the sandbox-exec command
	args := []string{"-f", policyFile}
	if cmd != nil {
		args = append(args, cmd.Argv...)
	} else {
		args = append(args, "/bin/true")
	}

	var execTimeout time.Duration
	if h.config.Timeout > 0 {
		execTimeout = time.Duration(h.config.Timeout) * time.Second
	} else {
		execTimeout = 0
	}

	var execCtx context.Context
	var execCancel context.CancelFunc
	if execTimeout > 0 {
		execCtx, execCancel = context.WithTimeout(ctx, execTimeout)
	} else {
		execCtx, execCancel = context.WithTimeout(ctx, 30*time.Second) // default timeout
	}
	defer execCancel()

	startTime := time.Now()
	command := exec.CommandContext(execCtx, args[0], args[1:]...)

	if cmd != nil {
		if cmd.Workdir != "" {
			command.Dir = cmd.Workdir
		}
		if len(cmd.Env) > 0 {
			command.Env = cmd.Env
		}
		if cmd.Stdin != nil {
			command.Stdin = cmd.Stdin
		}
	}

	stdout, err := command.CombinedOutput()
	duration := time.Since(startTime)

	result := &ExecutionResult{
		ExitCode: command.ProcessState.ExitCode(),
		Stdout:   string(stdout),
		Duration: duration,
	}

	if err != nil {
		// CombinedOutput returns an error if the command exits non-zero
		result.Stderr = string(stdout)
		return result, nil
	}

	return result, nil
}

// Stop terminates the sandbox process and cleans up resources.
func (h *SeatbeltHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status != StatusRunning {
		return nil // Already stopped or never started
	}

	// Kill the process if running
	if h.proc != nil {
		h.proc.Kill()
		h.proc = nil
	}

	// Remove the policy file
	if h.policyFile != "" {
		os.Remove(h.policyFile)
		h.policyFile = ""
	}

	h.status = StatusStopped
	return nil
}

// Status returns the current lifecycle state of the handle.
func (h *SeatbeltHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// Cancel terminates the running sandbox process.
func (h *SeatbeltHandle) Cancel(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil {
		h.proc.Kill()
		h.proc = nil
	}
	return nil
}

// PolicyFilePath returns the path to the generated SBPL policy file.
func (h *SeatbeltHandle) PolicyFilePath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.policyFile
}

var _ Handle = (*SeatbeltHandle)(nil)

// RegisterSeatbeltProvider registers the Seatbelt provider into the given Manager.
func RegisterSeatbeltProvider(m *Manager) error {
	provider := NewSeatbeltProvider()
	if provider.available {
		result, err := provider.InspectAvailability()
		if err == nil && result.Available {
			_ = m.Register(provider)
		}
	}
	return nil
}
