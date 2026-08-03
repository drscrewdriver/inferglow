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

package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/flow"
	"github.com/inferglow/skill"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeSkillMD creates a temporary .md skill file and returns the directory.
func writeSkillMD(t *testing.T, name, runas, body string) string {
	t.Helper()
	dir := t.TempDir()
	content := "---\n" +
		"name: " + name + "\n" +
		"description: A test skill\n" +
		"runas: " + runas + "\n" +
		"---\n" +
		body
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newInlineStore creates a skill.Store with a single inline skill.
func newInlineStore(t *testing.T) *skill.Store {
	t.Helper()
	dir := writeSkillMD(t, "test-inline", "inline", "# test-inline\nDo something inline")
	return skill.NewStore(dir, "")
}

// emptyStore creates a skill.Store pointing to an empty directory.
func emptyStore(t *testing.T) *skill.Store {
	t.Helper()
	return skill.NewStore(t.TempDir(), "")
}

// flowMock is a minimal flow.Context mock for subagent tests.
type flowMock struct {
	agentResult string
	agentErr    error
}

func (m *flowMock) ExecuteAction(_ context.Context, _ string, _ map[string]any) (any, error) {
	return nil, nil
}
func (m *flowMock) GenerateModel(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *flowMock) SessionHistory() []map[string]any                    { return nil }
func (m *flowMock) AppendSession(_ string, _ any)                       {}
func (m *flowMock) AuditAppend(_, _ string, _, _ any)                   {}
func (m *flowMock) SetValue(_ string, _ any)                            {}
func (m *flowMock) GetValue(_ string) (any, bool)                       { return nil, false }
func (m *flowMock) StartSpan(_ context.Context, _ flow.SpanKind, _ string) (context.Context, flow.Span) {
	return context.Background(), flow.NoopSpan()
}
func (m *flowMock) MaskInput(s string) string              { return s }
func (m *flowMock) CheckOutput(_ string) error             { return nil }
func (m *flowMock) RequestPause(_ string) error            { return nil }
func (m *flowMock) RunAgent(_ context.Context, _, _ string, _ *flow.AgentRunOptions) (string, error) {
	return m.agentResult, m.agentErr
}
func (m *flowMock) RunAgentParallel(_ context.Context, _ []flow.AgentSubTask) ([]string, error) {
	return nil, nil
}

// flowContext returns a context.Context with a flowMock injected.
func flowContext(result string) context.Context {
	return flow.WithFlowContext(context.Background(), &flowMock{agentResult: result})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunSkillAction_NameRequired(t *testing.T) {
	store := emptyStore(t)
	a := NewRunSkillAction(RunSkillConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when name is empty")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "run_skill: name is required" {
		t.Errorf("Error = %q, want %q", res.Error, "run_skill: name is required")
	}
}

func TestRunSkillAction_SkillNotFound(t *testing.T) {
	store := emptyStore(t)
	a := NewRunSkillAction(RunSkillConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when skill not found")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "run_skill: skill 'nonexistent' not found. Available skills: list from skill store." {
		t.Errorf("Error = %q, want %q", res.Error, "run_skill: skill 'nonexistent' not found. Available skills: list from skill store.")
	}
}

func TestRunSkillAction_InlineMode(t *testing.T) {
	store := newInlineStore(t)
	a := NewRunSkillAction(RunSkillConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "test-inline",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want ok", res.Status)
	}
	body, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string: %T", res.Result)
	}
	if body != "# test-inline\nDo something inline" {
		t.Errorf("Result body = %q, want %q", body, "# test-inline\nDo something inline")
	}
}

func TestRunSkillAction_InlineModeWithArguments(t *testing.T) {
	store := newInlineStore(t)
	a := NewRunSkillAction(RunSkillConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name":      "test-inline",
		"arguments": "fix the bug",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	body, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string: %T", res.Result)
	}
	expected := "# test-inline\nDo something inline\n\n## Task Arguments\nfix the bug"
	if body != expected {
		t.Errorf("Result body = %q, want %q", body, expected)
	}
}

func TestRunSkillAction_InlineModeNoArguments(t *testing.T) {
	store := newInlineStore(t)
	a := NewRunSkillAction(RunSkillConfig{Store: store})
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "test-inline",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	body, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string: %T", res.Result)
	}
	// Should not contain "## Task Arguments" when arguments is empty
	if body != "# test-inline\nDo something inline" {
		t.Errorf("Result body = %q, want %q", body, "# test-inline\nDo something inline")
	}
}

