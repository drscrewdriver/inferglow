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
	"fmt"
	"reflect"
)

// TypeAdapter 类型适配器接口。
// 用于在 Go 值和 JSON Schema 类型之间做双向映射和校验。
type TypeAdapter interface {
	// Adapt 将 Go 值验证/转换到目标类型。
	// 若值已是目标类型，原样返回；否则尝试转换或返回 error。
	Adapt(value any) (any, error)
	// Validate 检查值是否符合类型约束（含子元素约束）。
	// 与 Adapt 的区别：Adapt 返回转换后的值，Validate 仅做校验。
	// 默认实现可委托给 Adapt；带子约束（Items/Properties）的适配器
	// 会递归校验子元素。
	Validate(value any) error
}

// ============================================================================
// StringTypeAdapter
// ============================================================================

// StringTypeAdapter 字符串类型适配器。
type StringTypeAdapter struct{}

// Adapt 验证 value 为 string。若 value 不是 string 返回 error。
// 支持底层类型为 string 的命名类型（如 type MyString string）。
func (a *StringTypeAdapter) Adapt(value any) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("string adapter is nil")
	}
	if s, ok := value.(string); ok {
		return s, nil
	}
	// 处理底层类型为 string 的命名类型（type MyString string 等）
	if value != nil {
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			// 解引用指针
			for rv.Kind() == reflect.Pointer {
				if rv.IsNil() {
					return nil, fmt.Errorf("expected string, got nil pointer")
				}
				rv = rv.Elem()
			}
			if rv.Kind() == reflect.String && rv.Type() != reflect.TypeOf("") {
				return rv.String(), nil
			}
		}
	}
	return nil, fmt.Errorf("expected string, got %T", value)
}

// Validate 验证 value 为 string。
func (a *StringTypeAdapter) Validate(value any) error {
	_, err := a.Adapt(value)
	return err
}

// ============================================================================
// NumberTypeAdapter
// ============================================================================

// NumberTypeAdapter 数字类型适配器。
// 支持 Go 的所有数值类型：int/int8/int16/int32/int64/uint*/float32/float64。
// Adapt 会将所有数值类型归一化为 float64（JSON Schema number 的标准表示）。
type NumberTypeAdapter struct{}

// Adapt 验证 value 为数字类型，并归一化为 float64。
// 支持底层类型为数值的命名类型（如 type MyInt int）。
func (a *NumberTypeAdapter) Adapt(value any) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("number adapter is nil")
	}
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	}
	// 处理底层类型为数值的命名类型
	if value != nil {
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			for rv.Kind() == reflect.Pointer {
				if rv.IsNil() {
					return nil, fmt.Errorf("expected number, got nil pointer")
				}
				rv = rv.Elem()
			}
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return float64(rv.Int()), nil
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return float64(rv.Uint()), nil
			case reflect.Float32, reflect.Float64:
				return rv.Float(), nil
			}
		}
	}
	return nil, fmt.Errorf("expected number, got %T", value)
}

// Validate 验证 value 为数字类型。
func (a *NumberTypeAdapter) Validate(value any) error {
	_, err := a.Adapt(value)
	return err
}

// ============================================================================
// BooleanTypeAdapter
// ============================================================================

// BooleanTypeAdapter 布尔类型适配器。
type BooleanTypeAdapter struct{}

// Adapt 验证 value 为 bool。
// 支持底层类型为 bool 的命名类型（如 type MyBool bool）。
func (a *BooleanTypeAdapter) Adapt(value any) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("boolean adapter is nil")
	}
	if b, ok := value.(bool); ok {
		return b, nil
	}
	// 处理底层类型为 bool 的命名类型
	if value != nil {
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			for rv.Kind() == reflect.Pointer {
				if rv.IsNil() {
					return nil, fmt.Errorf("expected bool, got nil pointer")
				}
				rv = rv.Elem()
			}
			if rv.Kind() == reflect.Bool && rv.Type() != reflect.TypeOf(false) {
				return rv.Bool(), nil
			}
		}
	}
	return nil, fmt.Errorf("expected bool, got %T", value)
}

// Validate 验证 value 为 bool。
func (a *BooleanTypeAdapter) Validate(value any) error {
	_, err := a.Adapt(value)
	return err
}

// ============================================================================
// ArrayTypeAdapter
// ============================================================================

// ArrayTypeAdapter 数组类型适配器。
// 支持所有 Go slice/array 类型。
// 若 Items 非 nil，Validate 会递归校验每个元素。
type ArrayTypeAdapter struct {
	Items TypeAdapter // 可选：元素类型约束
}

// Adapt 验证 value 为 slice/array。
// 若 Items 非 nil，会递归 Adapt 每个元素，返回转换后的 []any。
// 若 Items 为 nil，原样返回 value。
func (a *ArrayTypeAdapter) Adapt(value any) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("array adapter is nil")
	}
	if value == nil {
		return nil, fmt.Errorf("expected array, got nil")
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("expected array, got %T", value)
	}
	// 无元素约束，原样返回
	if a.Items == nil {
		return value, nil
	}
	// 有元素约束，递归 Adapt
	length := rv.Len()
	out := make([]any, length)
	for i := 0; i < length; i++ {
		adapted, err := a.Items.Adapt(rv.Index(i).Interface())
		if err != nil {
			return nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		out[i] = adapted
	}
	return out, nil
}

