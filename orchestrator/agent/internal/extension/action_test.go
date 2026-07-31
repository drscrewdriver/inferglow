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

package extension

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
)

type mockExecutor struct{}

func (mockExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	return &action.ActionResult{OK: true, Status: "success", Result: input}, nil
}

func newTestAction(name string, tags []string) *action.Action {
	return &action.Action{
		Name:        name,
		Description: "test " + name,
		Schema:      map[string]any{"type": "object"},
		Executor:    mockExecutor{},
		Tags:        tags,
	}
}

func newTestExtension() *ActionExtension {
	e := NewActionExtension()
	for _, a := range []*action.Action{
		newTestAction("cat", []string{"group:readonly", "read"}),
		newTestAction("ls", []string{"group:readonly", "group:plan", "read"}),
		newTestAction("write_file", []string{"group:write", "write"}),
		newTestAction("rm", []string{"group:write", "group:plan", "write"}),
	} {
		_ = e.Register(a)
	}
	_ = e.RegisterGroup(&action.ToolGroup{Name: "readonly", Description: "read-only", Tags: []string{"group:readonly"}})
	_ = e.RegisterGroup(&action.ToolGroup{Name: "write", Description: "write", Tags: []string{"group:write"}})
	_ = e.RegisterGroup(&action.ToolGroup{Name: "plan", Description: "plan", Tags: []string{"group:plan"}})
	return e
}

func TestListActionsBackwardCompatible(t *testing.T) {
	e := newTestExtension()
	got := e.ListActions()
	if len(got) != 4 {
		t.Fatalf("expected 4 tool definitions, got %d", len(got))
	}
	// ListActions must remain source-compatible (unchanged signature) and
	// continue to return every registered action.
	names := map[string]bool{}
	for _, def := range got {
		if n, ok := def["name"].(string); ok {
			names[n] = true
		}
	}
	for _, want := range []string{"cat", "ls", "write_file", "rm"} {
		if !names[want] {
			t.Errorf("expected tool definition %q to be present", want)
		}
	}
}

func TestListActionsByGroup(t *testing.T) {
	e := newTestExtension()
	ro, err := e.ListActionsByGroup("readonly")
	if err != nil {
		t.Fatalf("ListActionsByGroup(readonly) error: %v", err)
	}
	if len(ro) != 2 {
		t.Fatalf("expected 2 readonly tools, got %d", len(ro))
	}
	if ro[0]["name"] != "cat" || ro[1]["name"] != "ls" {
		t.Errorf("expected readonly=[cat ls], got %v", ro)
	}

	if _, err := e.ListActionsByGroup("missing"); !errors.Is(err, action.ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestListActionsFiltered(t *testing.T) {
	e := newTestExtension()

	// plan group has ls (read) and rm (write). A read-only filter should
	// keep only ls.
	specs := map[string]*action.ActionSpec{
		"ls": {Name: "ls", SideEffectLevel: action.SideEffectRead},
		"rm": {Name: "rm", SideEffectLevel: action.SideEffectWrite},
	}
	filter := action.ReadOnlyProfile()
	got, err := e.ListActionsFiltered("plan", filter, specs)
	if err != nil {
		t.Fatalf("ListActionsFiltered error: %v", err)
	}
	if len(got) != 1 || got[0]["name"] != "ls" {
		t.Errorf("expected filtered plan=[ls], got %v", got)
	}

	// Nil filter passes everything in the group.
	all, err := e.ListActionsFiltered("plan", nil, specs)
	if err != nil {
		t.Fatalf("ListActionsFiltered(nil) error: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected nil filter to return 2 plan tools, got %d", len(all))
	}
}

func TestGetGroupRegistry(t *testing.T) {
	e := newTestExtension()
	g := e.GetGroupRegistry()
	if g == nil {
		t.Fatal("expected non-nil group registry")
	}
	if !g.HasAction("readonly", "cat") {
		t.Errorf("expected cat to be in readonly group")
	}
}

func TestSetRegistryRebindsGroups(t *testing.T) {
	e := newTestExtension()
	// Replacing the registry must re-bind the group view to the new registry.
	// Note: previously registered groups are dropped along with the old
	// registry, so we re-register a group against the new one.
	r := action.NewRegistry()
	_ = r.Register(newTestAction("new_ro", []string{"group:readonly"}))
	e.SetRegistry(r)
	_ = e.RegisterGroup(&action.ToolGroup{Name: "readonly", Tags: []string{"group:readonly"}})

	ro, err := e.ListActionsByGroup("readonly")
	if err != nil {
		t.Fatalf("ListActionsByGroup after SetRegistry error: %v", err)
	}
	if len(ro) != 1 || ro[0]["name"] != "new_ro" {
		t.Errorf("expected new_ro after SetRegistry, got %v", ro)
	}
}