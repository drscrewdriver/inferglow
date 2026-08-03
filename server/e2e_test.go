// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"testing"
	"time"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/flowdef"
	"github.com/inferglow/flow/stage"
	"github.com/inferglow/flow/stage/builtin"
	"gopkg.in/yaml.v3"
)

// mockContext is a minimal Context for testing.
type mockContext struct {
	flow.Context
}

func (m *mockContext) GenerateModel(ctx context.Context, system, userMessage string) (string, error) {
	// Return a mock JSON response based on the system prompt.
	if contains(system, "triage") {
		return `{"category": "bug", "priority": "high", "summary": "Test issue"}`, nil
	}
	if contains(system, "plan") {
		return `{"title": "Fix bug", "steps": ["step1"], "risks": [], "estimated_complexity": "low"}`, nil
	}
	if contains(system, "coder") {
		return `{"files": [{"path": "fix.go", "content": "package main"}], "summary": "Fixed"}`, nil
	}
	if contains(system, "review") {
		return `{"approved": true, "comments": [], "summary": "LGTM"}`, nil
	}
	return `{"result": "ok"}`, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestEndToEnd_BugFixWorkflow tests the complete chain:
// YAML → FlowDef → Flow → Execute with Context injection.
func TestEndToEnd_BugFixWorkflow(t *testing.T) {
	// 1. Create stage registry and register builtins.
	stages := stage.NewRegistry()
	builtin.RegisterAll(stages)

	// 2. Create FlowStore.
	flowStore := NewFlowStore(stages)

	// 3. Parse YAML workflow.
	yamlContent := `
api_version: flowdef/v1
kind: Flow
metadata:
  name: bug-fix
  version: "1.0.0"
spec:
  steps:
    - name: triage
      operator: stage
      stage: triage
      inputs:
        issue_title: "{{.issue_title}}"
        issue_body: "{{.issue_body}}"
    - name: plan
      operator: stage
      stage: plan
      depends_on: [triage]
    - name: coder
      operator: stage
      stage: coder
      depends_on: [plan]
    - name: reviewer
      operator: stage
      stage: reviewer
      depends_on: [coder]
`
	def := &flowdef.FlowDef{}
	if err := yaml.Unmarshal([]byte(yamlContent), def); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}

	// 4. Register flow.
	if err := flowStore.Register(def); err != nil {
		t.Fatalf("register flow: %v", err)
	}

	// 5. Create RunManager with mock Context factory.
	runMgr := NewRunManager(flowStore)
	runMgr.SetContextFactory(func(ctx context.Context) flow.Context {
		return &mockContext{}
	})

	// 6. Start run.
	inputs := map[string]any{
		"issue_title": "Test bug",
		"issue_body":  "Something is broken",
	}
	handle, err := runMgr.Start("bug-fix", inputs, "test-user")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	// 7. Wait for completion (with timeout).
	done := make(chan struct{})
	go func() {
		for {
			h, _ := runMgr.Status(handle.ID)
			if h.Status == RunStatusDone || h.Status == RunStatusFailed {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for run to complete")
	}

	// 8. Verify result.
	h, _ := runMgr.Status(handle.ID)
	if h.Status != RunStatusDone {
		t.Errorf("expected status done, got %s (error: %s)", h.Status, h.Error)
	}
	if h.Output == nil {
		t.Error("expected output, got nil")
	}
}
