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

// StepFunc defines a single step function
type StepFunc func(ctx context.Context, input any) (any, error)

// Step represents a single executable step in a flow
type Step struct {
	Name   string
	Func   StepFunc
	Schema *schema.OutputSchema
}

// StepBuilder builds Step instances with chainable API
type StepBuilder struct {
	name   string
	fn     StepFunc
	schema *schema.OutputSchema
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

// Build creates the Step
func (b *StepBuilder) Build() *Step {
	return &Step{
		Name:   b.name,
		Func:   b.fn,
		Schema: b.schema,
	}
}
