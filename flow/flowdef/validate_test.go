package flowdef

import (
	"strings"
	"testing"
)

// validFlowDef is a reusable, well-formed FlowDef used as the baseline for
// mutation in the validate tests.
func validFlowDef() *FlowDef {
	return &FlowDef{
		APIVersion: "flowdef/v1",
		Kind:       "Flow",
		Metadata:   Metadata{Name: "test-flow", Version: "1.0.0"},
		Spec: Spec{
			Steps: []StepDef{
				{Name: "a", Operator: "stage", Stage: "triage"},
				{Name: "b", Operator: "stage", Stage: "coder", DependsOn: []string{"a"}},
				{Name: "c", Operator: "stage", Stage: "committer", DependsOn: []string{"b"}},
			},
		},
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := Validate(validFlowDef()); err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
}

func TestValidate_EmptyName(t *testing.T) {
	def := validFlowDef()
	def.Metadata.Name = ""
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for empty metadata.name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name, got: %v", err)
	}
}

func TestValidate_NoSteps(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps = nil
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for no steps, got nil")
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("error should mention step, got: %v", err)
	}
}

func TestValidate_StepMissingName(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].Name = ""
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for step with empty name, got nil")
	}
}

func TestValidate_StepMissingOperator(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].Operator = ""
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for step with empty operator, got nil")
	}
}

func TestValidate_UnknownOperator(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].Operator = "magic_op"
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for unknown operator, got nil")
	}
	if !strings.Contains(err.Error(), "operator") {
		t.Errorf("error should mention operator, got: %v", err)
	}
}

func TestValidate_DependsOnMissingRef(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].DependsOn = []string{"nonexistent"}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for depends_on referencing missing step, got nil")
	}
	if !strings.Contains(err.Error(), "depends_on") {
		t.Errorf("error should mention depends_on, got: %v", err)
	}
}

func TestValidate_Cycle(t *testing.T) {
	def := validFlowDef()
	// a -> b -> c -> a  (cycle)
	def.Spec.Steps[2].DependsOn = []string{"c", "a"}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

func TestValidate_SelfCycle(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].DependsOn = []string{"b"}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for self-cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle, got: %v", err)
	}
}

func TestValidate_DuplicateStepName(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps = append(def.Spec.Steps, StepDef{Name: "a", Operator: "stage", Stage: "x"})
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error for duplicate step name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}
