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

// GenerateJSONSchema 将 OutputSchema 转换为 JSON Schema
func GenerateJSONSchema(schema *OutputSchema) map[string]any {
	js := map[string]any{
		"type":       "object",
		"properties": make(map[string]any),
	}

	required := make([]string, 0)

	for name, field := range schema.Fields {
		fieldSchema := fieldToJSONSchema(field)
		js["properties"].(map[string]any)[name] = fieldSchema

		if field.Required {
			required = append(required, name)
		}
	}

	if len(required) > 0 {
		js["required"] = required
	}

	return js
}

// fieldToJSONSchema 将 FieldDef 转换为 JSON Schema 对象
func fieldToJSONSchema(field *FieldDef) map[string]any {
	result := map[string]any{}

	switch field.Type {
	case TypeString:
		result["type"] = "string"
	case TypeInt:
		result["type"] = "integer"
	case TypeFloat:
		result["type"] = "number"
	case TypeBool:
		result["type"] = "boolean"
	case TypeDict:
		result["type"] = "object"
		if len(field.Children) > 0 {
			properties := make(map[string]any)
			for childName, childField := range field.Children {
				properties[childName] = fieldToJSONSchema(childField)
			}
			result["properties"] = properties

			// 优先使用 RequiredFields，否则从 Children 的 Required 字段推断
			var childRequired []string
			if len(field.RequiredFields) > 0 {
				childRequired = field.RequiredFields
			} else {
				for childName, childField := range field.Children {
					if childField.Required {
						childRequired = append(childRequired, childName)
					}
				}
			}
			if len(childRequired) > 0 {
				result["required"] = childRequired
			}
		}
	case TypeList:
		result["type"] = "array"
		if field.ItemDef != nil {
			result["items"] = fieldToJSONSchema(field.ItemDef)
		}
	case TypeModel:
		result["type"] = "object"
	case TypeOptional:
		result["type"] = "string"
	default:
		result["type"] = "string"
	}

	if field.Description != "" {
		result["description"] = field.Description
	}

	// oneOf：字段必须匹配其中一个子 schema
	if len(field.OneOf) > 0 {
		oneOf := make([]map[string]any, 0, len(field.OneOf))
		for _, sub := range field.OneOf {
			oneOf = append(oneOf, fieldToJSONSchema(sub))
		}
		result["oneOf"] = oneOf
	}

	// anyOf：字段至少匹配其中一个子 schema
	if len(field.AnyOf) > 0 {
		anyOf := make([]map[string]any, 0, len(field.AnyOf))
		for _, sub := range field.AnyOf {
			anyOf = append(anyOf, fieldToJSONSchema(sub))
		}
		result["anyOf"] = anyOf
	}

	return result
}
