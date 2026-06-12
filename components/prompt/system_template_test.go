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
	"strings"
	"testing"

	"github.com/inferglow/model"
)

// Compile-time interface check.
var _ ChatTemplate = (*SystemTemplate)(nil)

func TestSystemTemplate_BasicFormat(t *testing.T) {
	st := NewSystemTemplate("You are a {{.role}} assistant.")

	msgs, err := st.Format(context.Background(), map[string]any{
		"role": "helpful",
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != string(model.RoleSystem) {
		t.Errorf("expected role 'system', got %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "helpful") {
		t.Errorf("expected content to contain 'helpful', got %q", msgs[0].Content)
	}
}

func TestSystemTemplate_ConditionalSection_Included(t *testing.T) {
	st := NewSystemTemplate("Base prompt.")
	err := st.AddConditionalSection("tools", "You have tools: {{.toolList}}.")
	if err != nil {
		t.Fatalf("AddConditionalSection returned error: %v", err)
	}

	msgs, err := st.Format(context.Background(), map[string]any{
		"tools":    true,
		"toolList": "search, calc",
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	content := msgs[0].Content
	if !strings.Contains(content, "Base prompt.") {
		t.Errorf("expected base prompt, got %q", content)
	}
	if !strings.Contains(content, "You have tools: search, calc.") {
		t.Errorf("expected tools section, got %q", content)
	}
}

func TestSystemTemplate_ConditionalSection_Excluded(t *testing.T) {
	st := NewSystemTemplate("Base prompt.")
	_ = st.AddConditionalSection("tools", "You have tools.")

	msgs, err := st.Format(context.Background(), map[string]any{
		"tools": false,
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	content := msgs[0].Content
	if strings.Contains(content, "You have tools") {
		t.Errorf("conditional section should be excluded when tools=false, got %q", content)
	}
}

func TestSystemTemplate_ConditionalSection_MissingVar(t *testing.T) {
	st := NewSystemTemplate("Base.")
	_ = st.AddConditionalSection("tools", "Tools section.")

	msgs, err := st.Format(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if strings.Contains(msgs[0].Content, "Tools section") {
		t.Errorf("section should be excluded when var is missing, got %q", msgs[0].Content)
	}
}

func TestSystemTemplate_IsTruthy(t *testing.T) {
	tests := []struct {
		name   string
		vars   map[string]any
		key    string
		expect bool
	}{
		{"true_bool", map[string]any{"k": true}, "k", true},
		{"false_bool", map[string]any{"k": false}, "k", false},
		{"nonempty_string", map[string]any{"k": "yes"}, "k", true},
		{"empty_string", map[string]any{"k": ""}, "k", false},
		{"nonzero_int", map[string]any{"k": 1}, "k", true},
		{"zero_int", map[string]any{"k": 0}, "k", false},
		{"nil_val", map[string]any{"k": nil}, "k", false},
		{"missing_key", map[string]any{}, "k", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTruthy(tt.vars, tt.key)
			if got != tt.expect {
				t.Errorf("isTruthy(%v, %q) = %v, want %v", tt.vars, tt.key, got, tt.expect)
			}
		})
	}
}

func TestSystemTemplate_InvalidBaseTemplate(t *testing.T) {
	st := NewSystemTemplate("{{.invalid")
	_, err := st.Format(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for invalid base template")
	}
}

func TestSystemTemplate_MultipleConditionalSections(t *testing.T) {
	st := NewSystemTemplate("Base.")
	_ = st.AddConditionalSection("a", "Section A.")
	_ = st.AddConditionalSection("b", "Section B.")

	msgs, err := st.Format(context.Background(), map[string]any{
		"a": true,
		"b": false,
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	content := msgs[0].Content
	if !strings.Contains(content, "Section A.") {
		t.Errorf("expected Section A, got %q", content)
	}
	if strings.Contains(content, "Section B.") {
		t.Errorf("Section B should be excluded, got %q", content)
	}
}
