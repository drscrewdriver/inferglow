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

package model

// BuildJSONSchemaFromOutput converts OutputSchema to a standard JSON Schema map
// for use as response_format in OpenAI-compatible API requests.
// vLLM/SGLang XGrammar consumes this schema for token-level constrained decoding.
func BuildJSONSchemaFromOutput(os *OutputSchema) map[string]any {
	if os == nil {
		return map[string]any{"type": "object"}
	}
	properties := os.Properties
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(os.Required) > 0 {
		schema["required"] = os.Required
	}
	return schema
}

// forceJSONObject checks whether the caller explicitly requested json_object
// mode (degradation switch). When true, the provider sends {"type":"json_object"}
// instead of the full json_schema, and L3 prompt fallback is enabled.
func forceJSONObject(req *ModelRequest) bool {
	if req == nil || req.Options == nil {
		return false
	}
	mode, _ := req.Options["response_format_mode"].(string)
	return mode == "json_object"
}
