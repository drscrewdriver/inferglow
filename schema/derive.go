package schema

import (
	"reflect"
	"strings"
)

// DefineOutputFromType 通过 reflect 从 Go struct 类型推导 OutputSchema。
// 这是 DefineOutput[T] 的非泛型版本，便于在已有 reflect.Type 场景下复用。
func DefineOutputFromType(t reflect.Type) *OutputSchema {
	schema := &OutputSchema{
		Fields:    make(map[string]*FieldDef),
		EnsureAll: false,
	}
	deriveSchemaFromType(t, "", schema)
	return schema
}

// deriveSchemaFromType 递归推导 struct 类型为 OutputSchema。
// path 参数保留用于未来支持嵌套字段路径，当前实现仅写入顶层 Fields。
// 嵌套 struct 的字段会作为 FieldDef.Children 写入父字段的 Children map。
func deriveSchemaFromType(t reflect.Type, path string, schema *OutputSchema) {
	if t == nil {
		return
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		// 非 struct 类型（如 map[string]any）不推导字段
		return
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// 跳过非导出字段
		if !field.IsExported() {
			continue
		}
		jsonTag := parseJSONTag(field.Tag.Get("json"))
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		fieldDef := &FieldDef{
			Type:        goTypeToDataType(field.Type),
			Description: field.Tag.Get("description"),
			Required:    !strings.Contains(field.Tag.Get("json"), "omitempty"),
		}
		// 递归处理嵌套 struct（或 struct 指针），写入 Children
		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			fieldDef.Type = TypeDict
			fieldDef.Children = make(map[string]*FieldDef)
			deriveChildStruct(ft, fieldDef.Children)
		}
		// 处理 slice/array 的元素类型
		if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			elemType := ft.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				fieldDef.ItemDef = &FieldDef{
					Type:     TypeDict,
					Children: make(map[string]*FieldDef),
				}
				deriveChildStruct(elemType, fieldDef.ItemDef.Children)
			}
		}
		schema.Fields[jsonTag] = fieldDef
	}
}

// deriveChildStruct 递归推导嵌套 struct 的字段到目标 Children map。
func deriveChildStruct(t reflect.Type, children map[string]*FieldDef) {
	if t == nil || t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonTag := parseJSONTag(field.Tag.Get("json"))
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		fieldDef := &FieldDef{
			Type:        goTypeToDataType(field.Type),
			Description: field.Tag.Get("description"),
			Required:    !strings.Contains(field.Tag.Get("json"), "omitempty"),
		}
		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			fieldDef.Type = TypeDict
			fieldDef.Children = make(map[string]*FieldDef)
			deriveChildStruct(ft, fieldDef.Children)
		}
		children[jsonTag] = fieldDef
	}
}

// goTypeToDataType 将 Go 类型映射为 DataType。
// 数字类型区分 int/float，以保留语义用于 JSON Schema 生成。
func goTypeToDataType(t reflect.Type) DataType {
	if t == nil {
		return TypeString
	}
	// 处理指针类型：解引用后判断
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return TypeString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return TypeInt
	case reflect.Float32, reflect.Float64:
		return TypeFloat
	case reflect.Bool:
		return TypeBool
	case reflect.Slice, reflect.Array:
		return TypeList
	case reflect.Map:
		return TypeDict
	case reflect.Struct:
		return TypeDict
	case reflect.Interface:
		// any / interface{} → 视为通用 object
		return TypeDict
	default:
		return TypeString
	}
}

// parseJSONTag 解析 json tag，提取 field name（去掉 ,omitempty 等选项）。
// 例如 "name,omitempty" → "name"；"-" → "-"；"" → ""。
func parseJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	parts := strings.SplitN(tag, ",", 2)
	return parts[0]
}
