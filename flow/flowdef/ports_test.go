package flowdef

import (
	"strings"
	"testing"

	"github.com/inferglow/flow/stage"
)

func TestValidatePortConnections_NoPorts(t *testing.T) {
	// A definition with no port declarations must pass trivially.
	if err := ValidatePortConnections(validFlowDef()); err != nil {
		t.Fatalf("expected nil for port-less definition, got: %v", err)
	}
}

func TestValidatePortConnections_Nil(t *testing.T) {
	if err := ValidatePortConnections(nil); err == nil {
		t.Fatal("expected error for nil definition, got nil")
	}
}

func TestValidatePortConnections_Valid(t *testing.T) {
	def := validFlowDef()
	def.Spec.InputPorts = []stage.PortDef{{Name: "in", Type: stage.PortString}, {Name: "ctx", Type: stage.PortAny}}
	def.Spec.OutputPorts = []stage.PortDef{{Name: "out", Type: stage.PortJSON}}
	def.Spec.Steps[0].InputPorts = []stage.PortDef{{Name: "a.in", Type: stage.PortString}}
	def.Spec.Steps[0].OutputPorts = []stage.PortDef{{Name: "a.out", Type: stage.PortString}}

	if err := ValidatePortConnections(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePortConnections_EmptyPortName(t *testing.T) {
	def := validFlowDef()
	def.Spec.InputPorts = []stage.PortDef{{Name: "", Type: stage.PortAny}}
	err := ValidatePortConnections(def)
	if err == nil {
		t.Fatal("expected error for empty port name, got nil")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error should mention empty name, got: %v", err)
	}
}

func TestValidatePortConnections_DuplicateFlowInput(t *testing.T) {
	def := validFlowDef()
	def.Spec.InputPorts = []stage.PortDef{{Name: "x", Type: stage.PortString}, {Name: "x", Type: stage.PortString}}
	err := ValidatePortConnections(def)
	if err == nil {
		t.Fatal("expected error for duplicate flow input port, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestValidatePortConnections_DuplicateStepOutput(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[0].OutputPorts = []stage.PortDef{{Name: "o", Type: stage.PortString}, {Name: "o", Type: stage.PortString}}
	err := ValidatePortConnections(def)
	if err == nil {
		t.Fatal("expected error for duplicate step output port, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestStepInputPort(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[0].InputPorts = []stage.PortDef{{Name: "q", Type: stage.PortString}}

	p, ok := def.StepInputPort("a", "q")
	if !ok {
		t.Fatal("expected to find input port q on step a")
	}
	if p.Type != stage.PortString {
		t.Errorf("expected type string, got %q", p.Type)
	}

	if _, ok := def.StepInputPort("a", "missing"); ok {
		t.Error("expected ok=false for missing input port")
	}
	if _, ok := def.StepInputPort("nonexistent", "q"); ok {
		t.Error("expected ok=false for missing step")
	}
}

func TestStepOutputPort(t *testing.T) {
	def := validFlowDef()
	def.Spec.Steps[1].OutputPorts = []stage.PortDef{{Name: "r", Type: stage.PortAny}}

	p, ok := def.StepOutputPort("b", "r")
	if !ok {
		t.Fatal("expected to find output port r on step b")
	}
	if p.Type != stage.PortAny {
		t.Errorf("expected type any, got %q", p.Type)
	}

	if _, ok := def.StepOutputPort("b", "missing"); ok {
		t.Error("expected ok=false for missing output port")
	}
}