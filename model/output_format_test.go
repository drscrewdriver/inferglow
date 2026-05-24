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
	"reflect"
	"testing"
)

func TestBuildJSONSchemaFromOutput_NilSchema(t *testing.T) {
	got := BuildJSONSchemaFromOutput(nil)
	want := map[string]any{"type": "object"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildJSONSchemaFromOutput_EmptyProperties(t *testing.T) {
	got := BuildJSONSchemaFromOutput(&OutputSchema{
		Properties: map[string]any{},
	})
	want := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildJSONSchemaFromOutput_FullSchema(t *testing.T) {
	props := map[string]any{
		"name": map[string]any{"type": "string"},
		"age":  map[string]any{"type": "integer"},
	}
	got := BuildJSONSchemaFromOutput(&OutputSchema{
		Properties: props,
		Required:   []string{"name"},
	})
	want := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
		"required":             []string{"name"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestForceJSONObject(t *testing.T) {
	cases := []struct {
		name string
		req  *ModelRequest
		want bool
	}{
		{
			name: "json_object mode",
			req:  &ModelRequest{Options: map[string]any{"response_format_mode": "json_object"}},
			want: true,
		},
		{
			name: "empty options",
			req:  &ModelRequest{Options: map[string]any{}},
			want: false,
		},
		{
			name: "nil request",
			req:  nil,
			want: false,
		},
		{
			name: "nil options",
			req:  &ModelRequest{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forceJSONObject(tc.req)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