// Validate 验证 value 为 slice/array，若 Items 设置则递归校验元素。
func (a *ArrayTypeAdapter) Validate(value any) error {
	if a == nil {
		return fmt.Errorf("array adapter is nil")
	}
	if value == nil {
		return fmt.Errorf("expected array, got nil")
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return fmt.Errorf("expected array, got %T", value)
	}
	if a.Items != nil {
		length := rv.Len()
		for i := 0; i < length; i++ {
			if err := a.Items.Validate(rv.Index(i).Interface()); err != nil {
				return fmt.Errorf("items[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// ============================================================================
// ObjectTypeAdapter
// ============================================================================

// ObjectTypeAdapter 对象类型适配器。
// 支持所有 Go map（key 为 string）和 struct 类型。
// 若 Properties 非 nil，Validate 会按 Properties 中的 key 递归校验对应字段。
type ObjectTypeAdapter struct {
	Properties map[string]TypeAdapter // 可选：属性类型约束
}

// Adapt 验证 value 为 map[string]any 或 struct。
// 若 Properties 非 nil，会按 key 递归 Adapt 对应字段，返回转换后的 map[string]any。
// 若 Properties 为 nil，原样返回 value。
func (a *ObjectTypeAdapter) Adapt(value any) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("object adapter is nil")
	}
	if value == nil {
		return nil, fmt.Errorf("expected object, got nil")
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Map:
		// 必须是 map[string]XXX
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("expected map with string key, got %T", value)
		}
		// 无属性约束，原样返回
		if a.Properties == nil {
			return value, nil
		}
		// 有属性约束，按 key 递归 Adapt
		out := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			k := key.String()
			v := rv.MapIndex(key).Interface()
			if adapter, ok := a.Properties[k]; ok {
				adapted, err := adapter.Adapt(v)
				if err != nil {
					return nil, fmt.Errorf("properties[%s]: %w", k, err)
				}
				out[k] = adapted
			} else {
				out[k] = v
			}
		}
		return out, nil
	case reflect.Struct:
		// struct 转 map[string]any 后再校验
		m := structToMap(rv)
		// 复用 map 分支逻辑
		return a.Adapt(m)
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, fmt.Errorf("expected object, got nil pointer")
		}
		return a.Adapt(rv.Elem().Interface())
	}
	return nil, fmt.Errorf("expected object, got %T", value)
}

// Validate 验证 value 为 map 或 struct，若 Properties 设置则递归校验。
func (a *ObjectTypeAdapter) Validate(value any) error {
	if a == nil {
		return fmt.Errorf("object adapter is nil")
	}
	if value == nil {
		return fmt.Errorf("expected object, got nil")
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("expected map with string key, got %T", value)
		}
		if a.Properties != nil {
			for _, key := range rv.MapKeys() {
				k := key.String()
				if adapter, ok := a.Properties[k]; ok {
					if err := adapter.Validate(rv.MapIndex(key).Interface()); err != nil {
						return fmt.Errorf("properties[%s]: %w", k, err)
					}
				}
			}
		}
		return nil
	case reflect.Struct:
		m := structToMap(rv)
		return a.Validate(m)
	case reflect.Pointer:
		if rv.IsNil() {
			return fmt.Errorf("expected object, got nil pointer")
		}
		return a.Validate(rv.Elem().Interface())
	}
	return fmt.Errorf("expected object, got %T", value)
}

// structToMap 将 struct 转换为 map[string]any（使用 json tag 作为 key）。
func structToMap(rv reflect.Value) map[string]any {
	out := make(map[string]any)
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// 跳过未导出字段
		if !field.IsExported() {
			continue
		}
		// 解析 json tag
		name := field.Tag.Get("json")
		if name == "" || name == "-" {
			// 无 json tag 使用字段名
			name = field.Name
		} else {
			// 取逗号前的部分作为 name
			for j, c := range name {
				if c == ',' {
					name = name[:j]
					break
				}
			}
		}
		out[name] = rv.Field(i).Interface()
	}
	return out
}

// ============================================================================
// 工厂函数
// ============================================================================

// NewTypeAdapter 根据 DataType 创建对应的 TypeAdapter。
// 用于从 FieldDef.Type 构造适配器。
func NewTypeAdapter(t DataType) TypeAdapter {
	switch t {
	case TypeString:
		return &StringTypeAdapter{}
	case TypeInt, TypeFloat:
		return &NumberTypeAdapter{}
	case TypeBool:
		return &BooleanTypeAdapter{}
	case TypeList:
		return &ArrayTypeAdapter{}
	case TypeDict, TypeModel:
		return &ObjectTypeAdapter{}
	case TypeOptional:
		// Optional 默认为 string
		return &StringTypeAdapter{}
	}
	return nil
}

// NewTypeAdapterFromFieldDef 从 FieldDef 构造 TypeAdapter。
// 递归处理 Children/ItemDef。
func NewTypeAdapterFromFieldDef(fd *FieldDef) TypeAdapter {
	if fd == nil {
		return nil
	}
	switch fd.Type {
	case TypeString:
		return &StringTypeAdapter{}
	case TypeInt, TypeFloat:
		return &NumberTypeAdapter{}
	case TypeBool:
		return &BooleanTypeAdapter{}
	case TypeList:
		var items TypeAdapter
		if fd.ItemDef != nil {
			items = NewTypeAdapterFromFieldDef(fd.ItemDef)
		}
		return &ArrayTypeAdapter{Items: items}
	case TypeDict, TypeModel:
		var props map[string]TypeAdapter
		if len(fd.Children) > 0 {
			props = make(map[string]TypeAdapter, len(fd.Children))
			for k, child := range fd.Children {
				props[k] = NewTypeAdapterFromFieldDef(child)
			}
		}
		return &ObjectTypeAdapter{Properties: props}
	case TypeOptional:
		return &StringTypeAdapter{}
	}
	return nil
}
