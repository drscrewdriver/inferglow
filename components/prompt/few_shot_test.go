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

	"github.com/inferglow/model"
)

// Compile-time interface check.
var _ ChatTemplate = (*FewShotTemplate)(nil)

func TestFewShotTemplate_Format(t *testing.T) {
	tmpl := NewFewShotTemplate("You are a translator.", []Example{
		{Input: "Hello", Output: "Bonjour"},
		{Input: "Goodbye", Output: "Au revoir"},
	})

	msgs, err := tmpl.Format(context.Background(), map[string]any{"input": "Thanks"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	// Expected: system + 2×(user+assistant) + user = 6 messages.
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgs))
	}

	// Check system message.
	if msgs[0].Role != string(model.RoleSystem) {
		t.Errorf("msgs[0].Role = %q, want 'system'", msgs[0].Role)
	}
	if msgs[0].Content != "You are a translator." {
		t.Errorf("msgs[0].Content = %q, want 'You are a translator.'", msgs[0].Content)
	}

	// Check first example pair.
	if msgs[1].Role != string(model.RoleUser) || msgs[1].Content != "Hello" {
		t.Errorf("msgs[1] = {%q, %q}, want {user, Hello}", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != string(model.RoleAssistant) || msgs[2].Content != "Bonjour" {
		t.Errorf("msgs[2] = {%q, %q}, want {assistant, Bonjour}", msgs[2].Role, msgs[2].Content)
	}

	// Check second example pair.
	if msgs[3].Role != string(model.RoleUser) || msgs[3].Content != "Goodbye" {
		t.Errorf("msgs[3] = {%q, %q}, want {user, Goodbye}", msgs[3].Role, msgs[3].Content)
	}
	if msgs[4].Role != string(model.RoleAssistant) || msgs[4].Content != "Au revoir" {
		t.Errorf("msgs[4] = {%q, %q}, want {assistant, Au revoir}", msgs[4].Role, msgs[4].Content)
	}

	// Check actual user input.
	if msgs[5].Role != string(model.RoleUser) || msgs[5].Content != "Thanks" {
		t.Errorf("msgs[5] = {%q, %q}, want {user, Thanks}", msgs[5].Role, msgs[5].Content)
	}
}

func TestFewShotTemplate_MissingInput(t *testing.T) {
	tmpl := NewFewShotTemplate("system", []Example{{Input: "a", Output: "b"}})

	_, err := tmpl.Format(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing 'input' variable")
	}
}

func TestFewShotTemplate_NoExamples(t *testing.T) {
	tmpl := NewFewShotTemplate("You are helpful.", nil)

	msgs, err := tmpl.Format(context.Background(), map[string]any{"input": "hi"})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	// system + user = 2 messages.
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestFewShotTemplate_NonStringInput(t *testing.T) {
	tmpl := NewFewShotTemplate("sys", nil)

	msgs, err := tmpl.Format(context.Background(), map[string]any{"input": 42})
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if msgs[1].Content != "42" {
		t.Errorf("expected content '42', got %q", msgs[1].Content)
	}
}
