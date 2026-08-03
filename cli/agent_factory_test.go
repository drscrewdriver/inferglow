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

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/audit"
)

// TestBuildAgentWithAudit verifies that when audit is enabled, buildAgent
// returns a non-nil AuditChain and the agent engine uses it.
func TestBuildAgentWithAudit(t *testing.T) {
	// Create a temporary data directory.
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	auditDir := filepath.Join(dataDir, "audit")

	cfg := DefaultCLIConfig()
	cfg.DataDir = dataDir
	cfg.Audit.Enabled = true
	cfg.Audit.StoragePath = auditDir
	cfg.LLM.Endpoint = "http://localhost:99999/v1"
	cfg.LLM.Model = "test-model"
	cfg.LLM.APIKey = "test-key"

	// Create the audit chain first (same path as BuildRuntime).
	auditCfg := audit.AuditConfig{
		Enabled:        true,
		StorageBackend: "json_file",
		StoragePath:    auditDir,
	}
	ac, err := audit.NewAuditChain(auditCfg)
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}

	bridge, err := NewMemoryBridge(cfg, "test-session-audit")
	if err != nil {
		t.Fatalf("NewMemoryBridge: %v", err)
	}
	defer bridge.OnSessionEnd(context.Background())

	ag, returnedAC, err := buildAgent(cfg, bridge, "test-session-audit", ac)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}
	if ag == nil {
		t.Fatal("buildAgent returned nil agent")
	}
	if returnedAC == nil {
		t.Fatal("buildAgent returned nil AuditChain when audit was enabled")
	}
	if !returnedAC.IsEnabled() {
		t.Fatal("AuditChain.IsEnabled() returned false, expected true")
	}
}

// TestBuildAgentWithoutAudit verifies that when audit is disabled (default),
// buildAgent returns nil for the AuditChain (zero overhead).
func TestBuildAgentWithoutAudit(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")

	cfg := DefaultCLIConfig()
	cfg.DataDir = dataDir
	cfg.Audit.Enabled = false // default, explicit for clarity
	cfg.LLM.Endpoint = "http://localhost:99999/v1"
	cfg.LLM.Model = "test-model"
	cfg.LLM.APIKey = "test-key"

	bridge, err := NewMemoryBridge(cfg, "test-session-noaudit")
	if err != nil {
		t.Fatalf("NewMemoryBridge: %v", err)
	}
	defer bridge.OnSessionEnd(context.Background())

	ag, returnedAC, err := buildAgent(cfg, bridge, "test-session-noaudit", nil)
	if err != nil {
		t.Fatalf("buildAgent: %v", err)
	}
	if ag == nil {
		t.Fatal("buildAgent returned nil agent")
	}
	if returnedAC != nil {
		t.Fatal("buildAgent returned non-nil AuditChain when audit was disabled")
	}
}

// TestDefaultConfigAuditDisabled verifies that the default config has audit
// disabled, maintaining backward compatibility.
func TestDefaultConfigAuditDisabled(t *testing.T) {
	cfg := DefaultCLIConfig()
	if cfg.Audit.Enabled {
		t.Error("DefaultCLIConfig().Audit.Enabled should be false")
	}
}

// TestEnsureDataDirsCreatesAuditDir verifies that EnsureDataDirs creates the
// audit/ subdirectory.
func TestEnsureDataDirsCreatesAuditDir(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow-test")

	if err := EnsureDataDirs(dataDir); err != nil {
		t.Fatalf("EnsureDataDirs: %v", err)
	}

	auditDir := filepath.Join(dataDir, "audit")
	if _, err := os.Stat(auditDir); os.IsNotExist(err) {
		t.Fatalf("audit directory %s was not created", auditDir)
	}
}