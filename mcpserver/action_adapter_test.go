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

package mcpserver

import (
	"context"
	"testing"

	"github.com/inferglow/action"
)

type mockExecutor struct {
	result *action.ActionResult
	err    error
}

func (m *mockExecutor) Execute(_ context.Context, _ map[string]any) (*action.ActionResult, error) {
	return m.result, m.err
}

func TestActionRegistryAdapterListTools(t *testing.T) {
	reg := action.NewRegistry()
	reg.Register(&action.Action{
		Name:        "greet",
		Description: "Greets someone",
		Schema:      map[string]any{"type": "object"},
		Executor:    &mockExecutor{result: &action.ActionResult{OK: true, Result: "hello"}},
	})

	adapter := NewActionRegistryAdapter(reg)
	tools := adapter.ListTools()

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "greet" {
		t.Errorf("expected tool name 'greet', got %q", tools[0].Name)
	}
	if tools[0].Description != "Greets someone" {
		t.Errorf("expected description 'Greets someone', got %q", tools[0].Description)
	}
}

func TestActionRegistryAdapterCallTool(t *testing.T) {
	reg := action.NewRegistry()
	reg.Register(&action.Action{
		Name:        "echo",
		Description: "Echoes input",
		Schema:      map[string]any{"type": "object"},
		Executor: &mockExecutor{
			result: &action.ActionResult{OK: true, Result: "hello world"},
		},
	})

	adapter := NewActionRegistryAdapter(reg)
	result, err := adapter.CallTool(context.Background(), "echo", map[string]any{"msg": "hello world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected non-error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Content[0].Text)
	}
}

func TestActionRegistryAdapterCallToolError(t *testing.T) {
	reg := action.NewRegistry()
	reg.Register(&action.Action{
		Name:        "fail",
		Description: "Always fails",
		Schema:      map[string]any{"type": "object"},
		Executor: &mockExecutor{
			result: &action.ActionResult{OK: false, Error: "something went wrong"},
		},
	})

	adapter := NewActionRegistryAdapter(reg)
	result, err := adapter.CallTool(context.Background(), "fail", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if result.Content[0].Text != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %q", result.Content[0].Text)
	}
}

func TestActionRegistryAdapterCallToolNotFound(t *testing.T) {
	reg := action.NewRegistry()
	adapter := NewActionRegistryAdapter(reg)

	_, err := adapter.CallTool(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}
