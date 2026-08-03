package flowdef

import (
	"fmt"

	"github.com/inferglow/flow/stage"
)

// ValidatePortConnections validates the port declarations of a FlowDef
// (wp-b2, mirroring the flow port resolver rules):
//
//   - every declared port has a non-empty, unique name within its scope
//   - the flow-level input/output ports are structurally sound
//   - each step's input/output ports are structurally sound
//
// It is a structural (compile-time) check intended to run at flow-build time
// before execution. Connection-level checks (that a required input port has a
// source, that types are compatible) are the caller's responsibility and are
// provided by the generic helpers below.
//
// The function is additive: a FlowDef with no port declarations passes
// trivially, so legacy definitions are unaffected.
func ValidatePortConnections(def *FlowDef) error {
	if def == nil {
		return fmt.Errorf("flowdef: nil definition")
	}
	if err := uniquePortNames(def.Spec.InputPorts, "flow inputs"); err != nil {
		return err
	}
	if err := uniquePortNames(def.Spec.OutputPorts, "flow outputs"); err != nil {
		return err
	}
	for _, s := range def.Spec.Steps {
		if err := uniquePortNames(s.InputPorts, fmt.Sprintf("step %q inputs", s.Name)); err != nil {
			return err
		}
		if err := uniquePortNames(s.OutputPorts, fmt.Sprintf("step %q outputs", s.Name)); err != nil {
			return err
		}
	}
	return nil
}

// uniquePortNames asserts that every port in defs has a non-empty name and no
// two ports share a name within the given scope. Ports with empty names are
// rejected because they cannot be referenced by a connection.
func uniquePortNames(defs []stage.PortDef, scope string) error {
	seen := make(map[string]bool, len(defs))
	for _, d := range defs {
		if d.Name == "" {
			return fmt.Errorf("flowdef: %s: port with empty name", scope)
		}
		if seen[d.Name] {
			return fmt.Errorf("flowdef: %s: duplicate port name %q", scope, d.Name)
		}
		seen[d.Name] = true
	}
	return nil
}

// StepInputPort returns the input port definition of the given step with the
// given name, or ok=false when the step does not declare such an input port.
// It is a convenience wrapper for looking up a step's declared ports during
// connection validation.
func (def *FlowDef) StepInputPort(stepName, portName string) (stage.PortDef, bool) {
	for _, s := range def.Spec.Steps {
		if s.Name == stepName {
			return stage.FindPort(s.InputPorts, portName)
		}
	}
	return stage.PortDef{}, false
}

// StepOutputPort returns the output port definition of the given step with the
// given name, or ok=false when the step does not declare such an output port.
func (def *FlowDef) StepOutputPort(stepName, portName string) (stage.PortDef, bool) {
	for _, s := range def.Spec.Steps {
		if s.Name == stepName {
			return stage.FindPort(s.OutputPorts, portName)
		}
	}
	return stage.PortDef{}, false
}
