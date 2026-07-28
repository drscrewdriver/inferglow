package flowdef

import (
	"context"
	"testing"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/stage"
)

// newStageRegistry builds a stage.Registry with the given stage names, each
// registered as a no-op stage that echoes nothing. It is a test helper shared
// by the FlowRegistry tests.
func newStageRegistry(names ...string) *stage.Registry {
	reg := stage.NewRegistry()
	for _, n := range names {
		name := n
		reg.Register(name, func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
			return stage.Outputs{}, nil
		})
	}
	return reg
}

func TestFlowRegistry_RegisterGet(t *testing.T) {
	r := NewFlowRegistry()
	def := validFlowDef()
	if err := r.Register("f1", def, nil); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	got, ok := r.Get("f1")
	if !ok {
		t.Fatal("expected to find f1")
	}
	// A non-empty existing name is preserved (not overwritten by the key).
	if got.Metadata.Name != "test-flow" {
		t.Errorf("expected existing name test-flow preserved, got %q", got.Metadata.Name)
	}

	if _, ok := r.Get("missing"); ok {
		t.Error("expected ok=false for missing flow")
	}
}

func TestFlowRegistry_RegisterNil(t *testing.T) {
	r := NewFlowRegistry()
	if err := r.Register("f", nil, nil); err == nil {
		t.Fatal("expected error for nil definition, got nil")
	}
}

func TestFlowRegistry_RegisterBackfillsName(t *testing.T) {
	r := NewFlowRegistry()
	def := validFlowDef()
	def.Metadata.Name = ""
	if err := r.Register("auto", def, nil); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if _, ok := r.Get("auto"); !ok {
		t.Fatal("expected flow registered under auto")
	}
}

func TestFlowRegistry_Replace(t *testing.T) {
	r := NewFlowRegistry()
	_ = r.Register("f", validFlowDef(), nil)

	def2 := validFlowDef()
	def2.Metadata.Name = "f"
	def2.Metadata.Version = "2.0.0"
	if err := r.Register("f", def2, nil); err != nil {
		t.Fatalf("re-register returned error: %v", err)
	}
	got, _ := r.Get("f")
	if got.Metadata.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0 after replace, got %q", got.Metadata.Version)
	}
}

func TestFlowRegistry_List(t *testing.T) {
	r := NewFlowRegistry()
	_ = r.Register("a", validFlowDef(), nil)
	_ = r.Register("b", validFlowDef(), nil)

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("list missing expected names: %v", names)
	}
}

func TestFlowRegistry_Delete(t *testing.T) {
	r := NewFlowRegistry()
	_ = r.Register("f", validFlowDef(), nil)
	r.Delete("f")
	if _, ok := r.Get("f"); ok {
		t.Error("expected flow removed after Delete")
	}
	// Delete of a missing name is a no-op.
	r.Delete("missing")
}

func TestFlowRegistry_GetCompiled(t *testing.T) {
	r := NewFlowRegistry()
	adapter := NewAdapter(newStageRegistry("triage", "coder", "committer"))

	// Without an adapter (definitions-only), GetCompiled must be ok=false.
	if err := r.Register("defonly", validFlowDef(), nil); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if _, ok := r.GetCompiled("defonly"); ok {
		t.Error("expected ok=false for definitions-only registration")
	}

	// With an adapter, the flow is compiled and cached.
	if err := r.Register("compiled", validFlowDef(), adapter); err != nil {
		t.Fatalf("Register with adapter returned error: %v", err)
	}
	if f, ok := r.GetCompiled("compiled"); !ok || f == nil {
		t.Error("expected compiled flow to be present")
	}
}

func TestFlowRegistry_RegisterCompileError(t *testing.T) {
	r := NewFlowRegistry()
	adapter := NewAdapter(newStageRegistry("triage"))

	// A step referencing an unknown stage must fail compilation and leave the
	// registry unchanged.
	def := validFlowDef()
	def.Spec.Steps[1].Stage = "does_not_exist"
	if err := r.Register("bad", def, adapter); err == nil {
		t.Fatal("expected compile error for unknown stage, got nil")
	}
	if _, ok := r.Get("bad"); ok {
		t.Error("registry should not contain a failed registration")
	}
}