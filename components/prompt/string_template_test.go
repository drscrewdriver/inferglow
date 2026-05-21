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
	"testing"
)

func TestStringTemplate_VariableSubstitution(t *testing.T) {
	tmpl := NewStringTemplate()
	if err := tmpl.AddMessage("user", "Hello {{.name}}"); err != nil {
		t.Fatalf("AddMessage error: %v", err)
	}
	msgs, err := tmpl.Format(context.Background(), map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello World" {
		t.Errorf("expected content %q, got %q", "Hello World", msgs[0].Content)
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected role %q, got %q", "user", msgs[0].Role)
	}
}

func TestStringTemplate_ConditionalRendering(t *testing.T) {
	tmpl := NewStringTemplate()
	if err := tmpl.AddMessage("user", "{{if .flag}}Enabled{{else}}Disabled{{end}}"); err != nil {
		t.Fatalf("AddMessage error: %v", err)
	}

	msgs, err := tmpl.Format(context.Background(), map[string]any{"flag": true})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if msgs[0].Content != "Enabled" {
		t.Errorf("expected %q, got %q", "Enabled", msgs[0].Content)
	}

	msgs, err = tmpl.Format(context.Background(), map[string]any{"flag": false})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if msgs[0].Content != "Disabled" {
		t.Errorf("expected %q, got %q", "Disabled", msgs[0].Content)
	}
}

func TestStringTemplate_MultiMessage(t *testing.T) {
	tmpl := NewStringTemplate()
	if err := tmpl.AddMessage("system", "You are a helpful assistant."); err != nil {
		t.Fatalf("AddMessage system error: %v", err)
	}
	if err := tmpl.AddMessage("user", "Hello {{.name}}!"); err != nil {
		t.Fatalf("AddMessage user error: %v", err)
	}

	msgs, err := tmpl.Format(context.Background(), map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("message 0 role: got %q want %q", msgs[0].Role, "system")
	}
	if msgs[0].Content != "You are a helpful assistant." {
		t.Errorf("message 0 content: got %q want %q", msgs[0].Content, "You are a helpful assistant.")
	}
	if msgs[1].Role != "user" {
		t.Errorf("message 1 role: got %q want %q", msgs[1].Role, "user")
	}
	if msgs[1].Content != "Hello Alice!" {
		t.Errorf("message 1 content: got %q want %q", msgs[1].Content, "Hello Alice!")
	}
}

func TestStringTemplate_WithSystemMessage(t *testing.T) {
	tmpl := NewStringTemplate(
		WithSystemMessage("System: {{.mode}}"),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{"mode": "debug"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected role %q, got %q", "system", msgs[0].Role)
	}
	if msgs[0].Content != "System: debug" {
		t.Errorf("expected content %q, got %q", "System: debug", msgs[0].Content)
	}
}

func TestStringTemplate_WithUserMessage(t *testing.T) {
	tmpl := NewStringTemplate(
		WithUserMessage("Hi {{.name}}"),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{"name": "Bob"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected role %q, got %q", "user", msgs[0].Role)
	}
	if msgs[0].Content != "Hi Bob" {
		t.Errorf("expected content %q, got %q", "Hi Bob", msgs[0].Content)
	}
}

func TestStringTemplate_WithAssistantMessage(t *testing.T) {
	tmpl := NewStringTemplate(
		WithAssistantMessage("I can help with {{.topic}}"),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{"topic": "coding"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("expected role %q, got %q", "assistant", msgs[0].Role)
	}
	if msgs[0].Content != "I can help with coding" {
		t.Errorf("expected content %q, got %q", "I can help with coding", msgs[0].Content)
	}
}

func TestStringTemplate_WithSystemAndUserMessage(t *testing.T) {
	tmpl := NewStringTemplate(
		WithSystemMessage("You are {{.role}}"),
		WithUserMessage("Question: {{.query}}"),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{
		"role":  "an expert",
		"query": "What is Go?",
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "You are an expert" {
		t.Errorf("system message: got role=%q content=%q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "Question: What is Go?" {
		t.Errorf("user message: got role=%q content=%q", msgs[1].Role, msgs[1].Content)
	}
}

func TestStringTemplate_NoVars(t *testing.T) {
	tmpl := NewStringTemplate(
		WithUserMessage("static content with no variables"),
	)
	msgs, err := tmpl.Format(context.Background(), nil)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if msgs[0].Content != "static content with no variables" {
		t.Errorf("expected %q, got %q", "static content with no variables", msgs[0].Content)
	}
}

func TestStringTemplate_MultipleVariables(t *testing.T) {
	tmpl := NewStringTemplate(
		WithUserMessage("{{.greeting}}, {{.name}}! You have {{.count}} messages."),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{
		"greeting": "Hi",
		"name":     "Alice",
		"count":    3,
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	want := "Hi, Alice! You have 3 messages."
	if msgs[0].Content != want {
		t.Errorf("expected %q, got %q", want, msgs[0].Content)
	}
}

func TestStringTemplate_NestedData(t *testing.T) {
	tmpl := NewStringTemplate(
		WithUserMessage("Hello {{.user.name}}, age {{.user.age}}"),
	)
	msgs, err := tmpl.Format(context.Background(), map[string]any{
		"user": map[string]any{
			"name": "Bob",
			"age":  42,
		},
	})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	want := "Hello Bob, age 42"
	if msgs[0].Content != want {
		t.Errorf("expected %q, got %q", want, msgs[0].Content)
	}
}

func TestStringTemplate_ParseError(t *testing.T) {
	tmpl := NewStringTemplate()
	if err := tmpl.AddMessage("user", "{{ .name }"); err == nil {
		t.Fatal("expected error for invalid template syntax, got nil")
	}
}

func TestStringTemplate_OptionParseError(t *testing.T) {
	// A malformed template in an Option should surface as an error from Format.
	tmpl := NewStringTemplate(
		WithUserMessage("{{ .name }"), // malformed: missing closing brace
	)
	_, err := tmpl.Format(context.Background(), map[string]any{"name": "World"})
	if err == nil {
		t.Fatal("expected error for invalid template syntax, got nil")
	}
}

func TestStringTemplate_EmptyTemplate(t *testing.T) {
	tmpl := NewStringTemplate()
	msgs, err := tmpl.Format(context.Background(), nil)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}
