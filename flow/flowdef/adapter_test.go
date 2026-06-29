package flowdef

import (
	"context"
	"strings"
	"testing"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/stage"
)

// TestToFlow_LinearStages builds a 3-step linear flow with stage operators,
// executes it, and verifies each step ran and its output is accumulated.
func TestToFlow_LinearStages(t *testing.T) {
	stages := stage.NewRegistry()
	stages.Register("triage", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"category": "bug", "priority": "high"}, nil
	})
	stages.Register("coder", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"files": []string{"a.go", "b.go"}, "coded": true}, nil
	})
	stages.Register("committer", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"sha": "abc123", "committed": true}, nil
	})

	def := &FlowDef{
		APIVersion: "flowdef/v1",
		Kind:       "Flow",
		Metadata:   Metadata{Name: "linear-test"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "triage", Operator: "stage", Stage: "triage"},
				{Name: "coder", Operator: "stage", Stage: "coder", DependsOn: []string{"triage"}},
				{Name: "committer", Operator: "stage", Stage: "committer", DependsOn: []string{"coder"}},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{"repo_url": "https://github.com/test/repo"})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("flow status = %v, want completed. Errors: %v", exec.State.Status, exec.State.Errors)
	}

	result, ok := exec.State.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is %T, want map[string]any", exec.State.Result)
	}

	// All 3 step logs should exist.
	for _, name := range []string{"triage", "coder", "committer"} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("missing step log for %q", name)
		}
	}

	// Verify accumulated outputs.
	triage, ok := result["triage"].(map[string]any)
	if !ok {
		t.Fatalf("result[triage] = %T, want map[string]any", result["triage"])
	}
	if triage["category"] != "bug" {
		t.Errorf("triage.category = %v, want bug", triage["category"])
	}

	coder, ok := result["coder"].(map[string]any)
	if !ok {
		t.Fatalf("result[coder] = %T, want map[string]any", result["coder"])
	}
	if coder["coded"] != true {
		t.Errorf("coder.coded = %v, want true", coder["coded"])
	}

	committer, ok := result["committer"].(map[string]any)
	if !ok {
		t.Fatalf("result[committer] = %T, want map[string]any", result["committer"])
	}
	if committer["sha"] != "abc123" {
		t.Errorf("committer.sha = %v, want abc123", committer["sha"])
	}

	// Original input should still be accessible.
	if result["repo_url"] != "https://github.com/test/repo" {
		t.Errorf("repo_url = %v, want preserved input", result["repo_url"])
	}
}

// TestToFlow_WhenExpression verifies that a step with a `when` expression
// runs when the condition is true and is skipped (input passed through) when false.
func TestToFlow_WhenExpression(t *testing.T) {
	stages := stage.NewRegistry()
	// step1 echoes the should_run flag from the flow input.
	stages.Register("step1", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"should_run": in["should_run"]}, nil
	})
	stages.Register("step2", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"step2_ran": true}, nil
	})
	stages.Register("step3", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"step3_ran": true}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "when-test"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "step1", Operator: "stage", Stage: "step1"},
				{Name: "step2", Operator: "stage", Stage: "step2", DependsOn: []string{"step1"}, When: "{{.step1.should_run}}"},
				{Name: "step3", Operator: "stage", Stage: "step3", DependsOn: []string{"step2"}},
			},
		},
	}

	adapter := NewAdapter(stages)

	// Sub-case 1: should_run=true → step2 runs.
	t.Run("runs_when_true", func(t *testing.T) {
		f, err := adapter.ToFlow(def)
		if err != nil {
			t.Fatalf("ToFlow error: %v", err)
		}
		exec := f.Execute(context.Background(), map[string]any{"should_run": true})
		if exec.State.Status != flow.StatusCompleted {
			t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
		}
		result := exec.State.Result.(map[string]any)
		if _, ok := result["step2"]; !ok {
			t.Errorf("expected step2 output when should_run=true, result keys: %v", keys(result))
		}
		step2 := result["step2"].(map[string]any)
		if step2["step2_ran"] != true {
			t.Errorf("step2.step2_ran = %v, want true", step2["step2_ran"])
		}
	})

	// Sub-case 2: should_run=false → step2 skipped (no step2 output).
	t.Run("skipped_when_false", func(t *testing.T) {
		f, err := adapter.ToFlow(def)
		if err != nil {
			t.Fatalf("ToFlow error: %v", err)
		}
		exec := f.Execute(context.Background(), map[string]any{"should_run": false})
		if exec.State.Status != flow.StatusCompleted {
			t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
		}
		result := exec.State.Result.(map[string]any)
		if _, ok := result["step2"]; ok {
			t.Errorf("expected step2 to be skipped (no output), but got: %v", result["step2"])
		}
		// step3 should still run.
		if _, ok := result["step3"]; !ok {
			t.Errorf("expected step3 output even when step2 skipped, result keys: %v", keys(result))
		}
	})
}

