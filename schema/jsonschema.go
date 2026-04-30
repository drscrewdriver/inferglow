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

	return result
}
