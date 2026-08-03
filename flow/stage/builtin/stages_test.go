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

package builtin

import (
	"context"
	"sort"
	"testing"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/stage"
)

// mockContext implements flow.Context for testing the GenerateModel path.
// It returns a canned JSON response that exerciseJSONField extraction.
type mockContext struct {
	generateModel func(ctx context.Context, system, userMessage string) (string, error)
}

func (m *mockContext) ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error) {
	return nil, nil
}
func (m *mockContext) GenerateModel(ctx context.Context, system, userMessage string) (string, error) {
	if m.generateModel != nil {
		return m.generateModel(ctx, system, userMessage)
	}
	return `{"category": "bug", "priority": "high", "summary": "test summary"}`, nil
}
func (m *mockContext) SessionHistory() []map[string]any                     { return nil }
func (m *mockContext) AppendSession(role string, content any)               {}
func (m *mockContext) AuditAppend(source, action string, input, output any) {}
func (m *mockContext) SetValue(key string, value any)                       {}
func (m *mockContext) GetValue(key string) (any, bool)                      { return nil, false }
func (m *mockContext) StartSpan(ctx context.Context, kind flow.SpanKind, name string) (context.Context, flow.Span) {
	return ctx, flow.NoopSpan()
}
func (m *mockContext) MaskInput(input string) string    { return input }
func (m *mockContext) CheckOutput(output string) error  { return nil }
func (m *mockContext) RequestPause(reason string) error { return nil }
func (m *mockContext) RunAgent(ctx context.Context, userMessage string, systemPrompt string, opts *flow.AgentRunOptions) (string, error) {
	return "", nil
}
func (m *mockContext) RunAgentParallel(ctx context.Context, agents []flow.AgentSubTask) ([]string, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// RegisterAll
// ---------------------------------------------------------------------------

func TestRegisterAll(t *testing.T) {
	reg := stage.NewRegistry()
	RegisterAll(reg)

	names := reg.List()
	if len(names) != 4 {
		t.Fatalf("expected 4 registered stages, got %d: %v", len(names), names)
	}

	sort.Strings(names)
	expected := []string{"coder", "plan", "reviewer", "triage"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected names[%d] = %q, got %q", i, name, names[i])
		}
	}

	// Verify each name resolves to a Func.
	for _, name := range expected {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("stage %q should be resolvable after RegisterAll", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Triage
// ---------------------------------------------------------------------------

func TestTriage_NilContext(t *testing.T) {
	ctx := context.Background()
	in := stage.Inputs{"issue_title": "test", "issue_body": "body"}
	out, err := Triage(ctx, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["category"] != "unknown" {
		t.Errorf("expected category 'unknown', got %v", out["category"])
	}
	if out["priority"] != "medium" {
		t.Errorf("expected priority 'medium', got %v", out["priority"])
	}
	if out["summary"] != "no Context" {
		t.Errorf("expected summary 'no Context', got %v", out["summary"])
	}
}

func TestTriage_WithContext(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{}
	in := stage.Inputs{"issue_title": "test bug", "issue_body": "something broke"}
	out, err := Triage(ctx, in, fctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["category"] != "bug" {
		t.Errorf("expected category 'bug', got %v", out["category"])
	}
	if out["priority"] != "high" {
		t.Errorf("expected priority 'high', got %v", out["priority"])
	}
	if out["summary"] != "test summary" {
		t.Errorf("expected summary 'test summary', got %v", out["summary"])
	}
}

func TestTriage_CustomSystemPrompt(t *testing.T) {
	ctx := context.Background()
	var capturedSystem string
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			capturedSystem = system
			return `{"category": "feature", "priority": "low"}`, nil
		},
	}
	in := stage.Inputs{
		"issue_title":    "test",
		"issue_body":     "body",
		"_system_prompt": "custom prompt",
	}
	out, err := Triage(ctx, in, fctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSystem != "custom prompt" {
		t.Errorf("expected custom system prompt, got %q", capturedSystem)
	}
	if out["category"] != "feature" {
		t.Errorf("expected category 'feature', got %v", out["category"])
	}
	if out["priority"] != "low" {
		t.Errorf("expected priority 'low', got %v", out["priority"])
	}
}

func TestTriage_GenerateModelError(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return "", assertAnError("llm failure")
		},
	}
	in := stage.Inputs{"issue_title": "test", "issue_body": "body"}
	_, err := Triage(ctx, in, fctx)
	if err == nil {
		t.Fatal("expected error from GenerateModel, got nil")
	}
}

// ---------------------------------------------------------------------------
// Plan
// ---------------------------------------------------------------------------

func TestPlan_NilContext(t *testing.T) {
	ctx := context.Background()
	in := stage.Inputs{"issue_title": "test", "category": "bug", "priority": "high", "summary": "fix it"}
	out, err := Plan(ctx, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["title"] != "no-op plan" {
		t.Errorf("expected title 'no-op plan', got %v", out["title"])
	}
	if out["estimated_complexity"] != "low" {
		t.Errorf("expected complexity 'low', got %v", out["estimated_complexity"])
	}
}

func TestPlan_WithContext(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return `{"title": "my plan", "steps": "[step1]", "risks": "[]", "estimated_complexity": "medium"}`, nil
		},
	}
	in := stage.Inputs{"issue_title": "test", "category": "bug", "priority": "high", "summary": "fix it"}
	out, err := Plan(ctx, in, fctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["title"] != "my plan" {
		t.Errorf("expected title 'my plan', got %v", out["title"])
	}
	if out["estimated_complexity"] != "medium" {
		t.Errorf("expected complexity 'medium', got %v", out["estimated_complexity"])
	}
}

func TestPlan_GenerateModelError(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return "", assertAnError("plan failure")
		},
	}
	in := stage.Inputs{"issue_title": "test", "category": "bug", "priority": "high", "summary": "fix it"}
	_, err := Plan(ctx, in, fctx)
	if err == nil {
		t.Fatal("expected error from GenerateModel, got nil")
	}
}