// TestToFlow_StageNotFound verifies that referencing an unregistered stage
// produces an error from ToFlow.
func TestToFlow_StageNotFound(t *testing.T) {
	stages := stage.NewRegistry()
	stages.Register("triage", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return nil, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "missing-stage"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "s1", Operator: "stage", Stage: "triage"},
				{Name: "s2", Operator: "stage", Stage: "nonexistent", DependsOn: []string{"s1"}},
			},
		},
	}

	adapter := NewAdapter(stages)
	_, err := adapter.ToFlow(def)
	if err == nil {
		t.Fatal("expected error for unregistered stage, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing stage name, got: %v", err)
	}
}

// TestToFlow_TemplateInputs verifies that step inputs with {{.field}}
// templates are rendered against the accumulated data (flow inputs + prior
// step outputs) before being passed to the stage.
func TestToFlow_TemplateInputs(t *testing.T) {
	stages := stage.NewRegistry()
	var capturedInputs stage.Inputs
	stages.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		capturedInputs = in
		return stage.Outputs{"echoed": in["repo_url"]}, nil
	})
	stages.Register("consumer", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"consumed": in["url"]}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "template-test"},
		Spec: Spec{
			Inputs: []InputDef{{Name: "repo_url", Type: "string", Required: true}},
			Steps: []StepDef{
				{
					Name:     "echo",
					Operator: "stage",
					Stage:    "echo",
					Inputs:   map[string]any{"repo_url": "{{.repo_url}}"},
				},
				{
					Name:     "consumer",
					Operator: "stage",
					Stage:    "consumer",
					DependsOn: []string{"echo"},
					// Template referencing a prior step's output field.
					Inputs: map[string]any{"url": "{{.echo.echoed}}"},
				},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{"repo_url": "https://github.com/test/repo"})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}

	// The first step should have received the rendered repo_url.
	if capturedInputs["repo_url"] != "https://github.com/test/repo" {
		t.Errorf("echo stage received repo_url = %v, want rendered URL", capturedInputs["repo_url"])
	}

	// The consumer step should have received the echoed URL from step1's output.
	result := exec.State.Result.(map[string]any)
	consumer := result["consumer"].(map[string]any)
	if consumer["consumed"] != "https://github.com/test/repo" {
		t.Errorf("consumer.consumed = %v, want rendered URL from echo step", consumer["consumed"])
	}
}

// TestToFlow_PassThroughOperator verifies that non-stage operators
// (parallel_fanout, match_case, etc.) are accepted by ToFlow and become
// pass-through steps that do not crash the flow.
func TestToFlow_PassThroughOperator(t *testing.T) {
	stages := stage.NewRegistry()
	stages.Register("real", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"done": true}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "passthrough-test"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "s1", Operator: "stage", Stage: "real"},
				{Name: "s2", Operator: "parallel_fanout", Stage: "reviewer", DependsOn: []string{"s1"}},
				{Name: "s3", Operator: "match_case", DependsOn: []string{"s2"}},
				{Name: "s4", Operator: "stage", Stage: "real", DependsOn: []string{"s3"}},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}
	result := exec.State.Result.(map[string]any)
	// s4 (real stage) should still produce output despite pass-through intermediaries.
	if _, ok := result["s4"]; !ok {
		t.Errorf("expected s4 output after pass-through steps, result keys: %v", keys(result))
	}
}

