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
	"sync"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
)

// fakeAuditHook is a test AuditHook that counts Append calls and captures
// every entry it receives. It is safe for concurrent use because Execute
// fans out ActionCalls across goroutines.
type fakeAuditHook struct {
	mu      sync.Mutex
	count   int
	entries []*audit.AuditEntry
}

func (h *fakeAuditHook) Append(entry *audit.AuditEntry) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.entries = append(h.entries, entry)
	return "", nil
}

func (h *fakeAuditHook) IsEnabled() bool { return true }

func (h *fakeAuditHook) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func (h *fakeAuditHook) Snapshot() []*audit.AuditEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*audit.AuditEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

// TestDispatcher_AppendsAuditEntry verifies that the dispatcher appends one
// audit entry per ActionCall, regardless of action success or failure, with
// Source="action" and Action="execute".
func TestDispatcher_AppendsAuditEntry(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "result", nil
	}
	actionInst, _ := action.New("test", "test action", fn)
	registry.Register(actionInst)

	hook := &fakeAuditHook{}
	dispatcher := NewActionDispatcherWithAudit(registry, hook)
	calls := []ActionCall{
		{Name: "test", Params: map[string]any{"key": "value1"}},
		{Name: "test", Params: map[string]any{"key": "value2"}},
	}
	results := dispatcher.Execute(ctx, calls)
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if got := hook.Count(); got != 2 {
		t.Fatalf("Expected 2 audit Append calls, got %d", got)
	}
	entries := hook.Snapshot()
	for i, entry := range entries {
		if entry.Source != "action" {
			t.Errorf("Entry %d: expected Source='action', got %q", i, entry.Source)
		}
		if entry.Action != "execute" {
			t.Errorf("Entry %d: expected Action='execute', got %q", i, entry.Action)
		}
		if entry.Metadata == nil || entry.Metadata["action_name"] != "test" {
			t.Errorf("Entry %d: expected Metadata[action_name]='test', got %v", i, entry.Metadata)
		}
	}
}

// TestDispatcher_NilAuditHookNoOp verifies that a nil hook does not panic
// and produces the same results as the legacy behavior.
func TestDispatcher_NilAuditHookNoOp(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}
	actionInst, _ := action.New("noop", "", fn)
	registry.Register(actionInst)

	dispatcher := NewActionDispatcherWithAudit(registry, nil)
	calls := []ActionCall{{Name: "noop", Params: nil}}
	results := dispatcher.Execute(ctx, calls)
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if !results[0].OK {
		t.Errorf("Expected OK=true, got %v", results[0])
	}
}

// TestDispatcher_DefaultConstructorUsesNoOp verifies that NewActionDispatcher
// returns a dispatcher whose auditHook is a NoOpHook, preserving legacy
// behavior.
func TestDispatcher_DefaultConstructorUsesNoOp(t *testing.T) {
	registry := action.NewRegistry()
	dispatcher := NewActionDispatcher(registry)
	if dispatcher.auditHook == nil {
		t.Fatal("Expected non-nil auditHook from NewActionDispatcher")
	}
	if _, ok := dispatcher.auditHook.(*audit.NoOpHook); !ok {
		t.Errorf("Expected *audit.NoOpHook, got %T", dispatcher.auditHook)
	}
	if dispatcher.auditHook.IsEnabled() {
		t.Error("NoOpHook should not be enabled")
	}
}
