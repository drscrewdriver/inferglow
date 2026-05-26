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

	"github.com/inferglow/schema"
)

func TestFlow_Execute_SchemaValidation_Passes(t *testing.T) {
	schema := &schema.OutputSchema{
		Fields: map[string]*schema.FieldDef{
			"result": {Required: true},
		},
	}
	step := NewStep("validate", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"result": "ok"}, nil
	}).WithOutputSchema(schema).Build()

	f := NewFlow().AddStep(step).Build()

	exec := f.Execute(context.Background(), "input")
	if exec.State.Status != StatusCompleted {
		t.Fatalf("expected StatusCompleted, got %s; errors: %v", exec.State.Status, exec.State.Errors)
	}
}

func TestFlow_Execute_SchemaValidation_Fails(t *testing.T) {
	schema := &schema.OutputSchema{
		Fields: map[string]*schema.FieldDef{
			"result": {Required: true},
		},
	}
	step := NewStep("validate", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"other": "x"}, nil // missing "result"
	}).WithOutputSchema(schema).Build()

	f := NewFlow().AddStep(step).Build()

	exec := f.Execute(context.Background(), "input")
	if exec.State.Status != StatusFailed {
		t.Fatalf("expected StatusFailed, got %s", exec.State.Status)
	}
	if len(exec.State.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	errStr := exec.State.Errors[0].Error()
	if !strings.Contains(errStr, "output validation failed") {
		t.Fatalf("expected error to contain 'output validation failed', got: %s", errStr)
	}
	if !strings.Contains(errStr, "validate") {
		t.Fatalf("expected error to contain step name 'validate', got: %s", errStr)
	}
}
