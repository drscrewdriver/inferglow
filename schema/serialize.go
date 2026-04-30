package schema

import (
	"encoding/json"
	"fmt"
)

// fieldDefDTO 是 FieldDef 的可序列化表示。
// 用于 MarshalJSON/UnmarshalJSON，将 DataType 转为字符串。
type fieldDefDTO struct {
	Type          DataType             `json:"type"`
	Description   string               `json:"description,omitempty"`
	Ensure        EnsurePolicy         `json:"ensure,omitempty"`
	Required      bool                 `json:"required,omitempty"`
	RequiredFields []string            `json:"required_fields,omitempty"`
	Children      map[string]*FieldDef `json:"children,omitempty"`
	ItemDef       *FieldDef            `json:"item_def,omitempty"`
}

// outputSchemaDTO 是 OutputSchema 的可序列化表示。
type outputSchemaDTO struct {
	Format    OutputFormat          `json:"format,omitempty"`
	EnsureAll bool                  `json:"ensure_all,omitempty"`
	Fields    map[string]*FieldDef  `json:"fields"`
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
