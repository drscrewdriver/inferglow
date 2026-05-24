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

package model

import (
	"strings"
	"testing"
)

func TestOutputValidator_L4JSON_InvalidJSON(t *testing.T) {
	schema := &OutputSchema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	v := NewOutputValidator(schema)
	err := v.validate(&ModelResponse{Content: "not json{{"})
	if err == nil {
		t.Fatalf("expected error for invalid JSON content, got nil")
	}
	if !strings.Contains(err.Error(), "L4 validation: content is not valid JSON") {
		t.Fatalf("expected error to contain 'L4 validation: content is not valid JSON', got: %v", err)
	}
}

func TestOutputValidator_L4JSON_MissingRequired(t *testing.T) {
	schema := &OutputSchema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
		Required: []string{"name", "age"},
	}
	v := NewOutputValidator(schema)
	err := v.validate(&ModelResponse{Content: `{"name":"alice"}`})
	if err == nil {
		t.Fatalf("expected error for missing required field, got nil")
	}
	if !strings.Contains(err.Error(), `L4 validation: missing required field "age" in JSON output`) {
		t.Fatalf("expected error to contain 'L4 validation: missing required field \"age\" in JSON output', got: %v", err)
	}
}

func TestOutputValidator_L4JSON_TypeMismatch(t *testing.T) {
	schema := &OutputSchema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string"},
		},
		Required: []string{"name"},
	}
	v := NewOutputValidator(schema)
	err := v.validate(&ModelResponse{Content: `{"name":123}`})
	if err == nil {
		t.Fatalf("expected error for type mismatch, got nil")
	}
	if !strings.Contains(err.Error(), `L4 validation: field "name": expected string, got float64`) {
		t.Fatalf("expected error to contain 'L4 validation: field \"name\": expected string, got float64', got: %v", err)
	}
}

func TestOutputValidator_L4JSON_Valid(t *testing.T) {
	schema := &OutputSchema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
		Required: []string{"name"},
	}
	v := NewOutputValidator(schema)
	err := v.validate(&ModelResponse{Content: `{"name":"alice","age":30}`})
	if err != nil {
		t.Fatalf("expected no error for valid JSON, got: %v", err)
	}
}
