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

package schema

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// SC-MEDIUM-1: jsonschema oneOf/anyOf 支持
// ============================================================================

// TestJSONSchema_OneOf 验证 FieldDef 上的 OneOf 字段能正确生成 JSON Schema 的 oneOf 关键字。
func TestJSONSchema_OneOf(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"value": {
				Description: "value can be string or integer",
				OneOf: []*FieldDef{
					{Type: TypeString},
					{Type: TypeInt},
				},
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	valueSchema, ok := properties["value"].(map[string]any)
	if !ok {
		t.Fatal("value schema should be a map")
	}

	oneOf, ok := valueSchema["oneOf"].([]map[string]any)
	if !ok {
		t.Fatalf("valueSchema.oneOf should be []map[string]any, got %T", valueSchema["oneOf"])
	}

	if len(oneOf) != 2 {
		t.Fatalf("oneOf length = %d, want 2", len(oneOf))
	}

	if oneOf[0]["type"] != "string" {
		t.Errorf("oneOf[0].type = %v, want %q", oneOf[0]["type"], "string")
	}
	if oneOf[1]["type"] != "integer" {
		t.Errorf("oneOf[1].type = %v, want %q", oneOf[1]["type"], "integer")
	}

	if valueSchema["description"] != "value can be string or integer" {
		t.Errorf("description = %v, want %q", valueSchema["description"], "value can be string or integer")
	}
}

// TestJSONSchema_AnyOf 验证 FieldDef 上的 AnyOf 字段能正确生成 JSON Schema 的 anyOf 关键字。
func TestJSONSchema_AnyOf(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"identifier": {
				Description: "identifier can be string or integer",
				AnyOf: []*FieldDef{
					{Type: TypeString},
					{Type: TypeInt},
				},
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	idSchema, ok := properties["identifier"].(map[string]any)
	if !ok {
		t.Fatal("identifier schema should be a map")
	}

	anyOf, ok := idSchema["anyOf"].([]map[string]any)
	if !ok {
		t.Fatalf("idSchema.anyOf should be []map[string]any, got %T", idSchema["anyOf"])
	}

	if len(anyOf) != 2 {
		t.Fatalf("anyOf length = %d, want 2", len(anyOf))
	}

	if anyOf[0]["type"] != "string" {
		t.Errorf("anyOf[0].type = %v, want %q", anyOf[0]["type"], "string")
	}
	if anyOf[1]["type"] != "integer" {
		t.Errorf("anyOf[1].type = %v, want %q", anyOf[1]["type"], "integer")
	}
}

// TestJSONSchema_OneOfRoundTrip 验证 oneOf 在序列化往返后保留。
func TestJSONSchema_OneOfRoundTrip(t *testing.T) {
	original := &OutputSchema{
		Fields: map[string]*FieldDef{
			"value": {
				OneOf: []*FieldDef{
					{Type: TypeString},
					{Type: TypeInt},
				},
			},
		},
	}
	js := original.ToJSONSchema()

	// 验证 js 可以序列化为合法 JSON
	data, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	props, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing in decoded")
	}
	valueSchema, ok := props["value"].(map[string]any)
	if !ok {
		t.Fatal("value missing in props")
	}
	oneOf, ok := valueSchema["oneOf"].([]any)
	if !ok {
		t.Fatalf("oneOf missing or wrong type: %T", valueSchema["oneOf"])
	}
	if len(oneOf) != 2 {
		t.Errorf("oneOf length = %d, want 2", len(oneOf))
	}
}
