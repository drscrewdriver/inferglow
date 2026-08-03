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
	"strings"
	"testing"

	"github.com/inferglow/memory"
)

// sampleMemories returns a set of memories used across recall tests.
func sampleMemories() []memory.Memory {
	return []memory.Memory{
		{
			Name:        "user-prefs",
			Title:       "User Preferences",
			Description: "project preference about Go language",
			Type:        memory.TypeUser,
			Scope:       memory.FactScopeProject,
			Body:        "the user prefers Go language for backend services",
		},
		{
			Name:        "feedback-style",
			Title:       "Feedback Style",
			Description: "feedback on code review style",
			Type:        memory.TypeFeedback,
			Scope:       memory.FactScopeProject,
			Body:        "use conventional commits and prefer short functions",
		},
		{
			Name:        "global-ref",
			Title:       "Global Reference",
			Description: "reference to external API docs",
			Type:        memory.TypeReference,
			Scope:       memory.FactScopeGlobal,
			Body:        "see https://example.com/api for details",
		},
	}
}

func TestRecallExecutor_SearchNormal(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "search",
		"query":     "project",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want %q", res.Status, "ok")
	}
	resultStr, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string, got %T", res.Result)
	}
	// Should contain the memory that matched "project".
	if !strings.Contains(resultStr, "user-prefs") {
		t.Errorf("expected result to contain 'user-prefs', got: %s", resultStr)
	}
}

func TestRecallExecutor_SearchNoResults(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "search",
		"query":     "xyzzy",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "no_results" {
		t.Errorf("Status = %q, want %q", res.Status, "no_results")
	}
}

func TestRecallExecutor_SearchEmptyQuery(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "search",
		"query":     "",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for empty query")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error != "memory: query is required for search" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestRecallExecutor_ReadNormal(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "read",
		"name":      "user-prefs",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want %q", res.Status, "ok")
	}
	resultStr, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string, got %T", res.Result)
	}
	if !strings.Contains(resultStr, "User Preferences") {
		t.Errorf("expected result to contain title 'User Preferences', got: %s", resultStr)
	}
	if !strings.Contains(resultStr, "backend services") {
		t.Errorf("expected result to contain body text, got: %s", resultStr)
	}
}

func TestRecallExecutor_ReadNotFound(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "read",
		"name":      "nonexistent",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for missing memory")
	}
	if res.Status != "not_found" {
		t.Errorf("Status = %q, want %q", res.Status, "not_found")
	}
	if res.Error != `memory: "nonexistent" not found` {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestRecallExecutor_ListNormal(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want %q", res.Status, "ok")
	}
	resultStr, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string, got %T", res.Result)
	}
	// All three memories should appear.
	if !strings.Contains(resultStr, "user-prefs") {
		t.Errorf("expected 'user-prefs' in list, got: %s", resultStr)
	}
	if !strings.Contains(resultStr, "feedback-style") {
		t.Errorf("expected 'feedback-style' in list, got: %s", resultStr)
	}
	if !strings.Contains(resultStr, "global-ref") {
		t.Errorf("expected 'global-ref' in list, got: %s", resultStr)
	}
}

func TestRecallExecutor_ListEmpty(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "empty" {
		t.Errorf("Status = %q, want %q", res.Status, "empty")
	}
	if res.Result != "No memories stored yet." {
		t.Errorf("Result = %q, want %q", res.Result, "No memories stored yet.")
	}
}

func TestRecallExecutor_ListWithTypeFilter(t *testing.T) {
	store := newMemoryTestStore(t, sampleMemories()...)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "list",
		"type":      "user",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "ok" {
		t.Errorf("Status = %q, want %q", res.Status, "ok")
	}
	resultStr, ok := res.Result.(string)
	if !ok {
		t.Fatalf("Result is not string, got %T", res.Result)
	}
	// Only user-prefs should appear.
	if !strings.Contains(resultStr, "user-prefs") {
		t.Errorf("expected 'user-prefs' in filtered list, got: %s", resultStr)
	}
	if strings.Contains(resultStr, "feedback-style") {
		t.Errorf("did not expect 'feedback-style' in user-filtered list, got: %s", resultStr)
	}
	if strings.Contains(resultStr, "global-ref") {
		t.Errorf("did not expect 'global-ref' in user-filtered list, got: %s", resultStr)
	}
}

func TestRecallExecutor_UnknownOperation(t *testing.T) {
	store := newMemoryTestStore(t)
	a := NewMemoryRecallAction(MemoryRecallConfig{Store: store})

	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false for unknown operation")
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
}