func TestRunSkillAction_NewActionDefaults(t *testing.T) {
	a := NewRunSkillAction(RunSkillConfig{})
	if a.Name != "run_skill" {
		t.Errorf("Name = %q, want run_skill", a.Name)
	}
	if a.Description == "" {
		t.Error("Description should not be empty")
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
	if len(a.Tags) == 0 {
		t.Error("Tags should not be empty")
	}
	// Verify schema requires name
	schema, ok := a.Schema["required"].([]string)
	if !ok {
		t.Fatal("Schema missing required field")
	}
	found := false
	for _, r := range schema {
		if r == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Schema should require 'name'")
	}
}

func TestRunSkillAction_SubagentModeRequiresFlowContext(t *testing.T) {
	// Create a skill with subagent runas in a temp dir
	dir := writeSkillMD(t, "test-subagent", "subagent", "# test-subagent\nRun as subagent")
	store := skill.NewStore(dir, "")
	a := NewRunSkillAction(RunSkillConfig{Store: store})

	// Use regular context.Background() — no flow context injected
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "test-subagent",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when flow context is missing")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "run_skill: subagent mode requires flow context (not running inside a flow)" {
		t.Errorf("Error = %q, want %q", res.Error, "run_skill: subagent mode requires flow context (not running inside a flow)")
	}
}

func TestRunSkillAction_SubagentModeWithFlowContext(t *testing.T) {
	// Create a skill with subagent runas
	dir := writeSkillMD(t, "test-subagent", "subagent", "# test-subagent\nRun as subagent")
	store := skill.NewStore(dir, "")
	a := NewRunSkillAction(RunSkillConfig{Store: store})

	ctx := flowContext("subagent result")
	res, err := a.Executor.Execute(ctx, map[string]any{
		"name": "test-subagent",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want ok", res.Status)
	}
	if res.Result != "subagent result" {
		t.Errorf("Result = %q, want %q", res.Result, "subagent result")
	}
}

func TestRunSkillAction_SubagentModeWithArguments(t *testing.T) {
	dir := writeSkillMD(t, "test-subagent", "subagent", "# test-subagent\nRun as subagent")
	store := skill.NewStore(dir, "")
	a := NewRunSkillAction(RunSkillConfig{Store: store})

	ctx := flowContext("subagent with args")
	res, err := a.Executor.Execute(ctx, map[string]any{
		"name":      "test-subagent",
		"arguments": "custom task",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Result != "subagent with args" {
		t.Errorf("Result = %q, want %q", res.Result, "subagent with args")
	}
}

func TestRunSkillAction_DefaultMaxRounds(t *testing.T) {
	cfg := RunSkillConfig{MaxRounds: 0}
	a := NewRunSkillAction(cfg)
	exec := a.Executor.(*runSkillExecutor)
	if exec.cfg.MaxRounds != 15 {
		t.Errorf("default MaxRounds = %d, want 15", exec.cfg.MaxRounds)
	}
}

func TestRunSkillAction_CustomMaxRounds(t *testing.T) {
	cfg := RunSkillConfig{MaxRounds: 5}
	a := NewRunSkillAction(cfg)
	exec := a.Executor.(*runSkillExecutor)
	if exec.cfg.MaxRounds != 5 {
		t.Errorf("MaxRounds = %d, want 5", exec.cfg.MaxRounds)
	}
}

// TestRunSkillAction_ActionRegistration verifies the action can be registered.
func TestRunSkillAction_ActionRegistration(t *testing.T) {
	store := newInlineStore(t)
	r := action.NewRegistry()
	if err := r.Register(NewRunSkillAction(RunSkillConfig{Store: store})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	got, err := r.Get("run_skill")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Name != "run_skill" {
		t.Errorf("Name = %q, want run_skill", got.Name)
	}
}