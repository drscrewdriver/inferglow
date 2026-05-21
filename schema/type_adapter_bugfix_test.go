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
	"strings"
	"testing"
)

// ============================================================================
// SC-MEDIUM-3: type_adapter 类型别名
// ============================================================================

// MyString 是 string 的自定义命名类型（非真别名，但通常被称作类型别名）。
type MyString string

// MyInt 是 int 的自定义命名类型。
type MyInt int

// MyBool 是 bool 的自定义命名类型。
type MyBool bool

// TestTypeAdapter_TypeAlias 验证 StringTypeAdapter 能正确处理 type MyString string。
func TestTypeAdapter_TypeAlias(t *testing.T) {
	a := &StringTypeAdapter{}
	v, err := a.Adapt(MyString("hello"))
	if err != nil {
		t.Fatalf("Adapt(MyString) error: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T", v)
	}
	if s != "hello" {
		t.Errorf("Adapt(myString) = %q, want %q", s, "hello")
	}
}

// TestTypeAdapter_TypeAliasValidate 验证 Validate 也支持类型别名。
func TestTypeAdapter_TypeAliasValidate(t *testing.T) {
	a := &StringTypeAdapter{}
	if err := a.Validate(MyString("ok")); err != nil {
		t.Errorf("Validate(MyString) error: %v", err)
	}
}

// TestTypeAdapter_IntAlias 验证 NumberTypeAdapter 能正确处理 type MyInt int。
func TestTypeAdapter_IntAlias(t *testing.T) {
	a := &NumberTypeAdapter{}
	v, err := a.Adapt(MyInt(42))
	if err != nil {
		t.Fatalf("Adapt(MyInt) error: %v", err)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", v)
	}
	if f != 42.0 {
		t.Errorf("Adapt(MyInt) = %v, want 42.0", f)
	}
}

// TestTypeAdapter_BoolAlias 验证 BooleanTypeAdapter 能正确处理 type MyBool bool。
func TestTypeAdapter_BoolAlias(t *testing.T) {
	a := &BooleanTypeAdapter{}
	v, err := a.Adapt(MyBool(true))
	if err != nil {
		t.Fatalf("Adapt(MyBool) error: %v", err)
	}
	b, ok := v.(bool)
	if !ok {
		t.Fatalf("expected bool, got %T", v)
	}
	if b != true {
		t.Errorf("Adapt(MyBool) = %v, want true", b)
	}
}

// TestTypeAdapter_TypeAliasSlice 验证 ArrayTypeAdapter 能处理 []MyString。
func TestTypeAdapter_TypeAliasSlice(t *testing.T) {
	a := &ArrayTypeAdapter{Items: &StringTypeAdapter{}}
	in := []MyString{"a", "b", "c"}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt([]MyString) error: %v", err)
	}
	out, ok := v.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0] != "a" {
		t.Errorf("out[0] = %v, want %q", out[0], "a")
	}
}

// TestTypeAdapter_TypeAliasErrorMessage 验证错误消息中包含原始类型名。
func TestTypeAdapter_TypeAliasErrorMessage(t *testing.T) {
	a := &StringTypeAdapter{}
	_, err := a.Adapt(MyInt(42))
	if err == nil {
		t.Fatal("expected error for non-string")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("error should mention 'expected string', got: %v", err)
	}
}

// TestTypeAdapter_MapWithAliasValue 验证 ObjectTypeAdapter 能处理 map[string]MyString。
func TestTypeAdapter_MapWithAliasValue(t *testing.T) {
	a := &ObjectTypeAdapter{
		Properties: map[string]TypeAdapter{
			"name": &StringTypeAdapter{},
		},
	}
	in := map[string]MyString{"name": "Alice"}
	v, err := a.Adapt(in)
	if err != nil {
		t.Fatalf("Adapt(map[string]MyString) error: %v", err)
	}
	out, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	if out["name"] != "Alice" {
		t.Errorf("name = %v, want %q", out["name"], "Alice")
	}
}
