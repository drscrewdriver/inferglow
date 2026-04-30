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
