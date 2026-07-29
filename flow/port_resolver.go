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

package flow

import (
	"fmt"
)

// PortResolver validates port-level connections in a flow graph (spec B-2).
// It is a build-time check that runs once the steps and their edges are known,
// before the flow is executed. The five rules are:
//
//  1. every source step of a mapping exists and declares the named output port
//  2. every target step of a mapping exists and declares the named input port
//  3. port types are compatible (PortAny is compatible with everything, else
//     exact match)
//  4. no dangling connections (a mapping references a step that is not present)
//  5. every Required input port has a source connection
//
// A flow with no port declarations (or only unordered edges) passes trivially,
// so legacy flows are unaffected.
type PortResolver struct {
	steps map[string]*Step
}

// NewPortResolver returns a PortResolver over the given steps keyed by name.
func NewPortResolver(steps map[string]*Step) *PortResolver {
	return &PortResolver{steps: steps}
}

// Validate is the fluent entry point: it validates every edge's port mappings
// against the resolver's step set. Edges without mappings are ignored (the
// legacy any→any path). It returns an aggregate error describing the first
// violation encountered.
func (r *PortResolver) Validate(edges []Edge) error {
	if r == nil {
		return fmt.Errorf("flow port resolver: nil receiver")
	}
	// Collect all declared input ports per step for the Required check.
	// Also track which input ports are already fed by a mapping.
	requiredInputs := map[string][]PortDef{} // step -> required input ports
	connectedInputs := map[string]map[string]bool{}
	for name, s := range r.steps {
		for _, p := range s.InputPorts {
			if p.Required {
				requiredInputs[name] = append(requiredInputs[name], p)
			}
		}
	}

	for _, e := range edges {
		for _, m := range e.PortMappings {
			if err := r.validateMapping(m); err != nil {
				return err
			}
			if connectedInputs[m.ToStep] == nil {
				connectedInputs[m.ToStep] = map[string]bool{}
			}
			connectedInputs[m.ToStep][m.ToPort] = true
		}
	}

	// Rule 5: every Required input port must be fed by at least one mapping.
	for step, ports := range requiredInputs {
		feeds := connectedInputs[step]
		for _, p := range ports {
			if !feeds[p.Name] {
				return fmt.Errorf("flow port resolver: step %q required input port %q has no source connection", step, p.Name)
			}
		}
	}
	return nil
}

// validateMapping checks a single port mapping against the resolver's steps.
func (r *PortResolver) validateMapping(m EdgePort) error {
	// Rule 1 & 4: source step exists and declares the output port.
	src, ok := r.steps[m.FromStep]
	if !ok {
		return fmt.Errorf("flow port resolver: mapping references unknown source step %q", m.FromStep)
	}
	srcPort, ok := FindPort(src.OutputPorts, m.FromPort)
	if !ok {
		return fmt.Errorf("flow port resolver: source step %q has no output port %q", m.FromStep, m.FromPort)
	}

	// Rule 2 & 4: target step exists and declares the input port.
	dst, ok := r.steps[m.ToStep]
	if !ok {
		return fmt.Errorf("flow port resolver: mapping references unknown target step %q", m.ToStep)
	}
	dstPort, ok := FindPort(dst.InputPorts, m.ToPort)
	if !ok {
		return fmt.Errorf("flow port resolver: target step %q has no input port %q", m.ToStep, m.ToPort)
	}

	// Rule 3: port type compatibility.
	if !srcPort.Type.CompatibleWith(dstPort.Type) {
		return fmt.Errorf("flow port resolver: port type mismatch: %q.%s (%s) -> %q.%s (%s)",
			m.FromStep, m.FromPort, srcPort.Type, m.ToStep, m.ToPort, dstPort.Type)
	}
	return nil
}

// ValidatePortConnections is a package-level convenience wrapper that builds a
// PortResolver over the given steps and validates the given edges. It mirrors
// the spec B-2 signature.
func ValidatePortConnections(steps map[string]*Step, edges []Edge) error {
	return NewPortResolver(steps).Validate(edges)
}