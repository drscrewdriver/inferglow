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

package stage

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/inferglow/flow"
)

// TestRegistry_BackwardCompat is the critical regression guard: exercising only
// the existing Register/Get/List semantics proves their behaviour is unchanged
// even after the meta map has been populated.
func TestRegistry_BackwardCompat(t *testing.T) {
	r := NewRegistry()

	// Get of an unknown name returns ok=false.
	if _, ok := r.Get("missing"); ok {
		t.Fatal("expected ok=false for unknown stage")
	}

	// Register then Get round-trip.
	r.Register("a", tagged("first"))
	got, ok := r.Get("a")
	if !ok {
		t.Fatal("expected the registered stage to be found")
	}
	if gotTag(got) != "first" {
		t.Errorf("stored func differs from registered func, got tag %q", gotTag(got))
	}

	// Repeated registration replaces the previous one.
	r.Register("a", tagged("second"))
	replaced, ok := r.Get("a")
	if !ok {
		t.Fatal("stage should still be present after replacement")
	}
	if gotTag(replaced) != "second" {
		t.Errorf("re-register should replace, got tag %q", gotTag(replaced))
	}
	// The pre-replacement handle still refers to the old func value.
	if gotTag(got) != "first" {
		t.Errorf("old handle should keep earlier func, got tag %q", gotTag(got))
	}

	// List returns the full set of registered names.
	r.Register("b", tagged("b"))
	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if sorted[0] != "a" || sorted[1] != "b" {
		t.Errorf("unexpected names: %v", sorted)
	}
}

// TestMeta_IsolatedFromFunc verifies the Func map (m) and the
// metadata map (meta) are fully decoupled: Register only affects Get, and
// RegisterMeta only affects GetMeta.
func TestMeta_IsolatedFromFunc(t *testing.T) {
	r := NewRegistry()
	r.Register("s", tagged("s"))
	r.RegisterMeta("s", Meta{Name: "s", Description: "decl"})

	// Get still reads only m, even though meta exists for the same name.
	if _, ok := r.Get("s"); !ok {
		t.Fatal("stage func should be present")
	}

	// RegisterMeta without a stage func: Get must not see it, GetMeta must.
	r2 := NewRegistry()
	r2.RegisterMeta("meta-only", Meta{Name: "meta-only"})
	if _, ok := r2.Get("meta-only"); ok {
		t.Fatal("Get must not read the meta map")
	}
	if m, ok := r2.GetMeta("meta-only"); !ok || m.Name != "meta-only" {
		t.Fatalf("GetMeta should read meta map, ok=%v", ok)
	}
}

// TestMeta_RegisterGetMeta covers RegisterMeta -> GetMeta round-trip, name
// auto back-fill, overwrite replacement, and unknown-name handling.
func TestMeta_RegisterGetMeta(t *testing.T) {
	r := NewRegistry()

	// Unknown name returns zero value and ok=false.
	if m, ok := r.GetMeta("nope"); ok || m.Name != "" {
		t.Fatalf("expected (zero, false), got (%v, %v)", m, ok)
	}

	// Empty Name is auto back-filled to the registration key.
	r.RegisterMeta("alpha", Meta{Description: "a"})
	m, ok := r.GetMeta("alpha")
	if !ok {
		t.Fatal("expected meta to be found")
	}
	if m.Name != "alpha" {
		t.Errorf("expected Name back-filled to %q, got %q", "alpha", m.Name)
	}
	if m.Description != "a" {
		t.Errorf("expected Description %q, got %q", "a", m.Description)
	}

	// Registering under a different key preserves an explicit Name.
	r.RegisterMeta("key", Meta{Name: "renamed", Description: "b"})
	m, ok = r.GetMeta("key")
	if !ok || m.Name != "renamed" {
		t.Fatalf("explicit Name must be preserved, ok=%v name=%q", ok, m.Name)
	}

	// Re-registration replaces the previous metadata.
	r.RegisterMeta("alpha", Meta{Description: "updated"})
	m, ok = r.GetMeta("alpha")
	if !ok || m.Description != "updated" {
		t.Fatalf("re-register should replace meta, ok=%v desc=%q", ok, m.Description)
	}
	if m.Name != "alpha" {
		t.Errorf("re-registered meta should keep back-filled Name, got %q", m.Name)
	}
}

// TestMeta_RegisterWithMeta verifies one call populates both Get and GetMeta.
func TestMeta_RegisterWithMeta(t *testing.T) {
	r := NewRegistry()
	r.RegisterWithMeta("both", tagged("fn"), Meta{
		Name:        "both",
		Description: "one-shot",
		InputPorts:  []PortDef{{Name: "in", Type: PortString}},
	})

	if f, ok := r.Get("both"); !ok || gotTag(f) != "fn" {
		t.Fatalf("Get should succeed via RegisterWithMeta")
	}
	m, ok := r.GetMeta("both")
	if !ok {
		t.Fatal("GetMeta should succeed via RegisterWithMeta")
	}
	if m.Name != "both" || m.Description != "one-shot" {
		t.Errorf("meta not stored as expected: %+v", m)
	}
	if len(m.InputPorts) != 1 || m.InputPorts[0].Name != "in" {
		t.Errorf("input ports not preserved: %+v", m.InputPorts)
	}
}

