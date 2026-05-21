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
	"fmt"
	"reflect"
	"time"
)

// fieldDefDTO 是 FieldDef 的可序列化表示。
// 用于 MarshalJSON/UnmarshalJSON，将 DataType 转为字符串。
type fieldDefDTO struct {
	Type           DataType             `json:"type"`
	Description    string               `json:"description,omitempty"`
	Ensure         EnsurePolicy         `json:"ensure,omitempty"`
	Required       bool                 `json:"required,omitempty"`
	RequiredFields []string             `json:"required_fields,omitempty"`
	Children       map[string]*FieldDef `json:"children,omitempty"`
	ItemDef        *FieldDef            `json:"item_def,omitempty"`
}

// outputSchemaDTO 是 OutputSchema 的可序列化表示。
type outputSchemaDTO struct {
	Format    OutputFormat         `json:"format,omitempty"`
	EnsureAll bool                 `json:"ensure_all,omitempty"`
	Fields    map[string]*FieldDef `json:"fields"`
}

// MarshalJSON 实现 json.Marshaler 接口。
// OutputSchema 被序列化为包含 format/ensure_all/fields 三个字段的 JSON 对象。
// Fields 中的每个 FieldDef 直接以结构体形式序列化（DataType 本身就是 string）。
func (s *OutputSchema) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	dto := outputSchemaDTO{
		Format:    s.Format,
		EnsureAll: s.EnsureAll,
		Fields:    s.Fields,
	}
	return json.Marshal(dto)
}

// UnmarshalJSON 实现 json.Unmarshaler 接口。
// 与 MarshalJSON 配对，从 JSON 反序列化为 OutputSchema。
func (s *OutputSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("UnmarshalJSON: nil receiver")
	}
	var dto outputSchemaDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return fmt.Errorf("unmarshal OutputSchema: %w", err)
	}
	s.Format = dto.Format
	s.EnsureAll = dto.EnsureAll
	if dto.Fields == nil {
		s.Fields = make(map[string]*FieldDef)
	} else {
		s.Fields = dto.Fields
	}
	return nil
}

// ToJSONSchema 将 OutputSchema 导出为 JSON Schema Draft 表示。
// 复用 GenerateJSONSchema 实现。
func (s *OutputSchema) ToJSONSchema() map[string]any {
	if s == nil {
		return map[string]any{
			"type":       "object",
			"properties": make(map[string]any),
		}
	}
	return GenerateJSONSchema(s)
}

// OutputSchemaFromJSONSchema 从 JSON Schema Draft 导入为 OutputSchema。
// 支持的 JSON Schema 关键字：
//   - "type": "object"（顶层），以及 string/integer/number/boolean/array/object（字段）
//   - "properties": 字段定义映射
//   - "required": 必填字段名列表
//   - "description": 字段描述
//   - "items": 数组元素类型（递归）
//   - "properties"（嵌套）：对象类型的子字段（递归）
func OutputSchemaFromJSONSchema(js map[string]any) *OutputSchema {
	schema := &OutputSchema{
		Fields: make(map[string]*FieldDef),
	}
	if js == nil {
		return schema
	}
	props, _ := js["properties"].(map[string]any)
	if props == nil {
		return schema
	}
	requiredSet := collectRequiredSet(js["required"])
	for name, propAny := range props {
		propMap, ok := propAny.(map[string]any)
		if !ok {
			continue
		}
		schema.Fields[name] = jsonSchemaToFieldDef(propMap, requiredSet[name])
	}
	return schema
}

// collectRequiredSet 从 JSON Schema 的 "required" 字段构建 set。
// 兼容 []string 和 []any 两种表示。
func collectRequiredSet(required any) map[string]bool {
	result := make(map[string]bool)
	switch v := required.(type) {
	case []string:
		for _, r := range v {
			result[r] = true
		}
	case []any:
		for _, r := range v {
			if rs, ok := r.(string); ok {
				result[rs] = true
			}
		}
	}
	return result
}

