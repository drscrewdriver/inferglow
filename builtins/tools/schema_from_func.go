package tools

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// ToolDefinition describes a tool that an LLM can invoke. Its shape
// mirrors model.ToolDefinition so generated definitions can be used
// interchangeably with the model package's tool-call machinery.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// SchemaFromFunc builds a ToolDefinition from a Go function's signature.
//
// fn must be a function. The first parameter is skipped when it is a
// context.Context. The description may carry "@param name desc" tags to
// name and document individual parameters; parameters without a matching
// @param tag default to "arg0", "arg1", … in positional order.
func SchemaFromFunc(fn any, name string, description string) (*ToolDefinition, error) {
	if fn == nil {
		return nil, fmt.Errorf("tools: fn is nil")
	}
	t := reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("tools: expected func, got %s", t.Kind())
	}

	doc := ParseDocstring(description)
	paramEntries := parseParamEntries(description)

	tdDesc := doc.Description
	if tdDesc == "" {
		tdDesc = description
	}

	properties := make(map[string]any)
	var required []string

	paramIdx := 0
	for i := 0; i < t.NumIn(); i++ {
		pt := t.In(i)
		if i == 0 && pt.Implements(contextType) {
			continue
		}

		pname := fmt.Sprintf("arg%d", paramIdx)
		pdesc := ""
		if paramIdx < len(paramEntries) {
			pname = paramEntries[paramIdx].Name
			pdesc = paramEntries[paramIdx].Desc
		}

		properties[pname] = schemaForType(pt, pdesc)
		required = append(required, pname)
		paramIdx++
	}

	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}

	return &ToolDefinition{
		Name:        name,
		Description: tdDesc,
		Parameters:  parameters,
	}, nil
}

// schemaForType maps a reflect.Type to a JSON Schema fragment, attaching
// desc as the "description" field when non-empty.
func schemaForType(t reflect.Type, desc string) map[string]any {
	schema := jsonSchemaForType(t)
	if desc != "" {
		schema["description"] = desc
	}
	return schema
}

// jsonSchemaForType maps a reflect.Type to a JSON Schema fragment.
func jsonSchemaForType(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}
		}
		return map[string]any{
			"type":  "array",
			"items": jsonSchemaForType(t.Elem()),
		}
	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": jsonSchemaForType(t.Elem()),
		}
	case reflect.Struct:
		return structToSchema(t)
	case reflect.Interface:
		return map[string]any{}
	default:
		return map[string]any{"type": "object"}
	}
}

// structToSchema builds a JSON Schema object fragment for a struct type,
// deriving property names from `json` tags (falling back to field names).
func structToSchema(t reflect.Type) map[string]any {
	properties := make(map[string]any)
	var required []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("json")
		if name == "" {
			name = field.Name
		} else {
			if idx := strings.Index(name, ","); idx >= 0 {
				name = name[:idx]
			}
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
		}
		properties[name] = jsonSchemaForType(field.Type)
		required = append(required, name)
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
