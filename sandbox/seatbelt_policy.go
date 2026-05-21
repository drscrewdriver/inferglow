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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SeatbeltConfig holds type-safe configuration for a Seatbelt sandbox.
type SeatbeltConfig struct {
	// Timeout is the execution timeout in seconds (0 means no timeout).
	Timeout int `json:"timeout"`
	// NetworkAllowOutbound controls whether outbound network is allowed.
	NetworkAllowOutbound bool `json:"network_allow_outbound"`
	// WritablePaths lists directories the sandbox process can write to.
	WritablePaths []string `json:"writable_paths"`
	// ProtectedPaths lists system paths that should be deny-written.
	ProtectedPaths []string `json:"protected_paths"`
	// DenyReadPaths lists paths the sandbox process must not read.
	DenyReadPaths []string `json:"deny_read_paths"`
	// ExtraSBPLRules is an optional raw SBPL snippet appended at the end.
	ExtraSBPLRules string `json:"extra_sbpl_rules"`
	// PythonBinary overrides the Python interpreter path (used for shebang resolution).
	PythonBinary string `json:"python_binary"`
}

// buildSBPLProfile generates a complete SBPL (Seatbelt Policy Language) profile
// from the provided config and execution policy, following last-match-wins semantics.
//
// The profile is built in the following order:
//  1. (deny default) — global deny
//  2. Basic capabilities (process-exec, fork, signal, sysctl, mach-lookup, etc.)
//  3. (allow file-read*) — global file read allow
//  4. WritablePaths whitelist — (allow file-write*)
//  5. Temporary directory always writable
//  6. Device files allow
//  7. ProtectedPaths — (deny file-write*)
//  8. DenyReadPaths — (deny file-read*) + (deny file-write*)
//  9. Network policy — allow or deny outbound
//
// 10. Extra custom rules from ExtraSBPLRules
func buildSBPLProfile(cfg SeatbeltConfig, policy *ExecutionPolicy) string {
	var sb strings.Builder

	// 1. Global deny
	sb.WriteString("(deny default)\n\n")

	// 2. Basic capabilities
	sb.WriteString(";; Basic capabilities\n")
	sb.WriteString("(allow process-exec\n")
	sb.WriteString("  (regex #\"^(.+/)?\"+)\n")
	sb.WriteString(")\n")
	sb.WriteString("(allow fork)\n")
	sb.WriteString("(allow signal self)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow ipc-posix-shm)\n")
	sb.WriteString("(allow file-read* metadata)\n\n")

	// 3. Global file read allow
	sb.WriteString(";; File read allow\n")
	sb.WriteString("(allow file-read*\n")
	sb.WriteString("  (subpath \"/\")\n")
	sb.WriteString(")\n\n")

	// Merge policy paths into config
	writablePaths := make([]string, len(cfg.WritablePaths))
	copy(writablePaths, cfg.WritablePaths)
	protectedPaths := make([]string, len(cfg.ProtectedPaths))
	copy(protectedPaths, cfg.ProtectedPaths)
	denyReadPaths := make([]string, len(cfg.DenyReadPaths))
	copy(denyReadPaths, cfg.DenyReadPaths)

	if policy != nil {
		if policy.FilesystemAccess.AllowedPaths != nil {
			writablePaths = append(writablePaths, policy.FilesystemAccess.AllowedPaths...)
		}
		if policy.FilesystemAccess.DeniedPaths != nil {
			// DeniedPaths become both protected and deny-read
			protectedPaths = append(protectedPaths, policy.FilesystemAccess.DeniedPaths...)
			denyReadPaths = append(denyReadPaths, policy.FilesystemAccess.DeniedPaths...)
		}
	}

	// 4. WritablePaths whitelist
	if len(writablePaths) > 0 {
		sb.WriteString(";; Writable paths\n")
		sb.WriteString("(allow file-write*\n")
		for _, p := range writablePaths {
			if resolved := realPath(p); resolved != "" {
				sb.WriteString(fmt.Sprintf("  (subpath \"%s\")\n", resolved))
			}
		}
		sb.WriteString(")\n\n")
	}

	// 5. Temporary directory always writable
	sb.WriteString(";; Temp directory writable\n")
	sb.WriteString("(allow file-write*\n")
	sb.WriteString(fmt.Sprintf("  (subpath \"%s\")\n", realPathOr(os.TempDir(), "/tmp")))
	sb.WriteString(")\n\n")

	// 6. Device files allow
	sb.WriteString(";; Device access\n")
	sb.WriteString("(allow file-read*\n")
	sb.WriteString("  (subpath \"/dev/\")\n")
	sb.WriteString(")\n\n")

	// 7. Protected paths deny write
	if len(protectedPaths) > 0 {
		sb.WriteString(";; Protected paths (deny write)\n")
		sb.WriteString("(deny file-write*\n")
		for _, p := range protectedPaths {
			if resolved := realPath(p); resolved != "" {
				sb.WriteString(fmt.Sprintf("  (subpath \"%s\")\n", resolved))
			}
		}
		sb.WriteString(")\n\n")
	}

	// 8. Deny read paths
	if len(denyReadPaths) > 0 {
		sb.WriteString(";; Deny read paths\n")
		sb.WriteString("(deny file-read*\n")
		for _, p := range denyReadPaths {
			if resolved := realPath(p); resolved != "" {
				sb.WriteString(fmt.Sprintf("  (subpath \"%s\")\n", resolved))
			}
		}
		sb.WriteString(")\n")
		sb.WriteString("(deny file-write*\n")
		for _, p := range denyReadPaths {
			if resolved := realPath(p); resolved != "" {
				sb.WriteString(fmt.Sprintf("  (subpath \"%s\")\n", resolved))
			}
		}
		sb.WriteString(")\n\n")
	}

	// 9. Network policy
	sb.WriteString(";; Network policy\n")
	if cfg.NetworkAllowOutbound {
		sb.WriteString("(allow network-outbound)\n")
	} else {
		sb.WriteString("(deny network-outbound)\n")
	}
	sb.WriteString("\n")

	// 10. Extra custom rules
	if cfg.ExtraSBPLRules != "" {
		sb.WriteString(";; Custom rules\n")
		sb.WriteString(cfg.ExtraSBPLRules)
		sb.WriteString("\n")
	}

	return sb.String()
}