// jsonSchemaToFieldDef 从 JSON Schema 属性映射构建 FieldDef。
// required 参数来自父级的 "required" 列表。
func jsonSchemaToFieldDef(prop map[string]any, required bool) *FieldDef {
	fd := &FieldDef{
		Required: required,
	}
	if desc, ok := prop["description"].(string); ok {
		fd.Description = desc
	}
	typeStr, _ := prop["type"].(string)
	switch typeStr {
	case "string":
		fd.Type = TypeString
	case "integer":
		fd.Type = TypeInt
	case "number":
		fd.Type = TypeFloat
	case "boolean":
		fd.Type = TypeBool
	case "array":
		fd.Type = TypeList
		if items, ok := prop["items"].(map[string]any); ok {
			fd.ItemDef = jsonSchemaToFieldDef(items, false)
		}
	case "object":
		fd.Type = TypeDict
		if nested, ok := prop["properties"].(map[string]any); ok {
			fd.Children = make(map[string]*FieldDef)
			nestedRequired := collectRequiredSet(prop["required"])
			for n, p := range nested {
				if pm, ok := p.(map[string]any); ok {
					fd.Children[n] = jsonSchemaToFieldDef(pm, nestedRequired[n])
				}
			}
		}
	default:
		fd.Type = TypeString
	}
	return fd
}

// Ensure OutputSchema 满足 json.Marshaler 和 json.Unmarshaler 接口。
var (
	_ json.Marshaler   = (*OutputSchema)(nil)
	_ json.Unmarshaler = (*OutputSchema)(nil)
)

// SerializeValue 将任意值归一化后再交由调用方序列化。
// 主要解决 time.Time 时区信息丢失问题：
//   - time.Time → 调用 .UTC() 转 UTC 后返回
//   - *time.Time → 解引用后转 UTC（非 nil 时），nil 原样返回
//   - struct → 递归处理每个导出字段，遇到 time.Time/*time.Time 时转 UTC
//   - slice/array → 递归处理每个元素
//   - map → 递归处理每个 value（key 保持不变）
//   - 其他类型 → 原样返回
//
// 调用方应在 json.Marshal 之前调用本函数：
//
//	normalized := schema.SerializeValue(value)
//	data, err := json.Marshal(normalized)
func SerializeValue(value any) any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Invalid:
		return value
	}
	return normalizeValue(rv).Interface()
}

// normalizeValue 递归归一化 reflect.Value，返回新的 reflect.Value。
// 对 time.Time/*time.Time 调用 .UTC()；对容器类型递归处理子元素。
func normalizeValue(rv reflect.Value) reflect.Value {
	if !rv.IsValid() {
		return rv
	}

	// 解引用接口和指针以判断具体类型，但返回时需保持原指针语义
	switch rv.Kind() {
	case reflect.Interface:
		return normalizeValue(rv.Elem())
	case reflect.Pointer:
		if rv.IsNil() {
			return rv
		}
		// time.Time 指针
		if rv.Type() == reflect.TypeOf((*time.Time)(nil)) {
			t := rv.Elem().Interface().(time.Time).UTC()
			ptr := reflect.New(reflect.TypeOf(t))
			ptr.Elem().Set(reflect.ValueOf(t))
			return ptr
		}
		// 其他指针：解引用递归处理后再封装回指针
		normalized := normalizeValue(rv.Elem())
		// 如果元素类型未变，直接返回原指针（避免无谓分配）
		if normalized.Elem().Type() == rv.Elem().Type() {
			// 复制一份到新指针，避免修改原值
			ptr := reflect.New(rv.Elem().Type())
			ptr.Elem().Set(normalized.Elem())
			return ptr
		}
		// 类型变化（如 struct 内部字段被替换为 map），返回 normalized 本身
		return normalized
	}

	// 直接处理 time.Time
	if rv.Type() == reflect.TypeOf(time.Time{}) {
		t := rv.Interface().(time.Time).UTC()
		return reflect.ValueOf(t)
	}

	// 容器类型递归处理
	switch rv.Kind() {
	case reflect.Struct:
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.NumField(); i++ {
			field := rv.Field(i)
			if !field.CanInterface() {
				continue
			}
			normalized := normalizeValue(field)
			if normalized.IsValid() && out.Field(i).CanSet() {
				if normalized.Type() == out.Field(i).Type() {
					out.Field(i).Set(normalized)
				} else {
					// 类型变化（罕见），尽量赋值
					if normalized.Type().ConvertibleTo(out.Field(i).Type()) {
						out.Field(i).Set(normalized.Convert(out.Field(i).Type()))
					}
				}
			}
		}
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			normalized := normalizeValue(rv.Index(i))
			if normalized.IsValid() {
				out.Index(i).Set(normalized)
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.Len(); i++ {
			normalized := normalizeValue(rv.Index(i))
			if normalized.IsValid() && out.Index(i).CanSet() {
				out.Index(i).Set(normalized)
			}
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			normalized := normalizeValue(iter.Value())
			out.SetMapIndex(iter.Key(), normalized)
		}
		return out
	}

	// 其他类型原样返回
	return rv
}
