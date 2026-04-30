package schema

import (
	"encoding/json"
	"testing"
)

// Check 2.4.1: str → {"type": "string"}
func TestJSONSchemaString(t *testing.T) {
	schema := &OutputSchema{
		Format: OutputJSON,
		Fields: map[string]*FieldDef{
			"name": {Type: TypeString, Description: "User name"},
		},
	}

	js := GenerateJSONSchema(schema)
	properties, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	nameSchema, ok := properties["name"].(map[string]any)
	if !ok {
		t.Fatal("name schema should be a map")
	}

	if nameSchema["type"] != "string" {
		t.Errorf("name.type = %v, want %q", nameSchema["type"], "string")
	}
}

// Check 2.4.2: int → {"type": "integer"}
func TestJSONSchemaInt(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"age": {Type: TypeInt},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	ageSchema := properties["age"].(map[string]any)

	if ageSchema["type"] != "integer" {
		t.Errorf("age.type = %v, want %q", ageSchema["type"], "integer")
	}
}

// Check 2.4.3: dict → {"type": "object", "properties": {...}, "required": [...]}
func TestJSONSchemaDict(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"metadata": {
				Type: TypeDict,
				Children: map[string]*FieldDef{
					"title": {Type: TypeString},
					"year":  {Type: TypeInt},
				},
				RequiredFields: []string{"title"},
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	metaSchema := properties["metadata"].(map[string]any)

	if metaSchema["type"] != "object" {
		t.Errorf("metadata.type = %v, want %q", metaSchema["type"], "object")
	}

	childProps, ok := metaSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("metadata should have properties")
	}
	if _, ok := childProps["title"]; !ok {
		t.Error("metadata.properties should contain 'title'")
	}
	if _, ok := childProps["year"]; !ok {
		t.Error("metadata.properties should contain 'year'")
	}

	required, ok := metaSchema["required"].([]string)
	if !ok {
		t.Error("metadata should have required field")
	}
	found := false
	for _, r := range required {
		if r == "title" {
			found = true
		}
	}
	if !found {
		t.Error("metadata.required should contain 'title'")
	}
}

// Check 2.4.4: list → {"type": "array", "items": {...}}
func TestJSONSchemaList(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"tags": {
				Type:    TypeList,
				ItemDef: &FieldDef{Type: TypeString},
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	tagsSchema := properties["tags"].(map[string]any)

	if tagsSchema["type"] != "array" {
		t.Errorf("tags.type = %v, want %q", tagsSchema["type"], "array")
	}

	items, ok := tagsSchema["items"].(map[string]any)
	if !ok {
		t.Fatal("tags should have items")
	}
	if items["type"] != "string" {
		t.Errorf("items.type = %v, want %q", items["type"], "string")
	}
}

// Check 2.4.5: 嵌套类型递归映射正确
func TestJSONSchemaNested(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"user": {
				Type: TypeDict,
				Children: map[string]*FieldDef{
					"name": {Type: TypeString},
					"address": {
						Type: TypeDict,
						Children: map[string]*FieldDef{
							"city": {Type: TypeString},
							"zip":  {Type: TypeString},
						},
					},
				},
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	userSchema := properties["user"].(map[string]any)
	addressSchema := userSchema["properties"].(map[string]any)["address"].(map[string]any)

	if addressSchema["type"] != "object" {
		t.Error("address should be object")
	}

	citySchema := addressSchema["properties"].(map[string]any)["city"].(map[string]any)
	if citySchema["type"] != "string" {
		t.Error("city should be string")
	}
}

// Check 2.4.6: Required 字段标记正确输出到 JSON Schema
func TestJSONSchemaRequired(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"name":  {Type: TypeString, Required: true},
			"age":   {Type: TypeInt, Required: false},
			"email": {Type: TypeString, Required: true},
		},
	}

	js := GenerateJSONSchema(schema)
	required, ok := js["required"].([]string)
	if !ok {
		t.Fatal("js should have required field")
	}

	if len(required) != 2 {
		t.Errorf("required length = %d, want 2", len(required))
	}

	containsName := false
	containsEmail := false
	for _, r := range required {
		if r == "name" {
			containsName = true
		}
		if r == "email" {
			containsEmail = true
		}
	}
	if !containsName {
		t.Error("required should contain 'name'")
	}
	if !containsEmail {
		t.Error("required should contain 'email'")
	}
}

// Test float and bool types
func TestJSONSchemaFloatAndBool(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"score": {Type: TypeFloat},
			"active": {Type: TypeBool},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)

	scoreSchema := properties["score"].(map[string]any)
	if scoreSchema["type"] != "number" {
		t.Errorf("score.type = %v, want %q", scoreSchema["type"], "number")
	}

	activeSchema := properties["active"].(map[string]any)
	if activeSchema["type"] != "boolean" {
		t.Errorf("active.type = %v, want %q", activeSchema["type"], "boolean")
	}
}

// Test JSON Schema is valid JSON
func TestJSONSchemaIsValidJSON(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"name":  {Type: TypeString, Required: true, Description: "Name"},
			"age":   {Type: TypeInt, Required: false},
			"tags":  {Type: TypeList, ItemDef: &FieldDef{Type: TypeString}},
		},
	}

	js := GenerateJSONSchema(schema)

	data, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// 验证可以反序列化回来
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
}

// Test Description is included
func TestJSONSchemaDescription(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"name": {
				Type:        TypeString,
				Description: "The user's full name",
			},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	nameSchema := properties["name"].(map[string]any)

	if nameSchema["description"] != "The user's full name" {
		t.Errorf("name.description = %v, want %q", nameSchema["description"], "The user's full name")
	}
}

// Test model type
func TestJSONSchemaModel(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"status": {Type: TypeModel},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	statusSchema := properties["status"].(map[string]any)

	if statusSchema["type"] != "object" {
		t.Errorf("status.type = %v, want %q", statusSchema["type"], "object")
	}
}

// Test optional type
func TestJSONSchemaOptional(t *testing.T) {
	schema := &OutputSchema{
		Fields: map[string]*FieldDef{
			"nickname": {Type: TypeOptional},
		},
	}

	js := GenerateJSONSchema(schema)
	properties := js["properties"].(map[string]any)
	nicknameSchema := properties["nickname"].(map[string]any)

	if nicknameSchema["type"] != "string" {
		t.Errorf("nickname.type = %v, want %q", nicknameSchema["type"], "string")
	}
}
