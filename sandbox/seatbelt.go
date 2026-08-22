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
	"path/filepath"
	"sync"
	"time"
)

// seatbeltLoaderEnv is the environment variable that overrides the
// seatbelt-loader binary path resolution.
const seatbeltLoaderEnv = "INFERGLOW_SEATBELT_LOADER"

// SeatbeltProvider provides sandbox handles using macOS Seatbelt.
//
// The preferred backend is the built-in seatbelt-loader binary, which calls
// the private libsandbox API (sandbox_init) directly and no longer depends on
// the deprecated sandbox-exec CLI. If the loader is missing or its --self-test
// fails, the provider falls back to sandbox-exec when the CLI is still present
// on the system; if neither is usable the provider is unavailable (fail-closed).
type SeatbeltProvider struct {
	loaderPath      string // resolved seatbelt-loader binary (preferred)
	sandboxExecPath string // fallback sandbox-exec CLI (deprecated but may exist)
	available       bool
}

// NewSeatbeltProvider creates a new SeatbeltProvider.
//
// Detection order:
//  1. resolve the seatbelt-loader binary (build output dir → PATH → env var)
//     and verify it with `--self-test`;
//  2. fall back to the sandbox-exec CLI if the loader is unusable;
//  3. neither → available=false (fail-closed, no unconstrained passthrough).
func NewSeatbeltProvider() *SeatbeltProvider {
	p := &SeatbeltProvider{}

	if loader := resolveLoaderPath(); loader != "" && loaderSelfTest(loader) {
		p.loaderPath = loader
		p.available = true
		return p
	}

	if path, err := exec.LookPath("sandbox-exec"); err == nil {
		p.sandboxExecPath = path
		p.available = true
	}
	return p
}

// resolveLoaderPath resolves the seatbelt-loader binary in order:
// build output dir (next to the running executable) → PATH → env var.
func resolveLoaderPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, cand := range []string{
			filepath.Join(dir, "bin", "seatbelt-loader"),
			filepath.Join(dir, "seatbelt-loader"),
		} {
			if isExecutableFile(cand) {
				return cand
			}
		}
	}
	if path, err := exec.LookPath("seatbelt-loader"); err == nil {
		return path
	}
	return os.Getenv(seatbeltLoaderEnv)
}

// isExecutableFile reports whether path exists and is executable.
func isExecutableFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0111 != 0
}

// loaderSelfTest verifies that the loader binary can apply an empty profile.
func loaderSelfTest(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, "--self-test").Run() == nil
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
		BinaryPath: p.binaryPath(),
	}, nil
}

// binaryPath returns the backend binary actually used for execution.
func (p *SeatbeltProvider) binaryPath() string {
	if p.loaderPath != "" {
		return p.loaderPath
	}
	return p.sandboxExecPath
}

// CreateHandle creates a new SeatbeltHandle from the given config and policy.
func (p *SeatbeltProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	if !p.available {
		return nil, ErrProviderUnavailable
	}

	seCfg := parseSeatbeltConfig(cfg)
	profile := buildSBPLProfile(seCfg, policy)

	return &SeatbeltHandle{
		config:          seCfg,
		policy:          policy,
		status:          StatusCreated,
		profile:         profile,
		loaderPath:      p.loaderPath,
		sandboxExecPath: p.sandboxExecPath,
	}, nil
}

var _ Provider = (*SeatbeltProvider)(nil)

// SeatbeltHandle manages a sandboxed process lifecycle. The sandbox is
// applied either through the built-in seatbelt-loader binary (preferred) or
// through the deprecated sandbox-exec CLI as a fallback.
type SeatbeltHandle struct {
	config          SeatbeltConfig
	policy          *ExecutionPolicy
	profile         string
	status          HandleStatus
	pid             int
	proc            *os.Process
	mu              sync.Mutex
	policyFile      string
	loaderPath      string
	sandboxExecPath string
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

// Execute runs a command inside the sandbox using the seatbelt backend.
// The backend binary is the built-in seatbelt-loader when available, falling
// back to the deprecated sandbox-exec CLI otherwise; both honor the same
// policy file and command semantics.
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

	// Build the backend command: seatbelt-loader <policy-file> <cmd> <args...>
	// or, as a fallback, sandbox-exec -f <policy-file> <cmd> <args...>.
	var args []string
	if h.loaderPath != "" {
		args = []string{h.loaderPath, policyFile}
	} else {
		args = []string{h.sandboxExecPath, "-f", policyFile}
	}
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
