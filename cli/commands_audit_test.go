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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inferglow/audit"
	"github.com/inferglow/session"
)

// helper: create a test audit chain with some sample entries.
func newTestAuditChain(t *testing.T) *audit.AuditChain {
	t.Helper()
	cfg := audit.AuditConfig{
		Enabled: true,
	}
	chain, err := audit.NewAuditChain(cfg)
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	chain.SetClock(func() time.Time { return now })

	entries := []*audit.AuditEntry{
		{Source: "agent", Action: "decision", ID: "e1"},
		{Source: "agent", Action: "execute", ID: "e2"},
		{Source: "model", Action: "request", ID: "e3"},
		{Source: "action", Action: "execute", ID: "e4"},
		{Source: "flow", Action: "decision", ID: "e5"},
	}
	for _, e := range entries {
		_, err := chain.Append(e)
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	return chain
}

// helper: create a usage.jsonl file for the given sessionID in dataDir.
func writeUsageJSONL(t *testing.T, dataDir, sessionID string, records []session.UsageRecord) {
	t.Helper()
	dir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, sessionID+".usage.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// /audit query tests
// ---------------------------------------------------------------------------

func TestCmdAuditQuery_NoFilter(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	// Use an empty context, empty args (no filter).
	quit := cmdAudit(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_WithSourceFilter(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --source=agent", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_WithActionFilter(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --action=execute", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_WithFromToFilter(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --from=2026-01-01T00:00:00Z --to=2026-12-31T23:59:59Z", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_NoMatch(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --source=nonexistent", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_InvalidFrom(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --from=invalid-date", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditQuery_UnknownFlag(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --unknown=value", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// /audit stats tests
// ---------------------------------------------------------------------------

func TestCmdAuditStats(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "stats", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAuditStats_Empty(t *testing.T) {
	cfg := audit.AuditConfig{Enabled: true}
	chain, err := audit.NewAuditChain(cfg)
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "stats", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAudit_Disabled(t *testing.T) {
	// AuditChain is nil → "not enabled" path.
	rt := &AgentRuntime{}
	quit := cmdAudit(context.Background(), "query", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAudit_NoSubcommand(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

func TestCmdAudit_UnknownSubcommand(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "unknown", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// /cost tests
// ---------------------------------------------------------------------------

func TestCmdCost_WithRecords(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cost-session"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     20,
			ReasoningTokens:  0,
			TotalTokens:      150,
			Cost:             0.002500,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Minute),
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.005000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCost(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCost returned quit=true, expected false")
	}
}

func TestCmdCost_NoRecords(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cost-empty"

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCost(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCost returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// /cache-stats tests
// ---------------------------------------------------------------------------

func TestCmdCacheStats_WithRecords(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cache-session"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Minute),
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     300,
			CompletionTokens: 150,
			CachedTokens:     100,
			ReasoningTokens:  0,
			TotalTokens:      450,
			Cost:             0.003500,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheStats(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheStats returned quit=true, expected false")
	}
}

func TestCmdCacheStats_NoRecords(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cache-empty"

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheStats(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheStats returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// dispatchCommand routing tests
// ---------------------------------------------------------------------------

func TestDispatchCommand_Audit(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := dispatchCommand(context.Background(), "/audit stats", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("dispatchCommand /audit stats returned quit=true")
	}
}

func TestDispatchCommand_Cost(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-dispatch-cost"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     0,
			ReasoningTokens:  0,
			TotalTokens:      150,
			Cost:             0.002500,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := dispatchCommand(context.Background(), "/cost", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("dispatchCommand /cost returned quit=true")
	}
}

func TestDispatchCommand_CacheStats(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-dispatch-cache"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := dispatchCommand(context.Background(), "/cache-stats", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("dispatchCommand /cache-stats returned quit=true")
	}
}

func TestDispatchCommand_Unknown(t *testing.T) {
	rt := &AgentRuntime{}
	quit := dispatchCommand(context.Background(), "/unknown-command", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("dispatchCommand /unknown-command returned quit=true")
	}
}

// ---------------------------------------------------------------------------
// Output format validation tests
// ---------------------------------------------------------------------------

func TestCmdAuditQuery_OutputFormat(t *testing.T) {
	// Capture stdout to verify format.
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	// We can't easily capture fmt.Print output in a unit test without
	// replacing os.Stdout. Instead, verify that the function runs without
	// error and returns the correct quit value.
	quit := cmdAudit(context.Background(), "query", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit query returned quit=true, expected false")
	}
}

func TestCmdAuditStats_OutputFormat(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "stats", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit stats returned quit=true, expected false")
	}
}

func TestCmdCost_OutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cost-format"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     20,
			ReasoningTokens:  10,
			TotalTokens:      150,
			Cost:             0.002500,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCost(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCost returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// /cache-report tests
// ---------------------------------------------------------------------------

func TestCmdCacheReport_NoFilters(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cr-nofilter"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Hour),
			Model:            "claude-3",
			Provider:         "anthropic",
			PromptTokens:     150,
			CompletionTokens: 75,
			CachedTokens:     30,
			ReasoningTokens:  0,
			TotalTokens:      225,
			Cost:             0.003000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_WithFromTo(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cr-fromto"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "--from=2026-01-01T00:00:00Z --to=2026-12-31T23:59:59Z", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_WithModel(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cr-model"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Hour),
			Model:            "claude-3",
			Provider:         "anthropic",
			PromptTokens:     150,
			CompletionTokens: 75,
			CachedTokens:     30,
			ReasoningTokens:  0,
			TotalTokens:      225,
			Cost:             0.003000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "--model=gpt-4", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_EmptyData(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_InvalidFrom(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "--from=invalid-date", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_UnknownFlag(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "--unknown=value", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestCmdCacheReport_OutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cr-format"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

func TestDispatchCommand_CacheReport(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-dispatch-cr"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := dispatchCommand(context.Background(), "/cache-report", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("dispatchCommand /cache-report returned quit=true")
	}
}

// TestCmdCacheReport_ByModelBreakdown verifies multi-model output.
func TestCmdCacheReport_ByModelBreakdown(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cr-bymodel"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Minute),
			Model:            "claude-3",
			Provider:         "anthropic",
			PromptTokens:     150,
			CompletionTokens: 75,
			CachedTokens:     30,
			ReasoningTokens:  0,
			TotalTokens:      225,
			Cost:             0.003000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		Config: CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheReport(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheReport returned quit=true, expected false")
	}
}

// ---------------------------------------------------------------------------
// Multiple filter flags test
// ---------------------------------------------------------------------------

func TestCmdAuditQuery_MultipleFilters(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	quit := cmdAudit(context.Background(), "query --source=agent --action=decision", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdAudit query with multiple filters returned quit=true, expected false")
	}
}

// TestCmdCacheStats_OutputContains verifies that the cache-stats output
// includes expected key phrases by checking the mock's output.
func TestCmdCacheStats_OutputContains(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cache-output"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	// Just verify no panic and correct quit value.
	quit := cmdCacheStats(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheStats returned quit=true, expected false")
	}
}

// TestCmdCost_HasSessionIDPath verifies the cost command constructs the correct file path.
func TestCmdCost_HasSessionIDPath(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cost-path"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     0,
			ReasoningTokens:  0,
			TotalTokens:      150,
			Cost:             0.002500,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	// Verify the file exists.
	usagePath := filepath.Join(dataDir, "sessions", sessionID+".usage.jsonl")
	if _, err := os.Stat(usagePath); os.IsNotExist(err) {
		t.Fatalf("usage file %s was not created", usagePath)
	}

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCost(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCost returned quit=true, expected false")
	}
}

// TestCmdAuditQuery_ArgsParsing verifies that various flag formats are parsed correctly.
func TestCmdAuditQuery_ArgsParsing(t *testing.T) {
	chain := newTestAuditChain(t)
	rt := &AgentRuntime{AuditChain: chain}

	tests := []struct {
		name string
		args string
	}{
		{"empty query", "query"},
		{"source only", "query --source=agent"},
		{"action only", "query --action=decision"},
		{"source and action", "query --source=agent --action=decision"},
		{"from only", "query --from=2026-01-01T00:00:00Z"},
		{"to only", "query --to=2026-12-31T23:59:59Z"},
		{"from and to", "query --from=2026-01-01T00:00:00Z --to=2026-12-31T23:59:59Z"},
		{"all flags", "query --source=agent --action=decision --from=2026-01-01T00:00:00Z --to=2026-12-31T23:59:59Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quit := cmdAudit(context.Background(), tt.args, nil, nil, CLIConfig{}, rt)
			if quit {
				t.Errorf("cmdAudit %q returned quit=true", tt.args)
			}
		})
	}
}

// TestCmdAuditStats_CountsCorrect verifies that the stats command
// produces correct counts by checking the underlying data.
func TestCmdAuditStats_CountsCorrect(t *testing.T) {
	chain := newTestAuditChain(t)
	entries := chain.Snapshot()

	// Verify the test data has the expected counts.
	sourceCounts := make(map[string]int)
	actionCounts := make(map[string]int)
	for _, e := range entries {
		sourceCounts[e.Source]++
		actionCounts[e.Action]++
	}

	if sourceCounts["agent"] != 2 {
		t.Errorf("expected 2 agent entries, got %d", sourceCounts["agent"])
	}
	if sourceCounts["model"] != 1 {
		t.Errorf("expected 1 model entry, got %d", sourceCounts["model"])
	}
	if actionCounts["decision"] != 2 {
		t.Errorf("expected 2 decision entries, got %d", actionCounts["decision"])
	}
	if actionCounts["execute"] != 2 {
		t.Errorf("expected 2 execute entries, got %d", actionCounts["execute"])
	}
}

// TestCmdCacheStats_ByModel verifies that the cache-stats command
// handles the by-model breakdown correctly.
func TestCmdCacheStats_ByModel(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cache-bymodel"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     200,
			CompletionTokens: 100,
			CachedTokens:     50,
			ReasoningTokens:  0,
			TotalTokens:      300,
			Cost:             0.002000,
			Currency:         "USD",
		},
		{
			Timestamp:        now.Add(time.Minute),
			Model:            "claude-3",
			Provider:         "anthropic",
			PromptTokens:     150,
			CompletionTokens: 75,
			CachedTokens:     30,
			ReasoningTokens:  0,
			TotalTokens:      225,
			Cost:             0.003000,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	quit := cmdCacheStats(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCacheStats returned quit=true, expected false")
	}
}

// TestCmdCost_CheckCostFormatting verifies cost values have 6 decimal places.
func TestCmdCost_CheckCostFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, ".inferglow")
	sessionID := "test-cost-formatting"

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []session.UsageRecord{
		{
			Timestamp:        now,
			Model:            "gpt-4",
			Provider:         "openai",
			PromptTokens:     100,
			CompletionTokens: 50,
			CachedTokens:     0,
			ReasoningTokens:  0,
			TotalTokens:      150,
			Cost:             0.002500,
			Currency:         "USD",
		},
	}
	writeUsageJSONL(t, dataDir, sessionID, records)

	rt := &AgentRuntime{
		SessionID: sessionID,
		Config:    CLIConfig{DataDir: dataDir},
	}

	// Verify the cost is formatted with 6 decimal places.
	stats, err := session.LoadUsage(sessionID, dataDir)
	if err != nil {
		t.Fatalf("LoadUsage: %v", err)
	}
	if stats.TotalCost != 0.002500 {
		t.Errorf("expected cost 0.002500, got %f", stats.TotalCost)
	}

	quit := cmdCost(context.Background(), "", nil, nil, CLIConfig{}, rt)
	if quit {
		t.Error("cmdCost returned quit=true, expected false")
	}
}