// realPath resolves a path through symlink evaluation to prevent subpath bypass attacks.
// Returns empty string if resolution fails.
func realPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return resolved
}

// realPathOr returns the resolved path or fallback if resolution fails.
func realPathOr(path, fallback string) string {
	if resolved := realPath(path); resolved != "" {
		return resolved
	}
	return fallback
}

// parseSeatbeltConfig converts a map[string]any into a typed SeatbeltConfig.
func parseSeatbeltConfig(m map[string]any) SeatbeltConfig {
	cfg := SeatbeltConfig{}
	if m == nil {
		return cfg
	}

	if v, ok := m["timeout"].(int); ok {
		cfg.Timeout = v
	}
	if v, ok := m["network_allow_outbound"].(bool); ok {
		cfg.NetworkAllowOutbound = v
	}
	if v, ok := m["writable_paths"].([]string); ok {
		cfg.WritablePaths = v
	}
	if v, ok := m["protected_paths"].([]string); ok {
		cfg.ProtectedPaths = v
	}
	if v, ok := m["deny_read_paths"].([]string); ok {
		cfg.DenyReadPaths = v
	}
	if v, ok := m["extra_sbpl_rules"].(string); ok {
		cfg.ExtraSBPLRules = v
	}
	if v, ok := m["python_binary"].(string); ok {
		cfg.PythonBinary = v
	}

	return cfg
}

// writeSBPLProfile writes the generated SBPL profile to a temporary file.
// Returns the file path and a cleanup function.
func writeSBPLProfile(profile string) (string, func(), error) {
	tmpFile, err := os.CreateTemp("", "seatbelt-*.sbpl")
	if err != nil {
		return "", nil, fmt.Errorf("create temp policy file: %w", err)
	}

	if _, err := tmpFile.WriteString(profile); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil, fmt.Errorf("write policy file: %w", err)
	}
	tmpFile.Close()

	cleanup := func() {
		os.Remove(tmpFile.Name())
	}

	return tmpFile.Name(), cleanup, nil
}
