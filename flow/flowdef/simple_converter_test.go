package flowdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strPtr returns a pointer to s. Helper for building SimpleStage literals.
func strPtr(s string) *string { return &s }

// TestConvertSimpleToFlowDef_LinearChain verifies the conversion of a linear
// linked-list workflow (like bug_fix_workflow.yaml) into a FlowDef with
// correct depends_on relationships.
func TestConvertSimpleToFlowDef_LinearChain(t *testing.T) {
	sw := &SimpleWorkflow{
		Name:        "bug-fix-workflow",
		Description: "Bug fix pipeline",
		StartStage:  "triage",
		EndStage:    "review",
		Stages: []SimpleStage{
			{Name: "triage", Label: "问题理解", Next: strPtr("branch")},
			{Name: "branch", Label: "切分支", Next: strPtr("locate"), RollbackTo: strPtr("triage")},
			{Name: "locate", Label: "根因定位", Next: strPtr("generate"), RollbackTo: strPtr("branch")},
			{Name: "generate", Label: "生成修复代码", Next: strPtr("review"), RollbackTo: strPtr("locate")},
			{Name: "review", Label: "代码审查", RollbackTo: strPtr("generate")},
		},
	}

	def, err := ConvertSimpleToFlowDef(sw)
	if err != nil {
		t.Fatalf("ConvertSimpleToFlowDef: %v", err)
	}

	// Metadata checks.
	if def.Metadata.Name != "bug-fix-workflow" {
		t.Errorf("metadata.name = %q, want %q", def.Metadata.Name, "bug-fix-workflow")
	}
	if def.APIVersion != "flowdef/v1" {
		t.Errorf("api_version = %q, want %q", def.APIVersion, "flowdef/v1")
	}
	if def.Kind != "Flow" {
		t.Errorf("kind = %q, want %q", def.Kind, "Flow")
	}

	// Step count.
	if len(def.Spec.Steps) != 5 {
		t.Fatalf("got %d steps, want 5", len(def.Spec.Steps))
	}

	// Verify depends_on relationships.
	// triage: no predecessor → depends_on = []
	// branch: triage → branch, so branch.depends_on = [triage]
	// locate: branch → locate, so locate.depends_on = [branch]
	// generate: locate → generate, so generate.depends_on = [locate]
	// review: generate → review, so review.depends_on = [generate]
	expected := map[string][]string{
		"triage":   nil,
		"branch":   {"triage"},
		"locate":   {"branch"},
		"generate": {"locate"},
		"review":   {"generate"},
	}
	for _, step := range def.Spec.Steps {
		want := expected[step.Name]
		if len(step.DependsOn) != len(want) {
			t.Errorf("step %q depends_on = %v, want %v", step.Name, step.DependsOn, want)
			continue
		}
		for i, dep := range want {
			if step.DependsOn[i] != dep {
				t.Errorf("step %q depends_on[%d] = %q, want %q", step.Name, i, step.DependsOn[i], dep)
			}
		}
	}

	// Verify operator and stage are set.
	for _, step := range def.Spec.Steps {
		if step.Operator != "stage" {
			t.Errorf("step %q operator = %q, want %q", step.Name, step.Operator, "stage")
		}
		if step.Stage != step.Name {
			t.Errorf("step %q stage = %q, want %q", step.Name, step.Stage, step.Name)
		}
	}

	// Verify rollback_to is stored in inputs.
	for _, step := range def.Spec.Steps {
		if step.Name == "triage" {
			// triage has no rollback_to.
			if _, ok := step.Inputs["_rollback_to"]; ok {
				t.Errorf("step triage should not have _rollback_to")
			}
			continue
		}
		if rb, ok := step.Inputs["_rollback_to"]; ok {
			_ = rb // present, good
		} else {
			t.Errorf("step %q missing _rollback_to in inputs", step.Name)
		}
	}

	// Verify label/description stored in schema.
	branchStep := def.Spec.Steps[1]
	if branchStep.Schema["label"] != "切分支" {
		t.Errorf("branch schema label = %v, want 切分支", branchStep.Schema["label"])
	}

	// Validate the converted FlowDef.
	if err := Validate(def); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestConvertSimpleToFlowDef_EmptyStages verifies that an empty stages list
// is rejected.
func TestConvertSimpleToFlowDef_EmptyStages(t *testing.T) {
	sw := &SimpleWorkflow{Name: "empty", Stages: []SimpleStage{}}
	_, err := ConvertSimpleToFlowDef(sw)
	if err == nil {
		t.Fatal("expected error for empty stages")
	}
	if !strings.Contains(err.Error(), "no stages") {
		t.Errorf("error = %v, want mention of 'no stages'", err)
	}
}

// TestConvertSimpleToFlowDef_Cycle verifies that a cycle in next pointers
// is detected.
func TestConvertSimpleToFlowDef_Cycle(t *testing.T) {
	sw := &SimpleWorkflow{
		Name: "cyclic",
		Stages: []SimpleStage{
			{Name: "a", Next: strPtr("b")},
			{Name: "b", Next: strPtr("c")},
			{Name: "c", Next: strPtr("a")}, // cycle!
		},
	}
	_, err := ConvertSimpleToFlowDef(sw)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, want mention of 'cycle'", err)
	}
}

