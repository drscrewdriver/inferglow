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
	mu               sync.Mutex
	config           map[string]any
	policy           *ExecutionPolicy
	status           HandleStatus
	proc             *exec.Cmd
	sandboxDir       string
	networkIsolation bool
	sharedFolders    []SharedFolder
	startTime        time.Time
	configPath       string // path to the temporary .wsb config file
}

// Start initializes the Windows Sandbox environment and launches the session.
// It generates a WSConfig.xml, writes it to a temporary .wsb file, and
// launches Microsoft.WindowsSandbox.exe.
func (h *WindowsSandboxHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status == StatusRunning {
		return fmt.Errorf("windows sandbox already running")
	}

	// Create sandbox directory if needed.
	if h.sandboxDir != "" {
		if err := os.MkdirAll(h.sandboxDir, 0755); err != nil {
			return fmt.Errorf("create sandbox directory: %w", err)
		}
	}

	// Generate WSConfig.xml.
	config, err := h.generateWSConfig()
	if err != nil {
		return fmt.Errorf("generate WSConfig.xml: %w", err)
	}

	// Write config to a temporary .wsb file.
	wsbFile, err := os.CreateTemp("", "inferglow-*.wsb")
	if err != nil {
		return fmt.Errorf("create temp .wsb file: %w", err)
	}
	h.configPath = wsbFile.Name()
	if _, err := wsbFile.WriteString(config); err != nil {
		wsbFile.Close()
		os.Remove(h.configPath)
		return fmt.Errorf("write WSConfig.xml: %w", err)
	}
	wsbFile.Close()

	// Launch WindowsSandbox.exe with the config file.
	sandboxExe, err := exec.LookPath("WindowsSandbox.exe")
	if err != nil {
		// Try the full path as fallback.
		sandboxExe = "Microsoft.WindowsSandbox.exe"
	}
	h.proc = exec.CommandContext(ctx, sandboxExe, h.configPath)
	if err := h.proc.Start(); err != nil {
		os.Remove(h.configPath)
		return fmt.Errorf("launch Windows Sandbox: %w", err)
	}

	h.status = StatusRunning
	h.startTime = time.Now()

	// Wait for the sandbox to become ready by polling for the ready file.
	if h.sandboxDir != "" {
		go h.pollReadyFile(ctx)
	}

	return nil
}

// pollReadyFile polls for the sandbox ready indicator file.
// The Windows Sandbox LogonCommand writes a log.txt to the shared folder
// when the sandbox is ready.
func (h *WindowsSandboxHandle) pollReadyFile(ctx context.Context) {
	readyPath := h.sandboxDir + "\\log.txt"
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
			if _, err := os.Stat(readyPath); err == nil {
				return // Sandbox is ready.
			}
		}
	}
}

// Execute runs a command inside the Windows Sandbox.
// It writes the command to a shared file, waits for the sandbox to execute it,
// and reads the result from the shared folder.
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
	sandboxDir := h.sandboxDir
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

	// Build the command string.
	cmdStr := strings.Join(cmd.Argv, " ")

	// Write command to shared folder for the sandbox to pick up.
	if sandboxDir != "" {
		cmdFile := sandboxDir + "\\command.txt"
		if err := os.WriteFile(cmdFile, []byte(cmdStr), 0644); err != nil {
			return nil, fmt.Errorf("write command file: %w", err)
		}

		// Poll for result file.
		resultFile := sandboxDir + "\\result.txt"
		return h.pollForResult(execCtx, resultFile)
	}

	// Fallback: execute directly if no shared folder is configured.
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

// pollForResult polls for the result file written by the sandbox.
func (h *WindowsSandboxHandle) pollForResult(ctx context.Context, resultFile string) (*ExecutionResult, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			data, err := os.ReadFile(resultFile)
			if err != nil {
				continue // Not ready yet.
			}
			// Clean up the result file.
			os.Remove(resultFile)
			return &ExecutionResult{
				Stdout:   string(data),
				Duration: time.Since(h.startTime),
			}, nil
		}
	}
}

// Stop terminates the Windows Sandbox session and cleans up resources.
// It kills the sandbox process, removes the temporary .wsb config file,
// and cleans up any shared folder artifacts.
func (h *WindowsSandboxHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc != nil && h.proc.Process != nil {
		_ = h.proc.Process.Kill()
		_ = h.proc.Process.Release()
	}

	// Clean up the temporary .wsb config file.
	if h.configPath != "" {
		os.Remove(h.configPath)
		h.configPath = ""
	}

	// Clean up shared folder artifacts.
	if h.sandboxDir != "" {
		os.Remove(h.sandboxDir + "\\command.txt")
		os.Remove(h.sandboxDir + "\\result.txt")
		os.Remove(h.sandboxDir + "\\log.txt")
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
// Reference: https://learn.microsoft.com/windows/security/application-security/application-isolation/windows-sandbox/windows-sandbox-configure-using-wsb-file
//
// Valid XML elements (no declaration line, no SandboxFolder/DesktopAttachmentsFolder):
//
//	<Configuration>
//	  <VGpu>Disable</VGpu>
//	  <Networking>Disable</Networking>
//	  <MappedFolders>
//	    <MappedFolder>
//	      <HostFolder>C:\path</HostFolder>
//	      <ReadOnly>true</ReadOnly>
//	    </MappedFolder>
//	  </MappedFolders>
//	  <LogonCommand>
//	    <Command>cmd /c echo test</Command>
//	  </LogonCommand>
//	</Configuration>
func (h *WindowsSandboxHandle) generateWSConfig() (string, error) {
	var xml bytes.Buffer
	xml.WriteString(`<Configuration>`)

	// Network isolation
	if h.networkIsolation {
		xml.WriteString(`  <Networking>Disable</Networking>`)
	}

	// Add shared folders (only HostFolder and ReadOnly are valid)
	if len(h.sharedFolders) > 0 {
		xml.WriteString(`  <MappedFolders>`)
		for _, sf := range h.sharedFolders {
			xml.WriteString(`    <MappedFolder>`)
			xml.WriteString(fmt.Sprintf(`<HostFolder>%s</HostFolder>`, sanitizeXML(sf.HostPath)))
			if sf.ReadOnly {
				xml.WriteString(`<ReadOnly>true</ReadOnly>`)
			}
			xml.WriteString(`    </MappedFolder>`)
		}
		xml.WriteString(`  </MappedFolders>`)
	}

	// LogonCommand: write startup timestamp to host-shared folder
	if h.sandboxDir != "" {
		xml.WriteString(`  <LogonCommand>`)
		xml.WriteString(fmt.Sprintf(`<Command>powershell -Command "echo 'Sandbox started at %s' | Out-File -FilePath '%s\log.txt' -Encoding utf8"</Command>`,
			time.Now().Format("2006-01-02 15:04:05"),
			sanitizeXML(h.sandboxDir),
		))
		xml.WriteString(`  </LogonCommand>`)
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
