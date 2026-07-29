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
	"context"
	"strings"
	"testing"
)

func dummyFn(_ context.Context, _ any) (any, error) { return nil, nil }

func portSteps() map[string]*Step {
	return map[string]*Step{
		"a": NewStep("a", dummyFn).
			WithOutputPorts(PortDef{Name: "out", Type: PortString}).
			Build(),
		"b": NewStep("b", dummyFn).
			WithInputPorts(PortDef{Name: "in", Type: PortString}).
			WithOutputPorts(PortDef{Name: "out", Type: PortString}).
			Build(),
		"c": NewStep("c", dummyFn).
			WithInputPorts(PortDef{Name: "in", Type: PortString}).
			Build(),
	}
}

func TestValidatePortConnections_Valid(t *testing.T) {
	steps := portSteps()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "a", FromPort: "out", ToStep: "b", ToPort: "in"}},
	}}
	if err := ValidatePortConnections(steps, edges); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePortConnections_NoPortsTriviallyPasses(t *testing.T) {
	steps := map[string]*Step{"a": NewStep("a", dummyFn).Build()}
	if err := ValidatePortConnections(steps, nil); err != nil {
		t.Fatalf("port-less flow should pass, got %v", err)
	}
}

func TestValidatePortConnections_UnknownSourceStep(t *testing.T) {
	steps := portSteps()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "ghost", FromPort: "out", ToStep: "b", ToPort: "in"}},
	}}
	err := ValidatePortConnections(steps, edges)
	if err == nil {
		t.Fatal("expected error for unknown source step, got nil")
	}
	if !strings.Contains(err.Error(), "source step") {
		t.Errorf("expected source step reference, got %v", err)
	}
}

func TestValidatePortConnections_MissingOutputPort(t *testing.T) {
	steps := portSteps()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "a", FromPort: "nope", ToStep: "b", ToPort: "in"}},
	}}
	err := ValidatePortConnections(steps, edges)
	if err == nil {
		t.Fatal("expected error for missing output port, got nil")
	}
	if !strings.Contains(err.Error(), "output port") {
		t.Errorf("expected output port reference, got %v", err)
	}
}

func TestValidatePortConnections_MissingInputPort(t *testing.T) {
	steps := portSteps()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "a", FromPort: "out", ToStep: "b", ToPort: "nope"}},
	}}
	err := ValidatePortConnections(steps, edges)
	if err == nil {
		t.Fatal("expected error for missing input port, got nil")
	}
	if !strings.Contains(err.Error(), "input port") {
		t.Errorf("expected input port reference, got %v", err)
	}
}

func TestValidatePortConnections_TypeMismatch(t *testing.T) {
	steps := portSteps()
	steps["b"] = NewStep("b", dummyFn).
		WithInputPorts(PortDef{Name: "in", Type: PortInt}).
		Build()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "a", FromPort: "out", ToStep: "b", ToPort: "in"}},
	}}
	err := ValidatePortConnections(steps, edges)
	if err == nil {
		t.Fatal("expected error for type mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("expected type mismatch reference, got %v", err)
	}
}

func TestValidatePortConnections_AnyCompatible(t *testing.T) {
	steps := portSteps()
	steps["b"] = NewStep("b", dummyFn).
		WithInputPorts(PortDef{Name: "in", Type: PortAny}).
		Build()
	edges := []Edge{{
		From: "a", To: "b",
		PortMappings: []EdgePort{{FromStep: "a", FromPort: "out", ToStep: "b", ToPort: "in"}},
	}}
	// PortAny input must accept a string-typed output.
	if err := ValidatePortConnections(steps, edges); err != nil {
		t.Fatalf("PortAny should be compatible, got %v", err)
	}
}

func TestValidatePortConnections_RequiredInputUnsatisfied(t *testing.T) {
	steps := portSteps()
	steps["c"] = NewStep("c", dummyFn).
		WithInputPorts(PortDef{Name: "in", Type: PortString, Required: true}).
		Build()
	// No edge feeds c at all.
	err := ValidatePortConnections(steps, nil)
	if err == nil {
		t.Fatal("expected error for unsatisfied required input, got nil")
	}
	if !strings.Contains(err.Error(), "required input port") {
		t.Errorf("expected required input reference, got %v", err)
	}
}

func TestBuildValidated_Valid(t *testing.T) {
	a := NewStep("a", dummyFn).WithOutputPorts(PortDef{Name: "out", Type: PortString}).Build()
	b := NewStep("b", dummyFn).WithInputPorts(PortDef{Name: "in", Type: PortString}).Build()
	f, err := NewFlow().AddStep(a).Connect(b, EdgePort{FromPort: "out", ToPort: "in"}).BuildValidated()
	if err != nil {
		t.Fatalf("BuildValidated returned error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil flow")
	}
}

func TestBuildValidated_InvalidMapping(t *testing.T) {
	a := NewStep("a", dummyFn).WithOutputPorts(PortDef{Name: "out", Type: PortString}).Build()
	b := NewStep("b", dummyFn).WithInputPorts(PortDef{Name: "in", Type: PortString}).Build()
	_, err := NewFlow().AddStep(a).Connect(b, EdgePort{FromPort: "bad", ToPort: "in"}).BuildValidated()
	if err == nil {
		t.Fatal("expected BuildValidated error for bad mapping, got nil")
	}
}

func TestConnect_EmptyMappingsEquivalentToTo(t *testing.T) {
	a := NewStep("a", dummyFn).Build()
	b := NewStep("b", dummyFn).Build()
	f := NewFlow().AddStep(a).Connect(b).Build()
	if len(f.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(f.edges))
	}
	if len(f.edges[0].PortMappings) != 0 {
		t.Errorf("expected empty PortMappings, got %v", f.edges[0].PortMappings)
	}
}

func TestPortType_CompatibleWith(t *testing.T) {
	if !PortString.CompatibleWith(PortString) {
		t.Error("same type should be compatible")
	}
	if !PortString.CompatibleWith(PortAny) {
		t.Error("any should be compatible with string")
	}
	if !PortAny.CompatibleWith(PortInt) {
		t.Error("any should be compatible with int")
	}
	if PortString.CompatibleWith(PortInt) {
		t.Error("string and int should not be compatible")
	}
}

func TestFindPort(t *testing.T) {
	ports := []PortDef{{Name: "x", Type: PortString}}
	p, ok := FindPort(ports, "x")
	if !ok || p.Type != PortString {
		t.Errorf("expected to find x, got %v/%v", p, ok)
	}
	if _, ok := FindPort(ports, "missing"); ok {
		t.Error("expected ok=false for missing port")
	}
}

func TestPortResolver_NilReceiver(t *testing.T) {
	var r *PortResolver
	if err := r.Validate(nil); err == nil {
		t.Fatal("expected error for nil receiver, got nil")
	}
}