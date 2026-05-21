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

import "reflect"

// DataType 定义支持的字段类型
type DataType string

const (
	// TypeString represents a string field type.
	TypeString DataType = "str"
	// TypeInt represents an integer field type.
	TypeInt DataType = "int"
	// TypeFloat represents a floating-point field type.
	TypeFloat DataType = "float"
	// TypeBool represents a boolean field type.
	TypeBool DataType = "bool"
	// TypeDict represents a dictionary/object field type.
	TypeDict DataType = "dict"
	// TypeList represents a list/array field type.
	TypeList DataType = "list"
	// TypeModel represents a model reference field type.
	TypeModel DataType = "model"
	// TypeOptional represents an optional wrapper field type.
	TypeOptional DataType = "optional"
)

// EnsurePolicy 定义字段存在性策略
type EnsurePolicy string

const (
	// EnsurePresence requires the field to be present.
	EnsurePresence EnsurePolicy = "presence"
	// EnsureNotNull requires the field value to be non-null.
	EnsureNotNull EnsurePolicy = "not_null"
)

// OutputFormat 定义输出格式
type OutputFormat string

const (
	// OutputJSON serializes output as JSON.
	OutputJSON OutputFormat = "json"
	// OutputMarkdown serializes output as Markdown.
	OutputMarkdown OutputFormat = "markdown"
	// OutputText serializes output as plain text.
	OutputText OutputFormat = "text"
	// OutputFlatMarkdown serializes output as flat Markdown.
	OutputFlatMarkdown OutputFormat = "flat_markdown"
	// OutputHybrid serializes output as a hybrid of Markdown and JSON.
	OutputHybrid OutputFormat = "hybrid"
	// OutputXMLField serializes output using XML field encoding.
	OutputXMLField OutputFormat = "xml_field"
	// OutputYAMLLiteral serializes output as a YAML literal.
	OutputYAMLLiteral OutputFormat = "yaml_literal"
	// OutputAuto lets the engine pick the output format.
	OutputAuto OutputFormat = "auto"
)

// FieldDef 定义单个字段
type FieldDef struct {
	Type           DataType
	Description    string
	Ensure         EnsurePolicy
	Required       bool
	RequiredFields []string
	Children       map[string]*FieldDef
	ItemDef        *FieldDef
	// OneOf 表示字段必须匹配其中一个子 schema（对应 JSON Schema 的 oneOf 关键字）。
	OneOf []*FieldDef
	// AnyOf 表示字段至少匹配其中一个子 schema（对应 JSON Schema 的 anyOf 关键字）。
	AnyOf []*FieldDef
}

// OutputSchema 定义输出契约
type OutputSchema struct {
	Format    OutputFormat
	EnsureAll bool
	Fields    map[string]*FieldDef
}

// DefineOutput 泛型方法，用于定义输出 Schema。
// 当 T 为 Go struct 类型时，自动通过 reflect 推导字段：
//   - json tag 作为 field name（缺省时跳过该字段）
//   - description tag 作为字段描述
//   - json tag 含 ",omitempty" 时 Required=false，否则 Required=true
//   - 嵌套 struct 递归推导为 TypeDict + Children
//   - slice/array 推导为 TypeList，元素若为 struct 则写入 ItemDef.Children
//
// 当 T 为 map[string]any 等非 struct 类型时，返回空 Schema（保留旧行为）。
func DefineOutput[T any]() *OutputSchema {
	return DefineOutputFromType(reflect.TypeOf((*T)(nil)).Elem())
}