// TestConvertSimpleToFlowDef_InvalidNext verifies that a next pointer
// referencing a non-existent stage is rejected.
func TestConvertSimpleToFlowDef_InvalidNext(t *testing.T) {
	sw := &SimpleWorkflow{
		Name: "bad-next",
		Stages: []SimpleStage{
			{Name: "a", Next: strPtr("nonexistent")},
		},
	}
	_, err := ConvertSimpleToFlowDef(sw)
	if err == nil {
		t.Fatal("expected error for invalid next")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want mention of 'not found'", err)
	}
}

// TestConvertSimpleToFlowDef_InvalidRollback verifies that a rollback_to
// referencing a non-existent stage is rejected.
func TestConvertSimpleToFlowDef_InvalidRollback(t *testing.T) {
	sw := &SimpleWorkflow{
		Name: "bad-rollback",
		Stages: []SimpleStage{
			{Name: "a", Next: strPtr("b")},
			{Name: "b", RollbackTo: strPtr("nonexistent")},
		},
	}
	_, err := ConvertSimpleToFlowDef(sw)
	if err == nil {
		t.Fatal("expected error for invalid rollback_to")
	}
	if !strings.Contains(err.Error(), "rollback_to") {
		t.Errorf("error = %v, want mention of 'rollback_to'", err)
	}
}

// TestConvertSimpleToFlowDef_DuplicateName verifies that duplicate stage
// names are rejected.
func TestConvertSimpleToFlowDef_DuplicateName(t *testing.T) {
	sw := &SimpleWorkflow{
		Name: "dup",
		Stages: []SimpleStage{
			{Name: "a"},
			{Name: "a"},
		},
	}
	_, err := ConvertSimpleToFlowDef(sw)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v, want mention of 'duplicate'", err)
	}
}

// TestConvertSimpleToFlowDef_NilInput verifies that nil input is rejected.
func TestConvertSimpleToFlowDef_NilInput(t *testing.T) {
	_, err := ConvertSimpleToFlowDef(nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

// TestLoadFile_SimpleFormat verifies that LoadFile auto-detects the simplified
// format and returns a valid FlowDef.
func TestLoadFile_SimpleFormat(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: bug-fix-workflow
description: Bug fix pipeline
start_stage: triage
end_stage: review
stages:
  - name: triage
    label: 问题理解
    next: branch
  - name: branch
    label: 切分支
    next: locate
    rollback_to: triage
  - name: locate
    label: 根因定位
    next: null
    rollback_to: branch
`
	path := filepath.Join(dir, "bug_fix.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if def.Metadata.Name != "bug-fix-workflow" {
		t.Errorf("name = %q, want bug-fix-workflow", def.Metadata.Name)
	}
	if len(def.Spec.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(def.Spec.Steps))
	}

	// Verify depends_on.
	if len(def.Spec.Steps[0].DependsOn) != 0 {
		t.Errorf("triage depends_on = %v, want empty", def.Spec.Steps[0].DependsOn)
	}
	if len(def.Spec.Steps[1].DependsOn) != 1 || def.Spec.Steps[1].DependsOn[0] != "triage" {
		t.Errorf("branch depends_on = %v, want [triage]", def.Spec.Steps[1].DependsOn)
	}
	if len(def.Spec.Steps[2].DependsOn) != 1 || def.Spec.Steps[2].DependsOn[0] != "branch" {
		t.Errorf("locate depends_on = %v, want [branch]", def.Spec.Steps[2].DependsOn)
	}

	// Validate.
	if err := Validate(def); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestLoadFile_StructuredFormat verifies that LoadFile still works for the
// structured FlowDef format (no regression).
func TestLoadFile_StructuredFormat(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `api_version: flowdef/v1
kind: Flow
metadata:
  name: structured-flow
  version: v1
spec:
  steps:
    - name: step1
      operator: stage
      stage: echo
`
	path := filepath.Join(dir, "structured.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	def, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if def.Metadata.Name != "structured-flow" {
		t.Errorf("name = %q, want structured-flow", def.Metadata.Name)
	}
	if len(def.Spec.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(def.Spec.Steps))
	}
}
