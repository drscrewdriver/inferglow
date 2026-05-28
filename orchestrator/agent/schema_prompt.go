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

package agent

import (
	"fmt"
	"strings"

	"github.com/inferglow/model"
)

// shouldInjectSchemaPrompt decides whether L3 prompt fallback is needed.
// Returns true only when the provider cannot enforce json_schema-level
// response_format (either explicitly forced to json_object mode, or no
// response_format capability at all).
func shouldInjectSchemaPrompt(req *model.ModelRequest) bool {
	if req == nil || req.Output == nil || len(req.Output.Properties) == 0 {
		return false
	}
	// If json_schema-level response_format is already set, L1/L2 is active → skip L3
	if req.Options != nil {
		if rf, ok := req.Options["response_format"].(map[string]any); ok {
			if rf["type"] == "json_schema" {
				return false
			}
		}
		// Explicitly forced to json_object mode → need L3
		if mode, _ := req.Options["response_format_mode"].(string); mode == "json_object" {
			return true
		}
	}
	// No response_format capability (text provider) → need L3
	return req.OutputFormat == "" || req.OutputFormat == "text"
}

// formatSchemaInstruction generates an L3 fallback schema description
// to inject at the end of the system prompt.
func formatSchemaInstruction(s *model.OutputSchema) string {
	if s == nil || len(s.Properties) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n[输出格式要求]\n你必须严格以如下 JSON 格式输出，不要添加任何额外文本：\n```json\n{\n")
	for name, prop := range s.Properties {
		sb.WriteString(fmt.Sprintf("  \"%s\": ", name))
		if propMap, ok := prop.(map[string]any); ok {
			if desc, ok := propMap["description"].(string); ok && desc != "" {
				sb.WriteString(fmt.Sprintf("\"<%s>\"", desc))
			} else if t, ok := propMap["type"].(string); ok {
				sb.WriteString(fmt.Sprintf("\"<%s>\"", t))
			} else {
				sb.WriteString("\"<value>\"")
			}
		} else {
			sb.WriteString("\"<value>\"")
		}
		sb.WriteString(",\n")
	}
	sb.WriteString("}\n```\n")
	if len(s.Required) > 0 {
		sb.WriteString(fmt.Sprintf("必须包含的字段：%s\n", strings.Join(s.Required, ", ")))
	}
	return sb.String()
}