// TestMeta_MetaNames verifies the set of names carrying a Meta.
func TestMeta_MetaNames(t *testing.T) {
	r := NewRegistry()
	r.RegisterMeta("x", Meta{})
	r.RegisterMeta("y", Meta{})
	r.RegisterMeta("z", Meta{})

	names := r.MetaNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 meta names, got %d: %v", len(names), names)
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if sorted[0] != "x" || sorted[1] != "y" || sorted[2] != "z" {
		t.Errorf("unexpected meta names: %v", sorted)
	}

	// MetaNames is decoupled from stage funcs: plain func registration must not
	// change the meta set.
	r2 := NewRegistry()
	r2.Register("fn", tagged("fn"))
	if len(r2.MetaNames()) != 0 {
		t.Errorf("MetaNames should ignore plain stage funcs")
	}
}

// TestMeta_PortLookup verifies field-level lookups on a Meta and the
// package-level FindPort helper.
func TestMeta_PortLookup(t *testing.T) {
	m := Meta{
		InputPorts:  []PortDef{{Name: "prompt", Type: PortString}, {Name: "n", Type: PortInt}},
		OutputPorts: []PortDef{{Name: "out", Type: PortJSON}},
	}

	// InputPort hit with full field visibility.
	in, ok := m.InputPort("prompt")
	if !ok || in.Name != "prompt" || in.Type != PortString {
		t.Fatalf("InputPort hit expected, ok=%v def=%+v", ok, in)
	}
	if _, ok := m.InputPort("missing"); ok {
		t.Fatal("InputPort should miss for unknown input")
	}

	// OutputPort lookup.
	out, ok := m.OutputPort("out")
	if !ok || out.Name != "out" || out.Type != PortJSON {
		t.Fatalf("OutputPort hit expected, ok=%v def=%+v", ok, out)
	}
	if _, ok := m.OutputPort("prompt"); ok {
		t.Fatal("OutputPort must not search the input ports")
	}

	// FindPort handles empty slices gracefully.
	if _, ok := FindPort(nil, "anything"); ok {
		t.Fatal("FindPort should miss on an empty slice")
	}
}

// TestPortType_Compatible verifies the schema-level CompatibleWith rule:
// PortAny is compatible with everything; otherwise types must match exactly.
func TestPortType_Compatible(t *testing.T) {
	types := []PortType{PortString, PortInt, PortFloat, PortBool, PortJSON, PortFile, PortCode, PortModel}

	// PortAny is compatible with every type (in both directions).
	for _, pt := range types {
		if !PortAny.CompatibleWith(pt) {
			t.Errorf("PortAny should be compatible with %q", pt)
		}
		if !pt.CompatibleWith(PortAny) {
			t.Errorf("%q should be compatible with PortAny", pt)
		}
	}
	if !PortAny.CompatibleWith(PortAny) {
		t.Error("PortAny should be compatible with itself")
	}

	// Identical concrete types match.
	for _, pt := range types {
		if !pt.CompatibleWith(pt) {
			t.Errorf("%q should be compatible with itself", pt)
		}
	}

	// Distinct concrete types do not match.
	if PortString.CompatibleWith(PortInt) {
		t.Error("string and int are not compatible")
	}
	if PortJSON.CompatibleWith(PortFile) {
		t.Error("json and file are not compatible")
	}
}

// TestMeta_Concurrent exercises concurrent Register/RegisterMeta/Get/GetMeta
// to guard against data races under -race.
func TestMeta_Concurrent(t *testing.T) {
	r := NewRegistry()
	const workers = 8
	const per = 100

	// Deterministic, distinct names across workers. Encoding: workers (a-h),
	// then i%26 (a-z), then i/26 (a-d) — each (w,i) maps to a unique triple.
	names := make([]string, 0, workers*per)
	for w := 0; w < workers; w++ {
		for i := 0; i < per; i++ {
			names = append(names, "s"+stringrune(w)+stringrune(i)+stringrune(i/26))
		}
	}

	var wg sync.WaitGroup
	// Writers: RegisterWithMeta under concurrency.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for _, name := range names[start:end] {
				r.RegisterWithMeta(name, tagged("fn"), Meta{Name: name})
			}
		}(w*per, (w+1)*per)
	}

	// Readers: concurrent Get/GetMeta/MetaNames while writers run.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				for _, n := range r.MetaNames() {
					_, _ = r.Get(n)
					_, _ = r.GetMeta(n)
				}
			}
		}()
	}

	wg.Wait()

	// Final state is consistent: every registered name has both sides present.
	for _, n := range names {
		if _, ok := r.Get(n); !ok {
			t.Errorf("stage func missing for %q", n)
		}
		if _, ok := r.GetMeta(n); !ok {
			t.Errorf("meta missing for %q", n)
		}
	}
	if len(r.MetaNames()) != workers*per {
		t.Errorf("expected %d meta names, got %d", workers*per, len(r.MetaNames()))
	}
}

// tagged returns a Func that echoes a stable tag through its Outputs so a
// test can verify which function instance is actually stored.
func tagged(tag string) Func {
	return func(ctx context.Context, in Inputs, fctx flow.FlowContext) (Outputs, error) {
		_ = ctx
		_ = in
		_ = fctx
		return Outputs{"tag": tag}, nil
	}
}

// gotTag invokes a Func and returns its emitted tag.
func gotTag(f Func) string {
	outs, err := f(context.Background(), Inputs{}, nil)
	if err != nil {
		return "<err:" + err.Error() + ">"
	}
	tag, _ := outs["tag"].(string)
	return tag
}

// stringrune converts a small integer into a single-character string for
// building deterministic, short registry names.
func stringrune(i int) string {
	return string(rune('a' + i%26))
}
