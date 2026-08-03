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

	"github.com/inferglow/schema"
)

// StepFunc defines a single step function.
//
// StepFunc is the primary execution unit in InferGlow flows. It follows
// the LCEL (LangChain Expression Language) pattern: a generic any→any
// function that threads data through a pipeline.
//
// Relationship to stage.Func:
//   - stage.Func is a specialised form with typed Inputs/Outputs maps
//     and direct access to Context. Use stage.Adapt to convert a
//     Func into a StepFunc for use in LCEL chains or flow.Step.
//   - StepFunc can access Context via ContextFrom(ctx),
//     so it can do everything Func can, just with untyped input/output.
type StepFunc func(ctx context.Context, input any) (any, error)

// Step represents a single executable step in a flow
type Step struct {
	Name   string
	Func   StepFunc
	Schema *schema.OutputSchema

	// InputPorts/OutputPorts declare the step's explicit port interface
	// (spec B-3). They are optional: when empty, the step degrades to the
	// legacy any→any path and existing flows are unaffected.
	InputPorts  []PortDef
	OutputPorts []PortDef
}

// StepBuilder builds Step instances with chainable API
type StepBuilder struct {
	name        string
	fn          StepFunc
	schema      *schema.OutputSchema
	inputPorts  []PortDef
	outputPorts []PortDef
}

// NewStep creates a new StepBuilder
func NewStep(name string, fn StepFunc) *StepBuilder {
	return &StepBuilder{
		name: name,
		fn:   fn,
	}
}

// WithOutputSchema sets the output schema for validation
func (b *StepBuilder) WithOutputSchema(s *schema.OutputSchema) *StepBuilder {
	b.schema = s
	return b
}

// WithInputPorts declares the step's explicit input port schema (spec B-3).
func (b *StepBuilder) WithInputPorts(ports ...PortDef) *StepBuilder {
	b.inputPorts = ports
	return b
}

// WithOutputPorts declares the step's explicit output port schema (spec B-3).
func (b *StepBuilder) WithOutputPorts(ports ...PortDef) *StepBuilder {
	b.outputPorts = ports
	return b
}

// Build creates the Step
func (b *StepBuilder) Build() *Step {
	return &Step{
		Name:        b.name,
		Func:        b.fn,
		Schema:      b.schema,
		InputPorts:  b.inputPorts,
		OutputPorts: b.outputPorts,
	}
}
