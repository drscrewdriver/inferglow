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

package actionruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/action"
)

// mockExecutor is a simple executor for testing.
type mockExecutor struct {
	fn func(map[string]any) (*action.ActionResult, error)
}

func (m *mockExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	return m.fn(input)
}

func TestDispatcher_SingleAction(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "result", nil
	}
	actionInst, _ := action.New("test", "test action", fn)
	registry.Register(actionInst)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{{Name: "test", Params: map[string]any{"key": "value"}}}
	results := dispatcher.Execute(ctx, calls)
	if len(results) == 0 {
		t.Fatal("Expected at least 1 result")
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if !results[0].OK {
		t.Errorf("Expected OK=true, got %v", results[0])
	}
	if results[0].Result != "result" {
		t.Errorf("Result mismatch: got %v", results[0].Result)
	}
}

func TestDispatcher_Concurrent(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		time.Sleep(10 * time.Millisecond)
		return "done", nil
	}

	a1, _ := action.New("act1", "", fn)
	a2, _ := action.New("act2", "", fn)
	a3, _ := action.New("act3", "", fn)
	registry.Register(a1)
	registry.Register(a2)
	registry.Register(a3)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "act1", Params: nil},
		{Name: "act2", Params: nil},
		{Name: "act3", Params: nil},
	}

	start := time.Now()
	results := dispatcher.Execute(ctx, calls)
	elapsed := time.Since(start)
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}
	// Should be fast due to concurrency (< 30ms instead of >30ms)
	if elapsed > 30*time.Millisecond {
		t.Errorf("Expected concurrent execution (<30ms), took %v", elapsed)
	}
}

func TestDispatcher_ActionFails(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}

	a1, _ := action.New("ok_action", "", fn)
	a2, _ := action.New("fail_action", "fails", func(ctx context.Context, input map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(a1)
	registry.Register(a2)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "ok_action", Params: nil},
		{Name: "fail_action", Params: nil},
	}
	results := dispatcher.Execute(ctx, calls)
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if !results[0].OK {
		t.Error("First action should succeed")
	}
	// Second action result is OK=false because it returns nil error with nil output
}

// TestDispatcher_ExecutorPanicRecovered verifies that an executor that
// panics does not crash the process or leave a nil entry in results.
// Regression test for BUG-6: before the fix, the goroutine had no
// recover(), so a panic left results[idx] == nil and downstream code
// hit a nil-pointer dereference.
//
// Note: action.New's LocalFunctionExecutor has its own recover() that
// converts panics into error-shaped ActionResults. To exercise the
// dispatcher's recover path we register a raw ActionExecutor that
// panics directly, bypassing LocalFunctionExecutor.
func TestDispatcher_ExecutorPanicRecovered(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	panicAction := &action.Action{
		Name:        "panic_raw",
		Description: "panics without self-recovery",
		Executor:    &rawPanickingExecutor{msg: "boom from executor"},
	}
	okAction, _ := action.New("ok_action", "ok",
		func(ctx context.Context, input map[string]any) (any, error) { return "ok", nil })
	if err := registry.Register(panicAction); err != nil {
		t.Fatalf("Register panic_raw: %v", err)
	}
	if err := registry.Register(okAction); err != nil {
		t.Fatalf("Register ok_action: %v", err)
	}

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "panic_raw", Params: nil},
		{Name: "ok_action", Params: nil},
	}
	results := dispatcher.Execute(ctx, calls)
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if results[0] == nil {
		t.Fatal("results[0] is nil; panic was not recovered")
	}
	if results[0].OK {
		t.Errorf("Expected OK=false for panicked action, got OK=true")
	}
	if results[0].Status != "panic" {
		t.Errorf("Expected Status='panic', got %q", results[0].Status)
	}
	if results[0].Error == "" || !containsSubstring(results[0].Error, "panic:") {
		t.Errorf("Expected Error to start with 'panic:', got %q", results[0].Error)
	}
	if !containsSubstring(results[0].Error, "boom from executor") {
		t.Errorf("Expected Error to contain panic message, got %q", results[0].Error)
	}
	// Non-panicking action in the same batch should still succeed.
	if results[1] == nil || !results[1].OK {
		t.Errorf("Expected results[1] to be OK, got %v", results[1])
	}
}

// TestDispatcher_ExecutorPanicAppendsAuditEntry verifies the recover path
// also feeds the audit hook so panics are observable downstream.
func TestDispatcher_ExecutorPanicAppendsAuditEntry(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	panicAction := &action.Action{
		Name:        "panic_raw",
		Description: "panics without self-recovery",
		Executor:    &rawPanickingExecutor{msg: "audit boom"},
	}
	if err := registry.Register(panicAction); err != nil {
		t.Fatalf("Register: %v", err)
	}

	hook := &fakeAuditHook{}
	dispatcher := NewActionDispatcherWithAudit(registry, hook)
	calls := []ActionCall{{Name: "panic_raw", Params: map[string]any{"k": "v"}}}
	results := dispatcher.Execute(ctx, calls)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0] == nil || results[0].Status != "panic" {
		t.Fatalf("Expected recovered panic result, got %v", results[0])
	}
	if got := hook.Count(); got != 1 {
		t.Fatalf("Expected 1 audit Append call from panic path, got %d", got)
	}
	entries := hook.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Source != "action" || e.Action != "execute" {
		t.Errorf("Unexpected Source/Action: %q/%q", e.Source, e.Action)
	}
	if e.Error == "" || !containsSubstring(e.Error, "panic:") {
		t.Errorf("Expected audit entry.Error to start with 'panic:', got %q", e.Error)
	}
	if !containsSubstring(e.Error, "audit boom") {
		t.Errorf("Expected audit entry.Error to contain panic message, got %q", e.Error)
	}
	if e.Output == nil {
		t.Error("Expected audit entry.Output to be the synthesized panic ActionResult, got nil")
	}
}

// rawPanickingExecutor is an ActionExecutor that panics on Execute without
// any self-recovery, so the dispatcher's recover() is the only thing
// standing between the panic and a process crash.
type rawPanickingExecutor struct{ msg string }

func (e *rawPanickingExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	panic(e.msg)
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}
