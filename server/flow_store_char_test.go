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

// Characterization tests locking down FlowStore's observable behavior against
// the ORIGINAL map-based implementation. These must continue to pass unchanged
// after the storage abstraction refactor, proving old/new equivalence.

package server

import (
	"testing"

	"github.com/inferglow/flow/flowdef"
	"github.com/inferglow/flow/stage"
)

// validFlowDef returns a minimally valid FlowDef for Register().
func validFlowDef(name string) *flowdef.FlowDef {
	return &flowdef.FlowDef{
		Metadata: flowdef.Metadata{Name: name},
		Spec: flowdef.Spec{
			Steps: []flowdef.StepDef{{Name: "start", Operator: "stage"}},
		},
	}
}

func TestFlowStoreCharRegisterGet(t *testing.T) {
	fs := NewFlowStore(stage.NewRegistry())
	if err := fs.Register(validFlowDef("f1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	def, ok := fs.Get("f1")
	if !ok {
		t.Fatal("expected f1 to be found after Register")
	}
	if def == nil || def.Metadata.Name != "f1" {
		t.Fatalf("Get returned %+v", def)
	}
}

func TestFlowStoreCharGetMissing(t *testing.T) {
	fs := NewFlowStore(stage.NewRegistry())
	def, ok := fs.Get("nope")
	if ok {
		t.Fatal("expected missing flow to report !ok")
	}
	if def != nil {
		t.Fatalf("expected nil FlowDef for missing flow, got %+v", def)
	}
}

func TestFlowStoreCharRegisterInvalid(t *testing.T) {
	fs := NewFlowStore(stage.NewRegistry())
	// nil definition must be rejected.
	if err := fs.Register(nil); err == nil {
		t.Fatal("expected error for nil def")
	}
	// Def with no steps must be rejected.
	if err := fs.Register(&flowdef.FlowDef{Metadata: flowdef.Metadata{Name: "x"}}); err == nil {
		t.Fatal("expected error for def with no steps")
	}
	// Nothing should have been stored.
	if n := len(fs.List()); n != 0 {
		t.Fatalf("List len after failed registers = %d, want 0", n)
	}
}

func TestFlowStoreCharRegisterOverwrite(t *testing.T) {
	fs := NewFlowStore(stage.NewRegistry())
	if err := fs.Register(validFlowDef("f1")); err != nil {
		t.Fatalf("Register #1: %v", err)
	}
	// Registering the same name again overwrites the stored definition.
	if err := fs.Register(validFlowDef("f1")); err != nil {
		t.Fatalf("Register #2: %v", err)
	}
	if n := len(fs.List()); n != 1 {
		t.Fatalf("List len after overwrite = %d, want 1", n)
	}
}

func TestFlowStoreCharList(t *testing.T) {
	fs := NewFlowStore(stage.NewRegistry())
	for _, name := range []string{"a", "b", "c"} {
		if err := fs.Register(validFlowDef(name)); err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
	}
	names := fs.List()
	if len(names) != 3 {
		t.Fatalf("List len = %d, want 3", len(names))
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !seen[want] {
			t.Fatalf("List missing %q", want)
		}
	}
}

func TestFlowStoreCharAdapterStages(t *testing.T) {
	reg := stage.NewRegistry()
	fs := NewFlowStore(reg)
	if fs.Adapter() == nil {
		t.Fatal("Adapter() must be non-nil")
	}
	if fs.Stages() != reg {
		t.Fatal("Stages() should return the same registry instance")
	}
}