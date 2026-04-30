package schema

import (
	"testing"
)

// Check 2.1.1: DataType 枚举定义完整（str/int/float/bool/dict/list/model/optional）
func TestDataTypeConstants(t *testing.T) {
	if TypeString != "str" {
		t.Errorf("TypeString = %q, want %q", TypeString, "str")
	}
	if TypeInt != "int" {
		t.Errorf("TypeInt = %q, want %q", TypeInt, "int")
	}
	if TypeFloat != "float" {
		t.Errorf("TypeFloat = %q, want %q", TypeFloat, "float")
	}
	if TypeBool != "bool" {
		t.Errorf("TypeBool = %q, want %q", TypeBool, "bool")
	}
	if TypeDict != "dict" {
		t.Errorf("TypeDict = %q, want %q", TypeDict, "dict")
	}
	if TypeList != "list" {
		t.Errorf("TypeList = %q, want %q", TypeList, "list")
	}
	if TypeModel != "model" {
		t.Errorf("TypeModel = %q, want %q", TypeModel, "model")
	}
	if TypeOptional != "optional" {
		t.Errorf("TypeOptional = %q, want %q", TypeOptional, "optional")
	}
}

// Check 2.1.2: EnsurePolicy 枚举定义完整（presence/not_null）
func TestEnsurePolicyConstants(t *testing.T) {
	if EnsurePresence != "presence" {
		t.Errorf("EnsurePresence = %q, want %q", EnsurePresence, "presence")
	}
	if EnsureNotNull != "not_null" {
		t.Errorf("EnsureNotNull = %q, want %q", EnsureNotNull, "not_null")
	}
}

// Check 2.1.3: OutputFormat 枚举定义完整
func TestOutputFormatConstants(t *testing.T) {
	expectedFormats := map[OutputFormat]string{
		OutputJSON:       "json",
		OutputMarkdown:   "markdown",
		OutputText:       "text",
		OutputFlatMarkdown: "flat_markdown",
		OutputHybrid:     "hybrid",
		OutputXMLField:   "xml_field",
		OutputYAMLLiteral: "yaml_literal",
		OutputAuto:       "auto",
	}
	for fmt, want := range expectedFormats {
		if string(fmt) != want {
			t.Errorf("OutputFormat %v = %q, want %q", fmt, fmt, want)
		}
	}
}

// Check 2.1.4: FieldDef 包含所有字段
func TestFieldDefFields(t *testing.T) {
	field := &FieldDef{
		Type:        TypeString,
		Description: "test field",
		Ensure:      EnsurePresence,
		Required:    true,
		Children:    map[string]*FieldDef{},
		ItemDef:     nil,
	}

	if field.Type != TypeString {
		t.Errorf("field.Type = %v, want %v", field.Type, TypeString)
	}
	if field.Description != "test field" {
		t.Errorf("field.Description = %q, want %q", field.Description, "test field")
	}
	if field.Ensure != EnsurePresence {
		t.Errorf("field.Ensure = %v, want %v", field.Ensure, EnsurePresence)
	}
	if !field.Required {
		t.Error("field.Required should be true")
	}
	if field.Children == nil {
		t.Error("field.Children should not be nil")
	}
}

// Check 2.1.5: OutputSchema 包含所有字段
func TestOutputSchemaFields(t *testing.T) {
	schema := &OutputSchema{
		Format:    OutputJSON,
		EnsureAll: true,
		Fields:    map[string]*FieldDef{},
	}

	if schema.Format != OutputJSON {
		t.Errorf("schema.Format = %v, want %v", schema.Format, OutputJSON)
	}
	if !schema.EnsureAll {
		t.Error("schema.EnsureAll should be true")
	}
	if schema.Fields == nil {
		t.Error("schema.Fields should not be nil")
	}
}

// Check 2.1.5b: OutputSchema with nested Fields
func TestOutputSchemaWithFields(t *testing.T) {
	schema := DefineOutput[map[string]any]()
	schema.Format = OutputJSON
	schema.EnsureAll = true
	schema.Fields = map[string]*FieldDef{
		"name": {
			Type:        TypeString,
			Description: "User name",
			Required:    true,
		},
		"age": {
			Type:        TypeInt,
			Description: "User age",
		},
	}

	if schema.Format != OutputJSON {
		t.Error("Format should be JSON")
	}
	if !schema.EnsureAll {
		t.Error("EnsureAll should be true")
	}
	if len(schema.Fields) != 2 {
		t.Errorf("Fields length = %d, want 2", len(schema.Fields))
	}
	if _, ok := schema.Fields["name"]; !ok {
		t.Error("Fields missing 'name'")
	}
	if _, ok := schema.Fields["age"]; !ok {
		t.Error("Fields missing 'age'")
	}
}

// Check 2.1.6: DefineOutput[T any]() 泛型方法定义
func TestDefineOutputGenerics(t *testing.T) {
	// Test with different types
	s1 := DefineOutput[map[string]any]()
	if s1 == nil {
		t.Fatal("DefineOutput[map[string]any]() returned nil")
	}

	type User struct {
		Name string
		Age  int
	}
	s2 := DefineOutput[User]()
	if s2 == nil {
		t.Fatal("DefineOutput[User]() returned nil")
	}

	// Verify default values
	if s1.Format != "" {
		t.Error("default Format should be empty")
	}
	if s1.EnsureAll {
		t.Error("default EnsureAll should be false")
	}
}

// Test FieldDef with Children (nested dict)
func TestFieldDefWithChildren(t *testing.T) {
	child := &FieldDef{Type: TypeString, Description: "child field"}
	parent := &FieldDef{
		Type:     TypeDict,
		Children: map[string]*FieldDef{"child": child},
	}

	if len(parent.Children) != 1 {
		t.Error("parent should have 1 child")
	}
	if parent.Children["child"].Type != TypeString {
		t.Error("child type should be String")
	}
}

// Test FieldDef with ItemDef (list)
func TestFieldDefWithItemDef(t *testing.T) {
	item := &FieldDef{Type: TypeString, Description: "list item"}
	field := &FieldDef{
		Type:    TypeList,
		ItemDef: item,
	}

	if field.ItemDef == nil {
		t.Error("ItemDef should not be nil")
	}
	if field.ItemDef.Type != TypeString {
		t.Error("item type should be String")
	}
}
