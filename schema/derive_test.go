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
	"reflect"
	"testing"
)

// Check 2.1: DefineOutput[T] 基本推导正确
func TestDefineOutputDeriveBasic(t *testing.T) {
	type OutputResult struct {
		Summary string `json:"summary" description:"对话摘要"`
		Score   int    `json:"score" description:"评分"`
	}
	schema := DefineOutput[OutputResult]()

	if schema == nil {
		t.Fatal("DefineOutput returned nil")
	}
	if len(schema.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(schema.Fields))
	}
	summary, ok := schema.Fields["summary"]
	if !ok {
		t.Fatal("missing field 'summary'")
	}
	if summary.Type != TypeString {
		t.Errorf("summary.Type = %v, want %v", summary.Type, TypeString)
	}
	if summary.Description != "对话摘要" {
		t.Errorf("summary.Description = %q, want %q", summary.Description, "对话摘要")
	}
	if !summary.Required {
		t.Error("summary.Required should be true (no omitempty)")
	}
	score, ok := schema.Fields["score"]
	if !ok {
		t.Fatal("missing field 'score'")
	}
	if score.Type != TypeInt {
		t.Errorf("score.Type = %v, want %v", score.Type, TypeInt)
	}
	if score.Description != "评分" {
		t.Errorf("score.Description = %q, want %q", score.Description, "评分")
	}
	if !score.Required {
		t.Error("score.Required should be true (no omitempty)")
	}
}

// Check 2.2: 必填字段标记正确
func TestDefineOutputDeriveOmitempty(t *testing.T) {
	type Result struct {
		Required string   `json:"required"`
		Optional string   `json:"optional,omitempty"`
		Tags     []string `json:"tags,omitempty" description:"可选标签"`
	}
	schema := DefineOutput[Result]()

	req, ok := schema.Fields["required"]
	if !ok {
		t.Fatal("missing 'required'")
	}
	if !req.Required {
		t.Error("required should be Required=true")
	}
	opt, ok := schema.Fields["optional"]
	if !ok {
		t.Fatal("missing 'optional'")
	}
	if opt.Required {
		t.Error("optional should be Required=false (has omitempty)")
	}
	tags, ok := schema.Fields["tags"]
	if !ok {
		t.Fatal("missing 'tags'")
	}
	if tags.Required {
		t.Error("tags should be Required=false (has omitempty)")
	}
	if tags.Type != TypeList {
		t.Errorf("tags.Type = %v, want %v", tags.Type, TypeList)
	}
	if tags.Description != "可选标签" {
		t.Errorf("tags.Description = %q, want %q", tags.Description, "可选标签")
	}
}

// Check 2.3: 嵌套 struct 递归推导正确
func TestDefineOutputDeriveNestedStruct(t *testing.T) {
	type Address struct {
		City   string `json:"city"`
		Street string `json:"street,omitempty"`
	}
	type User struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}
	schema := DefineOutput[User]()

	if len(schema.Fields) != 2 {
		t.Fatalf("expected 2 top-level fields, got %d", len(schema.Fields))
	}
	addr, ok := schema.Fields["address"]
	if !ok {
		t.Fatal("missing 'address'")
	}
	if addr.Type != TypeDict {
		t.Errorf("address.Type = %v, want %v", addr.Type, TypeDict)
	}
	if addr.Children == nil {
		t.Fatal("address.Children should not be nil")
	}
	if len(addr.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(addr.Children))
	}
	city, ok := addr.Children["city"]
	if !ok {
		t.Fatal("missing 'address.city'")
	}
	if city.Type != TypeString {
		t.Errorf("city.Type = %v, want %v", city.Type, TypeString)
	}
	if !city.Required {
		t.Error("city should be Required=true")
	}
	street, ok := addr.Children["street"]
	if !ok {
		t.Fatal("missing 'address.street'")
	}
	if street.Required {
		t.Error("street should be Required=false (omitempty)")
	}
}

// Check 2.3b: 嵌套 struct 指针 (*T) 正确解引用
func TestDefineOutputDeriveNestedPointer(t *testing.T) {
	type Inner struct {
		Value string `json:"value"`
	}
	type Outer struct {
		Inner *Inner `json:"inner"`
	}
	schema := DefineOutput[Outer]()

	inner, ok := schema.Fields["inner"]
	if !ok {
		t.Fatal("missing 'inner'")
	}
	if inner.Type != TypeDict {
		t.Errorf("inner.Type = %v, want %v", inner.Type, TypeDict)
	}
	if inner.Children == nil {
		t.Fatal("inner.Children should not be nil")
	}
	if _, ok := inner.Children["value"]; !ok {
		t.Error("missing 'inner.value'")
	}
}

