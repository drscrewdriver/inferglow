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

package prompt

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/inferglow/model"
)

// ConditionalSection is a named section of a system prompt template that
// is only included when its condition evaluates to true.
type ConditionalSection struct {
	// Name is the variable name used in the condition check.
	Name string
	// Template is the Go text/template content for this section.
	Template string
	// parsed is the compiled template (populated during construction).
	parsed *template.Template
}

// SystemTemplate builds a system prompt from a base template and optional
// conditional sections. Variables in the base template and conditional
// sections are rendered using Go text/template syntax.
//
// Conditional sections are included only when their named variable is
// truthy in the vars map (non-empty string, non-zero number, non-nil,
// or true boolean).
//
// Example:
//
//	st := NewSystemTemplate("You are a {{.role}} assistant.")
//	st.AddConditionalSection("tools", "You have access to tools: {{.toolList}}")
//	msgs, _ := st.Format(ctx, map[string]any{
//	    "role": "helpful",
//	    "tools": true,
//	    "toolList": "search, calculator",
//	})
type SystemTemplate struct {
	baseTemplate string
	parsed       *template.Template
	sections     []*ConditionalSection
	parseErr     error
}

// NewSystemTemplate creates a SystemTemplate with the given base template.
// The template uses Go text/template syntax for variable substitution.
func NewSystemTemplate(baseTemplate string) *SystemTemplate {
	t := &SystemTemplate{
		baseTemplate: baseTemplate,
	}
	parsed, err := template.New("system_base").Parse(baseTemplate)
	if err != nil {
		t.parseErr = fmt.Errorf("system_template: parse base: %w", err)
		return t
	}
	t.parsed = parsed
	return t
}

// AddConditionalSection adds a section that is only included when the
// named variable is truthy in the vars map. The section template uses
// Go text/template syntax.
func (t *SystemTemplate) AddConditionalSection(name, sectionTemplate string) error {
	parsed, err := template.New("section_" + name).Parse(sectionTemplate)
	if err != nil {
		return fmt.Errorf("system_template: parse section %q: %w", name, err)
	}
	t.sections = append(t.sections, &ConditionalSection{
		Name:     name,
		Template: sectionTemplate,
		parsed:   parsed,
	})
	return nil
}

// Format renders the system template with the given variables. Returns a
// single ChatMessage with role "system" containing the rendered prompt.
func (t *SystemTemplate) Format(ctx context.Context, vars map[string]any) ([]model.ChatMessage, error) {
	if t.parseErr != nil {
		return nil, t.parseErr
	}

	// Render base template.
	var baseBuf strings.Builder
	if err := t.parsed.Execute(&baseBuf, vars); err != nil {
		return nil, fmt.Errorf("system_template: execute base: %w", err)
	}

	var parts []string
	parts = append(parts, baseBuf.String())

	// Render conditional sections.
	for _, section := range t.sections {
		if isTruthy(vars, section.Name) {
			var secBuf strings.Builder
			if err := section.parsed.Execute(&secBuf, vars); err != nil {
				return nil, fmt.Errorf("system_template: execute section %q: %w", section.Name, err)
			}
			parts = append(parts, secBuf.String())
		}
	}

	content := strings.Join(parts, "\n\n")
	return []model.ChatMessage{
		{
			Role:    string(model.RoleSystem),
			Content: content,
		},
	}, nil
}

// isTruthy checks if a variable in the map is truthy.
func isTruthy(vars map[string]any, name string) bool {
	v, ok := vars[name]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case int:
		return val != 0
	case float64:
		return val != 0
	case nil:
		return false
	default:
		return true
	}
}
