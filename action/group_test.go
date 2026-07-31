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

package action

import (
	"errors"
	"testing"
)

// newGroupTestAction builds an Action with the given tags and a mock executor.
func newGroupTestAction(name string, tags []string) *Action {
	return &Action{
		Name:        name,
		Description: "test action " + name,
		Schema:      map[string]any{"type": "object"},
		Executor:    &mockExecutor{result: &ActionResult{OK: true, Status: "success", Result: name}},
		Tags:        tags,
	}
}

// registerGroupTestActions registers a few tagged actions into a registry and
// returns the registry plus the group view derived from it.
func registerGroupTestActions() (*ActionRegistry, *GroupRegistry) {
	r := NewRegistry()
	for _, a := range []*Action{
		// read-only group members (reserved tag convention group:<name>)
		newGroupTestAction("cat", []string{"group:readonly", "read"}),
		newGroupTestAction("ls", []string{"group:readonly", "group:plan", "read"}),
		// write group members
		newGroupTestAction("write_file", []string{"group:write", "write"}),
		newGroupTestAction("rm", []string{"group:write", "group:plan", "write"}),
		// untagged action
		newGroupTestAction("orphan", []string{"loose"}),
	} {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}

	g := NewGroupRegistry(r)
	mustRegister := func(grp *ToolGroup) {
		if err := g.Register(grp); err != nil {
			panic(err)
		}
	}
	mustRegister(&ToolGroup{Name: "readonly", Description: "read-only tools", Tags: []string{"group:readonly"}})
	mustRegister(&ToolGroup{Name: "write", Description: "write tools", Tags: []string{"group:write"}})
	mustRegister(&ToolGroup{Name: "plan", Description: "plan mode tools", Tags: []string{"group:plan"}})
	return r, g
}

func TestGroupRegistryRegisterAndGet(t *testing.T) {
	_, g := registerGroupTestActions()
	grp, err := g.Get("readonly")
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if grp.Name != "readonly" {
		t.Errorf("expected group name readonly, got %q", grp.Name)
	}
	if grp.Description != "read-only tools" {
		t.Errorf("expected description read-only tools, got %q", grp.Description)
	}
}

func TestGroupRegistryRegisterDuplicate(t *testing.T) {
	_, g := registerGroupTestActions()
	err := g.Register(&ToolGroup{Name: "readonly", Tags: []string{"group:readonly"}})
	if !errors.Is(err, ErrGroupAlreadyRegistered) {
		t.Fatalf("expected ErrGroupAlreadyRegistered, got %v", err)
	}
}

func TestGroupRegistryRegisterNilAndEmpty(t *testing.T) {
	_, g := registerGroupTestActions()
	if err := g.Register(nil); !errors.Is(err, ErrGroupAlreadyRegistered) {
		t.Fatalf("expected nil-group error, got %v", err)
	}
	if err := g.Register(&ToolGroup{Name: "", Tags: nil}); !errors.Is(err, ErrGroupAlreadyRegistered) {
		t.Fatalf("expected empty-name error, got %v", err)
	}
}

func TestGroupRegistryGetMissing(t *testing.T) {
	_, g := registerGroupTestActions()
	if _, err := g.Get("nope"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestGroupRegistryListSorted(t *testing.T) {
	_, g := registerGroupTestActions()
	got := g.List()
	want := []string{"plan", "readonly", "write"}
	if len(got) != len(want) {
		t.Fatalf("expected %d groups, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected sorted list %v, got %v", want, got)
			break
		}
	}
}

func TestGroupRegistryUnregister(t *testing.T) {
	_, g := registerGroupTestActions()
	if !g.Unregister("write") {
		t.Fatalf("expected Unregister(write) to return true")
	}
	if g.Unregister("write") {
		t.Fatalf("expected second Unregister(write) to return false")
	}
}

func TestGroupRegistryListActionNames(t *testing.T) {
	_, g := registerGroupTestActions()

	ro, err := g.ListActionNames("readonly")
	if err != nil {
		t.Fatalf("ListActionNames(readonly) error: %v", err)
	}
	if len(ro) != 2 || ro[0] != "cat" || ro[1] != "ls" {
		t.Errorf("expected readonly=[cat ls], got %v", ro)
	}

	plan, err := g.ListActionNames("plan")
	if err != nil {
		t.Fatalf("ListActionNames(plan) error: %v", err)
	}
	if len(plan) != 2 || plan[0] != "ls" || plan[1] != "rm" {
		t.Errorf("expected plan=[ls rm], got %v", plan)
	}

	if _, err := g.ListActionNames("missing"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for missing group, got %v", err)
	}
}

func TestGroupRegistryHasAction(t *testing.T) {
	_, g := registerGroupTestActions()
	if !g.HasAction("readonly", "cat") {
		t.Errorf("expected cat to be in readonly group")
	}
	if g.HasAction("readonly", "write_file") {
		t.Errorf("expected write_file NOT to be in readonly group")
	}
	if g.HasAction("missing", "cat") {
		t.Errorf("expected missing group to contain nothing")
	}
}

func TestGroupRegistryNilRegistry(t *testing.T) {
	g := NewGroupRegistry(nil)
	if err := g.Register(&ToolGroup{Name: "g", Tags: []string{"x"}}); err != nil {
		t.Fatalf("Register on nil-registry group view failed: %v", err)
	}
	names, err := g.ListActionNames("g")
	if err != nil {
		t.Fatalf("ListActionNames error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty member list for nil registry, got %v", names)
	}
}

func TestGroupRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	g := NewGroupRegistry(r)

	// Register a fixed set of groups and actions concurrently; the derived
	// view must remain consistent without panics or data races.
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(i int) {
			name := "g" + string(rune('a'+i%26))
			_ = g.Register(&ToolGroup{Name: name, Tags: []string{"group:" + name}})
			_ = r.Register(newGroupTestAction(name, []string{"group:" + name}))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	// Unregister and read concurrently.
	for i := 0; i < 50; i++ {
		go func() {
			_ = g.List()
			_, _ = g.ListActionNames("greadonly")
		}()
	}
	_ = g.Unregister("ga")
}