// Check 2.4: goTypeToDataType 映射完整
func TestGoTypeToDataType(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want DataType
	}{
		{"string", reflect.TypeOf(""), TypeString},
		{"int", reflect.TypeOf(int(0)), TypeInt},
		{"int64", reflect.TypeOf(int64(0)), TypeInt},
		{"uint", reflect.TypeOf(uint(0)), TypeInt},
		{"float32", reflect.TypeOf(float32(0)), TypeFloat},
		{"float64", reflect.TypeOf(float64(0)), TypeFloat},
		{"bool", reflect.TypeOf(false), TypeBool},
		{"slice", reflect.TypeOf([]string{}), TypeList},
		{"array", reflect.TypeOf([3]string{}), TypeList},
		{"map", reflect.TypeOf(map[string]any{}), TypeDict},
		{"ptr string", reflect.TypeOf((*string)(nil)), TypeString},
		{"nil", nil, TypeString},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := goTypeToDataType(c.typ)
			if got != c.want {
				t.Errorf("goTypeToDataType(%v) = %v, want %v", c.typ, got, c.want)
			}
		})
	}
}

// Check: parseJSONTag 正确解析
func TestParseJSONTag(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"name", "name"},
		{"name,omitempty", "name"},
		{"-", "-"},
		{",omitempty", ""},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := parseJSONTag(c.input)
			if got != c.want {
				t.Errorf("parseJSONTag(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// Check: DefineOutput[T] 对非 struct 类型保留旧行为（返回空 schema）
func TestDefineOutputNonStruct(t *testing.T) {
	s1 := DefineOutput[map[string]any]()
	if s1 == nil {
		t.Fatal("DefineOutput[map[string]any]() returned nil")
	}
	if s1.Format != "" {
		t.Errorf("Format = %q, want empty", s1.Format)
	}
	if s1.EnsureAll {
		t.Error("EnsureAll should be false by default")
	}
	if len(s1.Fields) != 0 {
		t.Errorf("expected 0 fields for map type, got %d", len(s1.Fields))
	}
}

// Check: DefineOutput[T] 对无 json tag 的 struct 字段跳过
func TestDefineOutputNoJSONTags(t *testing.T) {
	type User struct {
		Name string
		Age  int
	}
	schema := DefineOutput[User]()
	if len(schema.Fields) != 0 {
		t.Errorf("expected 0 fields (no json tags), got %d", len(schema.Fields))
	}
}

// Check: DefineOutput[T] 对 slice of struct 推导 ItemDef.Children
func TestDefineOutputDeriveSliceOfStruct(t *testing.T) {
	type Item struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	}
	type Order struct {
		Items []Item `json:"items"`
	}
	schema := DefineOutput[Order]()

	items, ok := schema.Fields["items"]
	if !ok {
		t.Fatal("missing 'items'")
	}
	if items.Type != TypeList {
		t.Errorf("items.Type = %v, want %v", items.Type, TypeList)
	}
	if items.ItemDef == nil {
		t.Fatal("items.ItemDef should not be nil for slice of struct")
	}
	if items.ItemDef.Type != TypeDict {
		t.Errorf("items.ItemDef.Type = %v, want %v", items.ItemDef.Type, TypeDict)
	}
	if items.ItemDef.Children == nil {
		t.Fatal("items.ItemDef.Children should not be nil")
	}
	if _, ok := items.ItemDef.Children["id"]; !ok {
		t.Error("missing 'id' in ItemDef.Children")
	}
	if _, ok := items.ItemDef.Children["name"]; !ok {
		t.Error("missing 'name' in ItemDef.Children")
	}
}

// Check: DefineOutputFromType 直接通过 reflect.Type 推导
func TestDefineOutputFromType(t *testing.T) {
	type Result struct {
		Title string `json:"title"`
	}
	schema := DefineOutputFromType(reflect.TypeOf(Result{}))
	if schema == nil {
		t.Fatal("DefineOutputFromType returned nil")
	}
	if _, ok := schema.Fields["title"]; !ok {
		t.Error("missing 'title' field")
	}
}

// Check: 字段跳过 json:"-"
func TestDefineOutputDeriveSkipDash(t *testing.T) {
	type Result struct {
		Public  string `json:"public"`
		Private string `json:"-"`
	}
	schema := DefineOutput[Result]()
	if len(schema.Fields) != 1 {
		t.Fatalf("expected 1 field (skip '-'), got %d", len(schema.Fields))
	}
	if _, ok := schema.Fields["public"]; !ok {
		t.Error("missing 'public'")
	}
	if _, ok := schema.Fields["private"]; ok {
		t.Error("'private' should be skipped (json:'-')")
	}
}

// Check: 非导出字段被跳过
func TestDefineOutputDeriveSkipUnexported(t *testing.T) {
	type Result struct {
		Public  string `json:"public"`
		private string // json tag 对 unexported 字段无效，无需添加
	}
	schema := DefineOutput[Result]()
	if len(schema.Fields) != 1 {
		t.Fatalf("expected 1 field (skip unexported), got %d", len(schema.Fields))
	}
	if _, ok := schema.Fields["public"]; !ok {
		t.Error("missing 'public'")
	}
	if _, ok := schema.Fields["private"]; ok {
		t.Error("'private' should be skipped (unexported)")
	}
}
