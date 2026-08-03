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
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/flow"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// subAgentFlowMock is a minimal flow.Context mock for sub_agent tests.
type subAgentFlowMock struct {
	agentResult string
	agentErr    error
}

func (m *subAgentFlowMock) ExecuteAction(_ context.Context, _ string, _ map[string]any) (any, error) {
	return nil, nil
}
func (m *subAgentFlowMock) GenerateModel(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *subAgentFlowMock) SessionHistory() []map[string]any { return nil }
func (m *subAgentFlowMock) AppendSession(_ string, _ any)    {}
func (m *subAgentFlowMock) AuditAppend(_, _ string, _, _ any) {}
func (m *subAgentFlowMock) SetValue(_ string, _ any)          {}
func (m *subAgentFlowMock) GetValue(_ string) (any, bool)     { return nil, false }
func (m *subAgentFlowMock) StartSpan(_ context.Context, _ flow.SpanKind, _ string) (context.Context, flow.Span) {
	return context.Background(), flow.NoopSpan()
}
func (m *subAgentFlowMock) MaskInput(s string) string  { return s }
func (m *subAgentFlowMock) CheckOutput(_ string) error { return nil }
func (m *subAgentFlowMock) RequestPause(_ string) error {
	return nil
}
func (m *subAgentFlowMock) RunAgent(_ context.Context, _, _ string, _ *flow.AgentRunOptions) (string, error) {
	return m.agentResult, m.agentErr
}
func (m *subAgentFlowMock) RunAgentParallel(_ context.Context, _ []flow.AgentSubTask) ([]string, error) {
	return nil, nil
}

// subAgentFlowContext returns a context with a flowMock injected.
func subAgentFlowContext(result string) context.Context {
	return flow.WithFlowContext(context.Background(), &subAgentFlowMock{agentResult: result})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSubAgentAction_TaskRequired(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	res, err := a.Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false when task is empty")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want error", res.Status)
	}
	if res.Error != "spawn_agent: task is required" {
		t.Errorf("Error = %q, want %q", res.Error, "spawn_agent: task is required")
	}
}

func TestSubAgentAction_FlowContextRequired(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	// Use context.Background() — no flow context injected
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"task": "do something",
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
	if res.Error != "spawn_agent: flow context not available (not running inside a flow)" {
		t.Errorf("Error = %q, want %q", res.Error, "spawn_agent: flow context not available (not running inside a flow)")
	}
}

func TestSubAgentAction_WithFlowContext(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	ctx := subAgentFlowContext("agent completed successfully")
	res, err := a.Executor.Execute(ctx, map[string]any{
		"task": "analyze the codebase",
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
	if res.Result != "agent completed successfully" {
		t.Errorf("Result = %q, want %q", res.Result, "agent completed successfully")
	}
}

func TestSubAgentAction_WithSystemPrompt(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	ctx := subAgentFlowContext("result with system prompt")
	res, err := a.Executor.Execute(ctx, map[string]any{
		"task":          "fix the bug",
		"system_prompt": "You are a Go expert",
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Result != "result with system prompt" {
		t.Errorf("Result = %q, want %q", res.Result, "result with system prompt")
	}
}

func TestSubAgentAction_CustomMaxRounds(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	ctx := subAgentFlowContext("custom rounds result")
	res, err := a.Executor.Execute(ctx, map[string]any{
		"task":       "do work",
		"max_rounds": float64(5),
	})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Result != "custom rounds result" {
		t.Errorf("Result = %q, want %q", res.Result, "custom rounds result")
	}
}

func TestSubAgentAction_NewActionDefaults(t *testing.T) {
	a := NewSubAgentAction(SubAgentConfig{})
	if a.Name != "spawn_agent" {
		t.Errorf("Name = %q, want spawn_agent", a.Name)
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
	// Verify schema requires task
	schema, ok := a.Schema["required"].([]string)
	if !ok {
		t.Fatal("Schema missing required field")
	}
	found := false
	for _, r := range schema {
		if r == "task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Schema should require 'task'")
	}
}

func TestSubAgentAction_DefaultConfigValues(t *testing.T) {
	cfg := SubAgentConfig{}
	a := NewSubAgentAction(cfg)
	exec := a.Executor.(*subAgentExecutor)
	if exec.cfg.MaxDepth != 3 {
		t.Errorf("default MaxDepth = %d, want 3", exec.cfg.MaxDepth)
	}
	if exec.cfg.MaxRounds != 15 {
		t.Errorf("default MaxRounds = %d, want 15", exec.cfg.MaxRounds)
	}
}

func TestSubAgentAction_CustomConfigValues(t *testing.T) {
	cfg := SubAgentConfig{MaxDepth: 5, MaxRounds: 10}
	a := NewSubAgentAction(cfg)
	exec := a.Executor.(*subAgentExecutor)
	if exec.cfg.MaxDepth != 5 {
		t.Errorf("MaxDepth = %d, want 5", exec.cfg.MaxDepth)
	}
	if exec.cfg.MaxRounds != 10 {
		t.Errorf("MaxRounds = %d, want 10", exec.cfg.MaxRounds)
	}
}

// TestSubAgentAction_ActionRegistration verifies the action can be registered.
func TestSubAgentAction_ActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewSubAgentAction(SubAgentConfig{})); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	got, err := r.Get("spawn_agent")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Name != "spawn_agent" {
		t.Errorf("Name = %q, want spawn_agent", got.Name)
	}
}