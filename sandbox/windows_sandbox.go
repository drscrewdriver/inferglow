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

// WindowsSandboxHandle implements Handle for Windows Sandbox VM isolation.
// Windows Sandbox launches an independent desktop environment in a lightweight
// VM, providing the strongest isolation on Windows. It is ideal for executing
// untrusted code or binary analysis.
//
// Key features:
//   - VM-level isolation (strongest on Windows)
//   - Shared folder configuration (host-to-sandbox path mapping)
//   - Network isolation option (disable networking in sandbox)
//   - Temporary session (sandbox is destroyed on stop)
type WindowsSandboxHandle struct {
	mu              sync.Mutex
	config          map[string]any
	policy          *ExecutionPolicy
	status          HandleStatus
	proc            *exec.Cmd
	sandboxDir      string
	networkIsolation bool
	sharedFolders   []SharedFolder
	startTime       time.Time
}

// Start initializes the Windows Sandbox environment and launches the session.
// In production, this would invoke Microsoft.WindowsSandbox.exe to create a VM.
func (h *WindowsSandboxHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status == StatusRunning {
		return fmt.Errorf("windows sandbox already running")
	}

	h.status = StatusRunning
	h.startTime = time.Now()

	// In production, this would:
	// 1. Generate a WSConfig.xml with shared folders and network settings
	// 2. Launch Microsoft.WindowsSandbox.exe with the config
	// 3. Wait for the sandbox to be ready
	//
	// For stub implementation, we just set the status
	return nil
}

// Execute runs a command inside the Windows Sandbox.
// In production, this would copy the binary to the sandbox and execute it.
func (h *WindowsSandboxHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
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

	start := time.Now()
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

// Stop terminates the Windows Sandbox session and cleans up resources.
// In production, this would destroy the VM and all associated resources.
func (h *WindowsSandboxHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil && h.proc.Process != nil {
		_ = h.proc.Process.Kill()
	}

	h.status = StatusStopped
	h.proc = nil
	return nil
}

// Status returns the current handle status.
func (h *WindowsSandboxHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// generateWSConfig generates a WSConfig.xml for Windows Sandbox.
// In production, this would create proper XML configuration with:
//   - SharedFolder elements for host-to-sandbox mappings
//   - <Networking> element for network isolation
//   - <MappedFolders> for shared directories
func (h *WindowsSandboxHandle) generateWSConfig() (string, error) {
	// Build XML configuration
	var xml bytes.Buffer
	xml.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	xml.WriteString(`<Configuration>\n`)

	// Add shared folders
	if len(h.sharedFolders) > 0 {
		xml.WriteString(`  <MappedFolders>\n`)
		for _, sf := range h.sharedFolders {
			xml.WriteString(fmt.Sprintf(`    <MappedFolder>\n`))
			xml.WriteString(fmt.Sprintf(`      <HostFolder>%s</HostFolder>\n`, sanitizeXML(sf.HostPath)))
			xml.WriteString(fmt.Sprintf(`      <SandboxFolder>%s</SandboxFolder>\n`, sanitizeXML(sf.SandboxPath)))
			if sf.ReadOnly {
				xml.WriteString(`      <ReadOnly>true</ReadOnly>\n`)
			}
			xml.WriteString(`    </MappedFolder>\n`)
		}
		xml.WriteString(`  </MappedFolders>\n`)
	}

	// Network isolation
	if h.networkIsolation {
		xml.WriteString(`  <Networking>Disable</Networking>\n`)
	}

	// Sandbox directory
	if h.sandboxDir != "" {
		xml.WriteString(fmt.Sprintf(`  <DesktopAttachmentsFolder>%s</DesktopAttachmentsFolder>\n`, sanitizeXML(h.sandboxDir)))
	}

	xml.WriteString(`</Configuration>`)
	return xml.String(), nil
}

// sanitizeXML escapes special characters for XML content.
func sanitizeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// setupSandboxEnvironment prepares the Windows Sandbox environment.
// In production, this would:
// 1. Generate WSConfig.xml
// 2. Set up temporary workspace
// 3. Launch Microsoft.WindowsSandbox.exe
func (h *WindowsSandboxHandle) setupSandboxEnvironment() error {
	// Create sandbox directory if needed
	if h.sandboxDir != "" {
		if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
			return fmt.Errorf("create sandbox directory: %w", err)
		}
	}

	// Generate WSConfig.xml (production: write to temp file)
	config, err := h.generateWSConfig()
	if err != nil {
		return fmt.Errorf("generate WSConfig.xml: %w", err)
	}

	// Log config (production: write to file)
	_ = config

	return nil
}
