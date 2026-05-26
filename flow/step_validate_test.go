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
	"strings"
	"testing"

	"github.com/inferglow/schema"
)

func TestValidateStepOutput_NilSchema(t *testing.T) {
	output := map[string]any{"x": 1}
	if err := validateStepOutput(output, nil); err != nil {
		t.Fatalf("expected nil error for nil schema, got %v", err)
	}
}

func TestValidateStepOutput_ValidOutput(t *testing.T) {
	s := &schema.OutputSchema{
		Fields: map[string]*schema.FieldDef{
			"result": {Required: true},
		},
	}
	output := map[string]any{"result": "ok"}
	if err := validateStepOutput(output, s); err != nil {
		t.Fatalf("expected nil error for valid output, got %v", err)
	}
}

func TestValidateStepOutput_MissingField(t *testing.T) {
	s := &schema.OutputSchema{
		Fields: map[string]*schema.FieldDef{
			"result":   {Required: true},
			"optional": {Required: false},
		},
	}
	output := map[string]any{"optional": "x"}
	err := validateStepOutput(output, s)
	if err == nil {
		t.Fatalf("expected error for missing required field, got nil")
	}
	if !strings.Contains(err.Error(), "missing fields: result") {
		t.Fatalf("expected error to contain 'missing fields: result', got: %v", err)
	}
}

func TestValidateStepOutput_NonMapOutput(t *testing.T) {
	s := &schema.OutputSchema{
		Fields: map[string]*schema.FieldDef{
			"result": {Required: true},
		},
	}
	output := "some string"
	err := validateStepOutput(output, s)
	if err == nil {
		t.Fatalf("expected error for non-map output, got nil")
	}
	if !strings.Contains(err.Error(), "expected map[string]any, got string") {
		t.Fatalf("expected error to contain 'expected map[string]any, got string', got: %v", err)
	}
}
