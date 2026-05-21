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

package tool

import (
	"testing"
)

func TestToolInfoStructCreation(t *testing.T) {
	info := &ToolInfo{
		Name:        "calculator",
		Description: "a simple calculator",
		Params: &ParameterInfo{
			Type:       "object",
			Properties: map[string]any{"x": map[string]any{"type": "number"}},
			Required:   []string{"x"},
		},
		Tags:     []string{"math", "utility"},
		Metadata: map[string]any{"version": "1.0"},
	}

	if info.Name != "calculator" {
		t.Errorf("Name: got %q want %q", info.Name, "calculator")
	}
	if info.Description != "a simple calculator" {
		t.Errorf("Description: got %q want %q", info.Description, "a simple calculator")
	}
	if info.Params == nil {
		t.Fatal("Params is nil")
	}
	if info.Params.Type != "object" {
		t.Errorf("Params.Type: got %q want %q", info.Params.Type, "object")
	}
	if len(info.Params.Required) != 1 || info.Params.Required[0] != "x" {
		t.Errorf("Params.Required: got %v want [x]", info.Params.Required)
	}
	if len(info.Tags) != 2 {
		t.Fatalf("Tags length: got %d want 2", len(info.Tags))
	}
	if info.Tags[0] != "math" || info.Tags[1] != "utility" {
		t.Errorf("Tags: got %v want [math utility]", info.Tags)
	}
	if info.Metadata["version"] != "1.0" {
		t.Errorf("Metadata[version]: got %v want 1.0", info.Metadata["version"])
	}
}

func TestToolInfoEmpty(t *testing.T) {
	info := &ToolInfo{}
	if info.Name != "" {
		t.Errorf("empty Name: got %q", info.Name)
	}
	if info.Params != nil {
		t.Errorf("empty Params: got %v", info.Params)
	}
	if info.Tags != nil {
		t.Errorf("empty Tags: got %v", info.Tags)
	}
	if info.Metadata != nil {
		t.Errorf("empty Metadata: got %v", info.Metadata)
	}
}

func TestParameterInfoStruct(t *testing.T) {
	params := &ParameterInfo{
		Type:       "object",
		Properties: map[string]any{"name": map[string]any{"type": "string"}},
		Required:   []string{"name"},
	}
	if params.Type != "object" {
		t.Errorf("Type: got %q want %q", params.Type, "object")
	}
	if params.Properties["name"] == nil {
		t.Error("Properties[name] is nil")
	}
	if len(params.Required) != 1 || params.Required[0] != "name" {
		t.Errorf("Required: got %v want [name]", params.Required)
	}
}

func TestWithDescription(t *testing.T) {
	info := &ToolInfo{}
	WithDescription("test description")(info)
	if info.Description != "test description" {
		t.Errorf("WithDescription: got %q want %q", info.Description, "test description")
	}
}

func TestWithTags(t *testing.T) {
	info := &ToolInfo{}
	WithTags("alpha", "beta")(info)
	if len(info.Tags) != 2 {
		t.Fatalf("WithTags length: got %d want 2", len(info.Tags))
	}
	if info.Tags[0] != "alpha" || info.Tags[1] != "beta" {
		t.Errorf("WithTags: got %v want [alpha beta]", info.Tags)
	}
}

func TestWithMetadata(t *testing.T) {
	meta := map[string]any{"key": "value", "num": 42}
	info := &ToolInfo{}
	WithMetadata(meta)(info)
	if info.Metadata["key"] != "value" {
		t.Errorf("WithMetadata[key]: got %v want value", info.Metadata["key"])
	}
	if info.Metadata["num"] != 42 {
		t.Errorf("WithMetadata[num]: got %v want 42", info.Metadata["num"])
	}
}

func TestOptionChaining(t *testing.T) {
	info := &ToolInfo{}
	WithDescription("chained")(info)
	WithTags("tag1")(info)
	WithMetadata(map[string]any{"k": "v"})(info)

	if info.Description != "chained" {
		t.Errorf("Description: got %q want %q", info.Description, "chained")
	}
	if len(info.Tags) != 1 || info.Tags[0] != "tag1" {
		t.Errorf("Tags: got %v want [tag1]", info.Tags)
	}
	if info.Metadata["k"] != "v" {
		t.Errorf("Metadata[k]: got %v want v", info.Metadata["k"])
	}
}