// TestToFlow_SingleStep verifies a flow with one step and no depends_on.
func TestToFlow_SingleStep(t *testing.T) {
	stages := stage.NewRegistry()
	stages.Register("only", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		return stage.Outputs{"v": 42}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "single"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "s1", Operator: "stage", Stage: "only"},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}
	exec := f.Execute(context.Background(), map[string]any{})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}
	result := exec.State.Result.(map[string]any)
	if v, ok := result["s1"].(map[string]any); !ok || v["v"] != 42 {
		t.Errorf("s1 output = %v, want v=42", result["s1"])
	}
}

// TestNewAdapter_NilStages ensures NewAdapter does not panic and ToFlow
// returns an error when stages is nil and a stage operator is used.
func TestNewAdapter_NilStages(t *testing.T) {
	adapter := NewAdapter(nil)
	def := &FlowDef{
		Metadata: Metadata{Name: "nil-stages"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "s1", Operator: "stage", Stage: "x"},
			},
		},
	}
	_, err := adapter.ToFlow(def)
	if err == nil {
		t.Fatal("expected error when stages registry is nil, got nil")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestToFlow_SystemPrompt verifies that a step with system_prompt in the YAML
// injects _system_prompt into the stage inputs.
func TestToFlow_SystemPrompt(t *testing.T) {
	stages := stage.NewRegistry()
	var capturedInputs stage.Inputs
	stages.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		capturedInputs = in
		return stage.Outputs{"echoed": "ok"}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "system-prompt-test"},
		Spec: Spec{
			Steps: []StepDef{
				{
					Name:         "s1",
					Operator:     "stage",
					Stage:        "echo",
					SystemPrompt: "You are a helpful assistant.",
				},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}

	// The stage should have received _system_prompt in its inputs.
	if sp, ok := capturedInputs["_system_prompt"].(string); !ok || sp != "You are a helpful assistant." {
		t.Errorf("_system_prompt = %v, want 'You are a helpful assistant.'", capturedInputs["_system_prompt"])
	}
}

// TestToFlow_SystemPrompt_Template verifies that system_prompt supports
// template variables like {{.title}}.
func TestToFlow_SystemPrompt_Template(t *testing.T) {
	stages := stage.NewRegistry()
	var capturedInputs stage.Inputs
	stages.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		capturedInputs = in
		return stage.Outputs{"echoed": "ok"}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "system-prompt-template-test"},
		Spec: Spec{
			Inputs: []InputDef{{Name: "title", Type: "string", Required: true}},
			Steps: []StepDef{
				{
					Name:         "s1",
					Operator:     "stage",
					Stage:        "echo",
					SystemPrompt: "You are working on: {{.title}}",
				},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{"title": "Bug Fix Task"})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}

	// The stage should have received the rendered system_prompt.
	if sp, ok := capturedInputs["_system_prompt"].(string); !ok || sp != "You are working on: Bug Fix Task" {
		t.Errorf("_system_prompt = %v, want 'You are working on: Bug Fix Task'", capturedInputs["_system_prompt"])
	}
}

// TestToFlow_SystemPrompt_Empty verifies that when system_prompt is empty,
// _system_prompt is not injected into stage inputs.
func TestToFlow_SystemPrompt_Empty(t *testing.T) {
	stages := stage.NewRegistry()
	var capturedInputs stage.Inputs
	stages.Register("echo", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		capturedInputs = in
		return stage.Outputs{"echoed": "ok"}, nil
	})

	def := &FlowDef{
		Metadata: Metadata{Name: "no-system-prompt-test"},
		Spec: Spec{
			Steps: []StepDef{
				{
					Name:     "s1",
					Operator: "stage",
					Stage:    "echo",
					// SystemPrompt is empty (default).
				},
			},
		},
	}

	adapter := NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		t.Fatalf("ToFlow error: %v", err)
	}

	exec := f.Execute(context.Background(), map[string]any{})
	if exec.State.Status != flow.StatusCompleted {
		t.Fatalf("status = %v, errors: %v", exec.State.Status, exec.State.Errors)
	}

	// The stage should NOT have received _system_prompt.
	if _, ok := capturedInputs["_system_prompt"]; ok {
		t.Error("_system_prompt should not be present when SystemPrompt is empty")
	}
}
