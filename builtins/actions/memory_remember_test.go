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

package actions

import (
	"context"
	"testing"
)

func TestRememberExecutor_NormalSave(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRememberAction(MemoryRememberConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name":        "user-prefers-go",
		"description": "user prefers Go language",
		"body":        "the user prefers Go for backend services",
		"type":        "user",
		"scope":       "project",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "saved" {
		t.Errorf("Status = %q, want %q", res.Status, "saved")
	}

	resultMap, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result is not map[string]any, got %T", res.Result)
	}

	path, _ := resultMap["path"].(string)
	if path == "" {
		t.Error("expected non-empty path in result")
	}

	name, _ := resultMap["name"].(string)
	if name != "user-prefers-go" {
		t.Errorf("name = %q, want %q", name, "user-prefers-go")
	}

	id, _ := resultMap["id"].(string)
	if id == "" {
		t.Error("expected non-empty id in result")
	}

	rev, _ := resultMap["revision"].(int)
	if rev != 1 {
		t.Errorf("revision = %d, want 1", rev)
	}

	// Verify the memory is actually stored.
	if _, ok := store.Load("user-prefers-go"); !ok {
		t.Error("memory 'user-prefers-go' should be loadable after save")
	}
}

func TestRememberExecutor_EmptyDescription(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRememberAction(MemoryRememberConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"description": "",
		"body":        "some body content",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for empty description")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error != "remember: description and body are required" {
		t.Errorf("Error = %q, want %q", res.Error, "remember: description and body are required")
	}
}

func TestRememberExecutor_EmptyBody(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRememberAction(MemoryRememberConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"description": "some description",
		"body":        "",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for empty body")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error != "remember: description and body are required" {
		t.Errorf("Error = %q, want %q", res.Error, "remember: description and body are required")
	}
}

func TestRememberExecutor_NormalSaveWithName(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRememberAction(MemoryRememberConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name":        "my-custom-name",
		"title":       "My Custom Title",
		"description": "a custom named memory",
		"body":        "this memory has an explicit name",
		"type":        "reference",
		"scope":       "global",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "saved" {
		t.Errorf("Status = %q, want %q", res.Status, "saved")
	}

	resultMap, _ := res.Result.(map[string]any)
	name, _ := resultMap["name"].(string)
	if name != "my-custom-name" {
		t.Errorf("name = %q, want %q", name, "my-custom-name")
	}

	// Verify the memory uses the provided name.
	m, ok := store.Load("my-custom-name")
	if !ok {
		t.Fatal("memory 'my-custom-name' should be loadable")
	}
	if m.Scope != "global" {
		t.Errorf("Scope = %q, want %q", m.Scope, "global")
	}
	if m.Type != "reference" {
		t.Errorf("Type = %q, want %q", m.Type, "reference")
	}
}