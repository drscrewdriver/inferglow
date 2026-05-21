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
	"strings"
	"testing"
)

// ============================================================================
// OutputSchema MarshalJSON / UnmarshalJSON 测试
// ============================================================================

// Check 2.7: MarshalJSON 生成正确 JSON
func TestOutputSchemaMarshalJSON(t *testing.T) {
	schema := &OutputSchema{
		Format:    OutputJSON,
		EnsureAll: true,
		Fields: map[string]*FieldDef{
			"name": {Type: TypeString, Description: "User name", Required: true},
			"age":  {Type: TypeInt, Required: false},
		},
	}
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	str := string(data)
	if !strings.Contains(str, "name") {
		t.Errorf("JSON should contain 'name', got: %s", str)
	}
	if !strings.Contains(str, "User name") {
		t.Errorf("JSON should contain description, got: %s", str)
	}
	if !strings.Contains(str, "ensure_all") {
		t.Errorf("JSON should contain ensure_all, got: %s", str)
	}
}

// Check: UnmarshalJSON 正确解析
func TestOutputSchemaUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"format": "json",
		"ensure_all": true,
		"fields": {
			"name": {"type": "str", "description": "Name", "required": true},
			"age": {"type": "int"}
		}
	}`
	var schema OutputSchema
	if err := json.Unmarshal([]byte(jsonStr), &schema); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if schema.Format != OutputJSON {
		t.Errorf("Format = %v, want %v", schema.Format, OutputJSON)
	}
	if !schema.EnsureAll {
		t.Error("EnsureAll should be true")
	}
	if len(schema.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(schema.Fields))
	}
	name, ok := schema.Fields["name"]
	if !ok {
		t.Fatal("missing 'name' field")
	}
	if name.Type != TypeString {
		t.Errorf("name.Type = %v, want %v", name.Type, TypeString)
	}
	if name.Description != "Name" {
		t.Errorf("name.Description = %q, want 'Name'", name.Description)
	}
	if !name.Required {
		t.Error("name.Required should be true")
	}
	age, ok := schema.Fields["age"]
	if !ok {
		t.Fatal("missing 'age' field")
	}
	if age.Type != TypeInt {
		t.Errorf("age.Type = %v, want %v", age.Type, TypeInt)
	}
}

// Check: Marshal/Unmarshal 往返一致
func TestOutputSchemaRoundTripJSON(t *testing.T) {
	original := &OutputSchema{
		Format:    OutputJSON,
		EnsureAll: true,
		Fields: map[string]*FieldDef{
			"title": {Type: TypeString, Description: "Title", Required: true},
			"tags": {
				Type:    TypeList,
				ItemDef: &FieldDef{Type: TypeString},
			},
			"meta": {
				Type: TypeDict,
				Children: map[string]*FieldDef{
					"author": {Type: TypeString, Required: true},
				},
			},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var restored OutputSchema
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if restored.Format != original.Format {
		t.Errorf("Format mismatch: %v vs %v", restored.Format, original.Format)
	}
	if restored.EnsureAll != original.EnsureAll {
		t.Errorf("EnsureAll mismatch: %v vs %v", restored.EnsureAll, original.EnsureAll)
	}
	if len(restored.Fields) != len(original.Fields) {
		t.Fatalf("Fields length mismatch: %d vs %d", len(restored.Fields), len(original.Fields))
	}
	title, ok := restored.Fields["title"]
	if !ok {
		t.Fatal("missing 'title'")
	}
	if title.Description != "Title" {
		t.Errorf("title.Description = %q, want 'Title'", title.Description)
	}
	tags, ok := restored.Fields["tags"]
	if !ok {
		t.Fatal("missing 'tags'")
	}
	if tags.Type != TypeList {
		t.Errorf("tags.Type = %v, want %v", tags.Type, TypeList)
	}
	if tags.ItemDef == nil {
		t.Fatal("tags.ItemDef should not be nil")
	}
	if tags.ItemDef.Type != TypeString {
		t.Errorf("tags.ItemDef.Type = %v, want %v", tags.ItemDef.Type, TypeString)
	}
}

// Check: MarshalJSON 对 nil 接收者返回 "null"
func TestOutputSchemaMarshalNil(t *testing.T) {
	var s *OutputSchema
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal nil failed: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected 'null', got %s", string(data))
	}
}

// ============================================================================
// ToJSONSchema / OutputSchemaFromJSONSchema 测试
// ============================================================================

// Check: ToJSONSchema 生成合法 JSON Schema
func TestOutputSchemaToJSONSchema(t *testing.T) {
	schema := &OutputSchema{
		Format: OutputJSON,
		Fields: map[string]*FieldDef{
			"name": {Type: TypeString, Description: "Name", Required: true},
			"age":  {Type: TypeInt},
		},
	}
	js := schema.ToJSONSchema()
	if js == nil {
		t.Fatal("ToJSONSchema returned nil")
	}
	if js["type"] != "object" {
		t.Errorf("type = %v, want object", js["type"])
	}
	props, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}
	if len(props) != 2 {
		t.Errorf("expected 2 properties, got %d", len(props))
	}
	nameSchema, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatal("name schema should be a map")
	}
	if nameSchema["type"] != "string" {
		t.Errorf("name.type = %v, want string", nameSchema["type"])
	}
	// required 应包含 name
	reqList, ok := js["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}
	found := false
	for _, r := range reqList {
		if r == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("required should contain 'name'")
	}
}

// Check: ToJSONSchema 对 nil 接收者返回空 schema
func TestOutputSchemaToJSONSchemaNil(t *testing.T) {
	var s *OutputSchema
	js := s.ToJSONSchema()
	if js == nil {
		t.Fatal("ToJSONSchema returned nil for nil receiver")
	}
	if js["type"] != "object" {
		t.Errorf("type = %v, want object", js["type"])
	}
}

// Check: OutputSchemaFromJSONSchema 正确导入
func TestOutputSchemaFromJSONSchema(t *testing.T) {
	js := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "User name",
			},
			"age": map[string]any{
				"type": "integer",
			},
			"active": map[string]any{
				"type": "boolean",
			},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"meta": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"author": map[string]any{"type": "string"},
				},
				"required": []any{"author"},
			},
		},
		"required": []any{"name"},
	}
	schema := OutputSchemaFromJSONSchema(js)
	if schema == nil {
		t.Fatal("OutputSchemaFromJSONSchema returned nil")
	}
	name, ok := schema.Fields["name"]
	if !ok {
		t.Fatal("missing 'name'")
	}
	if name.Type != TypeString {
		t.Errorf("name.Type = %v, want %v", name.Type, TypeString)
	}
	if name.Description != "User name" {
		t.Errorf("name.Description = %q, want 'User name'", name.Description)
	}
	if !name.Required {
		t.Error("name.Required should be true")
	}
	age, ok := schema.Fields["age"]
	if !ok {
		t.Fatal("missing 'age'")
	}
	if age.Type != TypeInt {
		t.Errorf("age.Type = %v, want %v", age.Type, TypeInt)
	}
	active, ok := schema.Fields["active"]
	if !ok {
		t.Fatal("missing 'active'")
	}
	if active.Type != TypeBool {
		t.Errorf("active.Type = %v, want %v", active.Type, TypeBool)
	}
	tags, ok := schema.Fields["tags"]
	if !ok {
		t.Fatal("missing 'tags'")
	}
	if tags.Type != TypeList {
		t.Errorf("tags.Type = %v, want %v", tags.Type, TypeList)
	}
	if tags.ItemDef == nil {
		t.Fatal("tags.ItemDef should not be nil")
	}
	if tags.ItemDef.Type != TypeString {
		t.Errorf("tags.ItemDef.Type = %v, want %v", tags.ItemDef.Type, TypeString)
	}
	meta, ok := schema.Fields["meta"]
	if !ok {
		t.Fatal("missing 'meta'")
	}
	if meta.Type != TypeDict {
		t.Errorf("meta.Type = %v, want %v", meta.Type, TypeDict)
	}
	if meta.Children == nil {
		t.Fatal("meta.Children should not be nil")
	}
	author, ok := meta.Children["author"]
	if !ok {
		t.Fatal("missing 'meta.author'")
	}
	if author.Type != TypeString {
		t.Errorf("meta.author.Type = %v, want %v", author.Type, TypeString)
	}
	if !author.Required {
		t.Error("meta.author.Required should be true")
	}
}

// Check: OutputSchemaFromJSONSchema 对 nil 输入返回空 schema
func TestOutputSchemaFromJSONSchemaNil(t *testing.T) {
	schema := OutputSchemaFromJSONSchema(nil)
	if schema == nil {
		t.Fatal("returned nil")
	}
	if len(schema.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(schema.Fields))
	}
}

// Check: ToJSONSchema / OutputSchemaFromJSONSchema 往返基本一致
func TestOutputSchemaJSONSchemaRoundTrip(t *testing.T) {
	original := &OutputSchema{
		Fields: map[string]*FieldDef{
			"name": {Type: TypeString, Required: true, Description: "Name"},
			"age":  {Type: TypeInt},
		},
	}
	js := original.ToJSONSchema()
	restored := OutputSchemaFromJSONSchema(js)
	if len(restored.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(restored.Fields))
	}
	name, ok := restored.Fields["name"]
	if !ok {
		t.Fatal("missing 'name'")
	}
	if name.Type != TypeString {
		t.Errorf("name.Type = %v, want %v", name.Type, TypeString)
	}
	if !name.Required {
		t.Error("name.Required should be true")
	}
}
