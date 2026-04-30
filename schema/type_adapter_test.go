package schema

import (
	"strings"
	"testing"
)

// ============================================================================
// StringTypeAdapter
// ============================================================================

func TestStringTypeAdapterAdaptString(t *testing.T) {
	a := &StringTypeAdapter{}
	v, err := a.Adapt("hello")
	if err != nil {
		t.Fatalf("Adapt(hello) error: %v", err)
	}
	if v != "hello" {
		t.Errorf("Adapt(hello) = %v, want hello", v)
	}
}

func TestStringTypeAdapterAdaptNonString(t *testing.T) {
	a := &StringTypeAdapter{}
	_, err := a.Adapt(123)
	if err == nil {
		t.Fatal("expected error for non-string")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("error should mention 'expected string', got: %v", err)
	}
}

func TestStringTypeAdapterValidate(t *testing.T) {
	a := &StringTypeAdapter{}
	if err := a.Validate("ok"); err != nil {
		t.Errorf("Validate(ok) error: %v", err)
	}
	if err := a.Validate(42); err == nil {
		t.Error("expected error for Validate(42)")
	}
}

// ============================================================================
// NumberTypeAdapter
// ============================================================================

func TestNumberTypeAdapterAdaptInt(t *testing.T) {
	a := &NumberTypeAdapter{}
	v, err := a.Adapt(42)
	if err != nil {
		t.Fatalf("Adapt(42) error: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 42.0 {
		t.Errorf("Adapt(42) = %v (%T), want 42.0 float64", v, v)
	}
}

func TestNumberTypeAdapterAdaptInt64(t *testing.T) {
	a := &NumberTypeAdapter{}
	v, err := a.Adapt(int64(100))
	if err != nil {
		t.Fatalf("Adapt(int64(100)) error: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 100.0 {
		t.Errorf("Adapt(int64(100)) = %v, want 100.0", v)
	}
}

func TestNumberTypeAdapterAdaptFloat64(t *testing.T) {
	a := &NumberTypeAdapter{}
	v, err := a.Adapt(3.14)
	if err != nil {
		t.Fatalf("Adapt(3.14) error: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 3.14 {
		t.Errorf("Adapt(3.14) = %v, want 3.14", v)
	}
}

func TestNumberTypeAdapterAdaptFloat32(t *testing.T) {
	a := &NumberTypeAdapter{}
	v, err := a.Adapt(float32(2.5))
	if err != nil {
		t.Fatalf("Adapt(float32(2.5)) error: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 2.5 {
		t.Errorf("Adapt(float32(2.5)) = %v, want 2.5", v)
	}
}

func TestNumberTypeAdapterAdaptNonNumber(t *testing.T) {
	a := &NumberTypeAdapter{}
	_, err := a.Adapt("not a number")
	if err == nil {
		t.Fatal("expected error for non-number")
	}
	if !strings.Contains(err.Error(), "expected number") {
		t.Errorf("error should mention 'expected number', got: %v", err)
	}
}

func TestNumberTypeAdapterValidateUint(t *testing.T) {
	a := &NumberTypeAdapter{}
	if err := a.Validate(uint(10)); err != nil {
		t.Errorf("Validate(uint(10)) error: %v", err)
	}
}

// ============================================================================
// BooleanTypeAdapter
// ============================================================================

func TestBooleanTypeAdapterAdaptTrue(t *testing.T) {
	a := &BooleanTypeAdapter{}
	v, err := a.Adapt(true)
	if err != nil {
		t.Fatalf("Adapt(true) error: %v", err)
	}
	if v != true {
		t.Errorf("Adapt(true) = %v, want true", v)
	}
}

func TestBooleanTypeAdapterAdaptFalse(t *testing.T) {
	a := &BooleanTypeAdapter{}
	v, err := a.Adapt(false)
	if err != nil {
		t.Fatalf("Adapt(false) error: %v", err)
	}
	if v != false {
		t.Errorf("Adapt(false) = %v, want false", v)
	}
}

func TestBooleanTypeAdapterAdaptNonBool(t *testing.T) {
	a := &BooleanTypeAdapter{}
	_, err := a.Adapt(1)
	if err == nil {
		t.Fatal("expected error for non-bool")
	}
	if !strings.Contains(err.Error(), "expected bool") {
		t.Errorf("error should mention 'expected bool', got: %v", err)
	}
}

// ============================================================================
// ArrayTypeAdapter
// ============================================================================

func TestArrayTypeAdapterAdaptAnySlice(t *testing.T) {
	a := &ArrayTypeAdapter{}
	in := []any{"a", "b", "c"}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	out, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(out) != 3 {
		t.Errorf("len = %d, want 3", len(out))
	}
}

func TestArrayTypeAdapterAdaptTypedSlice(t *testing.T) {
	a := &ArrayTypeAdapter{}
	in := []int{1, 2, 3}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	// 原样返回 []int
	if _, ok := v.([]int); !ok {
		t.Errorf("expected []int (passthrough), got %T", v)
	}
}

func TestArrayTypeAdapterAdaptArray(t *testing.T) {
	a := &ArrayTypeAdapter{}
	in := [3]int{1, 2, 3}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	_ = v
}

func TestArrayTypeAdapterAdaptNonArray(t *testing.T) {
	a := &ArrayTypeAdapter{}
	_, err := a.Adapt("string")
	if err == nil {
		t.Fatal("expected error for non-array")
	}
	if !strings.Contains(err.Error(), "expected array") {
		t.Errorf("error should mention 'expected array', got: %v", err)
	}
}

func TestArrayTypeAdapterAdaptNil(t *testing.T) {
	a := &ArrayTypeAdapter{}
	_, err := a.Adapt(nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestArrayTypeAdapterWithItemsConstraint(t *testing.T) {
	a := &ArrayTypeAdapter{Items: &StringTypeAdapter{}}
	in := []any{"a", "b", "c"}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	out, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(out) != 3 {
		t.Errorf("len = %d, want 3", len(out))
	}
}

func TestArrayTypeAdapterWithItemsConstraintFails(t *testing.T) {
	a := &ArrayTypeAdapter{Items: &StringTypeAdapter{}}
	in := []any{"a", 1, "c"}
	_, err := a.Adapt(in)
	if err == nil {
		t.Fatal("expected error for items[1] not string")
	}
	if !strings.Contains(err.Error(), "items[1]") {
		t.Errorf("error should mention 'items[1]', got: %v", err)
	}
}

func TestArrayTypeAdapterValidateWithItems(t *testing.T) {
	a := &ArrayTypeAdapter{Items: &NumberTypeAdapter{}}
	if err := a.Validate([]any{1, 2, 3}); err != nil {
		t.Errorf("Validate([1,2,3]) error: %v", err)
	}
	if err := a.Validate([]any{1, "x", 3}); err == nil {
		t.Error("expected error for items[1] not number")
	}
}

// ============================================================================
// ObjectTypeAdapter
// ============================================================================

func TestObjectTypeAdapterAdaptMapStringAny(t *testing.T) {
	a := &ObjectTypeAdapter{}
	in := map[string]any{"name": "Alice", "age": 30}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if m, ok := v.(map[string]any); !ok {
		t.Errorf("expected map[string]any, got %T", v)
	} else if m["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", m["name"])
	}
}

func TestObjectTypeAdapterAdaptNonMap(t *testing.T) {
	a := &ObjectTypeAdapter{}
	_, err := a.Adapt("string")
	if err == nil {
		t.Fatal("expected error for non-object")
	}
	if !strings.Contains(err.Error(), "expected object") {
		t.Errorf("error should mention 'expected object', got: %v", err)
	}
}

func TestObjectTypeAdapterAdaptNil(t *testing.T) {
	a := &ObjectTypeAdapter{}
	_, err := a.Adapt(nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestObjectTypeAdapterAdaptMapWithIntKey(t *testing.T) {
	a := &ObjectTypeAdapter{}
	in := map[int]string{1: "a"}
	_, err := a.Adapt(in)
	if err == nil {
		t.Fatal("expected error for map with int key")
	}
}

func TestObjectTypeAdapterWithPropertiesConstraint(t *testing.T) {
	a := &ObjectTypeAdapter{
		Properties: map[string]TypeAdapter{
			"name": &StringTypeAdapter{},
			"age":  &NumberTypeAdapter{},
		},
	}
	in := map[string]any{"name": "Bob", "age": 25}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	out, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if out["name"] != "Bob" {
		t.Errorf("name = %v, want Bob", out["name"])
	}
	// age 应被转换为 float64
	if f, ok := out["age"].(float64); !ok || f != 25.0 {
		t.Errorf("age = %v (%T), want 25.0 float64", out["age"], out["age"])
	}
}

func TestObjectTypeAdapterWithPropertiesFails(t *testing.T) {
	a := &ObjectTypeAdapter{
		Properties: map[string]TypeAdapter{
			"age": &NumberTypeAdapter{},
		},
	}
	in := map[string]any{"age": "old"}
	_, err := a.Adapt(in)
	if err == nil {
		t.Fatal("expected error for properties[age] not number")
	}
	if !strings.Contains(err.Error(), "properties[age]") {
		t.Errorf("error should mention 'properties[age]', got: %v", err)
	}
}

func TestObjectTypeAdapterValidateWithProperties(t *testing.T) {
	a := &ObjectTypeAdapter{
		Properties: map[string]TypeAdapter{
			"name": &StringTypeAdapter{},
		},
	}
	if err := a.Validate(map[string]any{"name": "Alice"}); err != nil {
		t.Errorf("Validate error: %v", err)
	}
	if err := a.Validate(map[string]any{"name": 42}); err == nil {
		t.Error("expected error for name not string")
	}
}

func TestObjectTypeAdapterAdaptStruct(t *testing.T) {
	a := &ObjectTypeAdapter{
		Properties: map[string]TypeAdapter{
			"name": &StringTypeAdapter{},
		},
	}
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	v, err := a.Adapt(Person{Name: "Charlie", Age: 40})
	if err != nil {
		t.Fatalf("Adapt(struct) error: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if m["name"] != "Charlie" {
		t.Errorf("name = %v, want Charlie", m["name"])
	}
}

func TestObjectTypeAdapterAdaptStructPointer(t *testing.T) {
	a := &ObjectTypeAdapter{}
	type Person struct {
		Name string `json:"name"`
	}
	p := &Person{Name: "Dave"}
	v, err := a.Adapt(p)
	if err != nil {
		t.Fatalf("Adapt(*struct) error: %v", err)
	}
	_ = v
}

// ============================================================================
// 工厂函数测试
// ============================================================================

func TestNewTypeAdapter(t *testing.T) {
	cases := []struct {
		dt     DataType
		target TypeAdapter
	}{
		{TypeString, &StringTypeAdapter{}},
		{TypeInt, &NumberTypeAdapter{}},
		{TypeFloat, &NumberTypeAdapter{}},
		{TypeBool, &BooleanTypeAdapter{}},
		{TypeList, &ArrayTypeAdapter{}},
		{TypeDict, &ObjectTypeAdapter{}},
		{TypeModel, &ObjectTypeAdapter{}},
		{TypeOptional, &StringTypeAdapter{}},
	}
	for _, c := range cases {
		a := NewTypeAdapter(c.dt)
		if a == nil {
			t.Errorf("NewTypeAdapter(%s) = nil", c.dt)
			continue
		}
		// 验证类型匹配（通过 Adapt 行为）
		switch c.dt {
		case TypeString:
			if _, err := a.Adapt("x"); err != nil {
				t.Errorf("StringAdapter.Adapt(x) error: %v", err)
			}
		case TypeInt, TypeFloat:
			if _, err := a.Adapt(1); err != nil {
				t.Errorf("NumberAdapter.Adapt(1) error: %v", err)
			}
		case TypeBool:
			if _, err := a.Adapt(true); err != nil {
				t.Errorf("BooleanAdapter.Adapt(true) error: %v", err)
			}
		case TypeList:
			if _, err := a.Adapt([]any{1}); err != nil {
				t.Errorf("ArrayAdapter.Adapt([1]) error: %v", err)
			}
		case TypeDict, TypeModel:
			if _, err := a.Adapt(map[string]any{}); err != nil {
				t.Errorf("ObjectAdapter.Adapt({}) error: %v", err)
			}
		}
	}
}

func TestNewTypeAdapterUnknown(t *testing.T) {
	a := NewTypeAdapter(DataType("unknown"))
	if a != nil {
		t.Errorf("NewTypeAdapter(unknown) should return nil, got %T", a)
	}
}

func TestNewTypeAdapterFromFieldDefSimple(t *testing.T) {
	fd := &FieldDef{Type: TypeString}
	a := NewTypeAdapterFromFieldDef(fd)
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if _, err := a.Adapt("ok"); err != nil {
		t.Errorf("Adapt error: %v", err)
	}
}

func TestNewTypeAdapterFromFieldDefList(t *testing.T) {
	fd := &FieldDef{
		Type:    TypeList,
		ItemDef: &FieldDef{Type: TypeInt},
	}
	a := NewTypeAdapterFromFieldDef(fd)
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	// Adapt []any{1,2,3} 应返回 float64 化的 []any
	v, err := a.Adapt([]any{1, 2, 3})
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	out, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(out) != 3 {
		t.Errorf("len = %d, want 3", len(out))
	}
	if f, ok := out[0].(float64); !ok || f != 1.0 {
		t.Errorf("out[0] = %v, want 1.0 float64", out[0])
	}
}

func TestNewTypeAdapterFromFieldDefDictWithChildren(t *testing.T) {
	fd := &FieldDef{
		Type: TypeDict,
		Children: map[string]*FieldDef{
			"name": {Type: TypeString},
			"age":  {Type: TypeInt},
		},
	}
	a := NewTypeAdapterFromFieldDef(fd)
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	in := map[string]any{"name": "Eve", "age": 28}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	out, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if out["name"] != "Eve" {
		t.Errorf("name = %v, want Eve", out["name"])
	}
	if f, ok := out["age"].(float64); !ok || f != 28.0 {
		t.Errorf("age = %v, want 28.0 float64", out["age"])
	}
}

func TestNewTypeAdapterFromFieldDefNil(t *testing.T) {
	a := NewTypeAdapterFromFieldDef(nil)
	if a != nil {
		t.Errorf("expected nil for nil FieldDef, got %T", a)
	}
}

// ============================================================================
// 接口兼容性测试
// ============================================================================

func TestTypeAdapterInterface(t *testing.T) {
	// 所有适配器都应实现 TypeAdapter 接口
	var _ TypeAdapter = (*StringTypeAdapter)(nil)
	var _ TypeAdapter = (*NumberTypeAdapter)(nil)
	var _ TypeAdapter = (*BooleanTypeAdapter)(nil)
	var _ TypeAdapter = (*ArrayTypeAdapter)(nil)
	var _ TypeAdapter = (*ObjectTypeAdapter)(nil)
}