// ---------------------------------------------------------------------------
// Coder
// ---------------------------------------------------------------------------

func TestCoder_NilContext(t *testing.T) {
	ctx := context.Background()
	in := stage.Inputs{"title": "plan", "steps": "[]", "issue_title": "test"}
	out, err := Coder(ctx, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["summary"] != "no-op" {
		t.Errorf("expected summary 'no-op', got %v", out["summary"])
	}
}

func TestCoder_WithContext(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return `{"files": "[{\"path\":\"main.go\"}]", "summary": "added main.go", "tests": "[]"}`, nil
		},
	}
	in := stage.Inputs{"title": "plan", "steps": "[]", "issue_title": "test"}
	out, err := Coder(ctx, in, fctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["summary"] != "added main.go" {
		t.Errorf("expected summary 'added main.go', got %v", out["summary"])
	}
}

func TestCoder_GenerateModelError(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return "", assertAnError("coder failure")
		},
	}
	in := stage.Inputs{"title": "plan", "steps": "[]", "issue_title": "test"}
	_, err := Coder(ctx, in, fctx)
	if err == nil {
		t.Fatal("expected error from GenerateModel, got nil")
	}
}

// ---------------------------------------------------------------------------
// Reviewer
// ---------------------------------------------------------------------------

func TestReviewer_NilContext(t *testing.T) {
	ctx := context.Background()
	in := stage.Inputs{"files": "[]", "summary": "code", "issue_title": "test"}
	out, err := Reviewer(ctx, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["approved"] != "true" {
		t.Errorf("expected approved 'true', got %v", out["approved"])
	}
	if out["summary"] != "no-op review" {
		t.Errorf("expected summary 'no-op review', got %v", out["summary"])
	}
}

func TestReviewer_WithContext(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return `{"approved": "false", "comments": "[]", "summary": "needs work", "suggestions": "[\"add tests\"]"}`, nil
		},
	}
	in := stage.Inputs{"files": "[]", "summary": "code", "issue_title": "test"}
	out, err := Reviewer(ctx, in, fctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["approved"] != "false" {
		t.Errorf("expected approved 'false', got %v", out["approved"])
	}
	if out["summary"] != "needs work" {
		t.Errorf("expected summary 'needs work', got %v", out["summary"])
	}
}

func TestReviewer_GenerateModelError(t *testing.T) {
	ctx := context.Background()
	fctx := &mockContext{
		generateModel: func(ctx context.Context, system, userMessage string) (string, error) {
			return "", assertAnError("reviewer failure")
		},
	}
	in := stage.Inputs{"files": "[]", "summary": "code", "issue_title": "test"}
	_, err := Reviewer(ctx, in, fctx)
	if err == nil {
		t.Fatal("expected error from GenerateModel, got nil")
	}
}

// ---------------------------------------------------------------------------
// System prompt constants
// ---------------------------------------------------------------------------

func TestSystemPromptConstants_NonEmpty(t *testing.T) {
	prompts := map[string]string{
		"TriageSystemPrompt":   TriageSystemPrompt,
		"PlanSystemPrompt":     PlanSystemPrompt,
		"CoderSystemPrompt":    CoderSystemPrompt,
		"ReviewerSystemPrompt": ReviewerSystemPrompt,
	}
	for name, val := range prompts {
		if val == "" {
			t.Errorf("%s should not be empty", name)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertAnError returns a simple error for testing error propagation.
type assertAnError string

func (e assertAnError) Error() string { return string(e) }
