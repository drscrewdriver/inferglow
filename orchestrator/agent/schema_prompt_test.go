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

package agent

import (
	"strings"
	"testing"

	"github.com/inferglow/model"
)

func TestShouldInjectSchemaPrompt_NilOutput(t *testing.T) {
	req := &model.ModelRequest{}
	if shouldInjectSchemaPrompt(req) {
		t.Fatalf("expected false when Output is nil")
	}
}

func TestShouldInjectSchemaPrompt_AlreadyJSONSchema(t *testing.T) {
	req := &model.ModelRequest{
		Options: map[string]any{
			"response_format": map[string]any{"type": "json_schema"},
		},
		Output: &model.OutputSchema{
			Properties: map[string]any{
				"x": map[string]any{"type": "string"},
			},
		},
	}
	if shouldInjectSchemaPrompt(req) {
		t.Fatalf("expected false when response_format type is json_schema")
	}
}

func TestShouldInjectSchemaPrompt_ForcedJSONObject(t *testing.T) {
	req := &model.ModelRequest{
		Options: map[string]any{
			"response_format_mode": "json_object",
		},
		Output: &model.OutputSchema{
			Properties: map[string]any{
				"x": map[string]any{"type": "string"},
			},
		},
	}
	if !shouldInjectSchemaPrompt(req) {
		t.Fatalf("expected true when response_format_mode is json_object")
	}
}

func TestShouldInjectSchemaPrompt_NoResponseFormatCapability(t *testing.T) {
	req := &model.ModelRequest{
		Output: &model.OutputSchema{
			Properties: map[string]any{
				"x": map[string]any{"type": "string"},
			},
		},
		Options:      nil,
		OutputFormat: "",
	}
	if !shouldInjectSchemaPrompt(req) {
		t.Fatalf("expected true when no response_format capability (text provider)")
	}
}

func TestFormatSchemaInstruction(t *testing.T) {
	s := &model.OutputSchema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string", "description": "user name"},
			"age":  map[string]any{"type": "integer"},
		},
		Required: []string{"name"},
	}
	got := formatSchemaInstruction(s)
	if !strings.Contains(got, "输出格式要求") {
		t.Errorf("expected instruction to contain 输出格式要求, got %q", got)
	}
	if !strings.Contains(got, "name") {
		t.Errorf("expected instruction to contain field name, got %q", got)
	}
	if !strings.Contains(got, "age") {
		t.Errorf("expected instruction to contain field age, got %q", got)
	}
	if !strings.Contains(got, "必须包含的字段") {
		t.Errorf("expected instruction to contain required-field section marker, got %q", got)
	}
}
