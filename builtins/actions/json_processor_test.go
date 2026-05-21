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

	"github.com/inferglow/action"
)

func TestJSONProcessorSpec(t *testing.T) {
	if JSONProcessorSpec.SideEffectLevel != action.SideEffectNone {
		t.Errorf("SideEffectLevel = %q, want %q", JSONProcessorSpec.SideEffectLevel, action.SideEffectNone)
	}
	if JSONProcessorSpec.ApprovalRequired {
		t.Errorf("ApprovalRequired = true, want false")
	}
	if JSONProcessorSpec.SandboxRequired {
		t.Errorf("SandboxRequired = true, want false")
	}
}

func TestJSONQueryRoot(t *testing.T) {
	doc := `{"a": 1, "b": [10, 20, 30]}`
	got, err := JSONProcess(doc, "", "")
	if err != nil {
		t.Fatalf("JSONProcess error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("root not an object: %T", got)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("a = %v, want 1", m["a"])
	}
}

func TestJSONQueryDollarRoot(t *testing.T) {
	doc := `{"x": "y"}`
	got, err := JSONProcess(doc, "$", "")
	if err != nil {
		t.Fatalf("JSONProcess error: %v", err)
	}
	m, _ := got.(map[string]any)
	if m["x"] != "y" {
		t.Errorf("x = %v, want y", m["x"])
	}
}

func TestJSONQueryFieldAccess(t *testing.T) {
	doc := `{"user": {"name": "Alice", "age": 30}}`
	cases := []struct {
		path string
		want any
	}{
		{"$.user.name", "Alice"},
		{"$.user.age", float64(30)},
		{"user.name", "Alice"}, // leading $ optional
		{"$.user['name']", "Alice"},
		{`$.user["name"]`, "Alice"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := JSONProcess(doc, tc.path, "")
			if err != nil {
				t.Fatalf("JSONProcess(%q) error: %v", tc.path, err)
			}
			if got != tc.want {
				t.Errorf("JSONProcess(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestJSONQueryArrayIndex(t *testing.T) {
	doc := `{"items": [100, 200, 300, 400]}`
	cases := []struct {
		path string
		want any
	}{
		{"$.items[0]", float64(100)},
		{"$.items[3]", float64(400)},
		{"$.items[-1]", float64(400)},
		{"$.items[-2]", float64(300)},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := JSONProcess(doc, tc.path, "")
			if err != nil {
				t.Fatalf("JSONProcess(%q) error: %v", tc.path, err)
			}
			if got != tc.want {
				t.Errorf("JSONProcess(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestJSONQuerySlice(t *testing.T) {
	doc := `{"arr": [10, 20, 30, 40, 50]}`
	cases := []struct {
		path string
		want []any
	}{
		{"$.arr[1:4]", []any{float64(20), float64(30), float64(40)}},
		{"$.arr[:2]", []any{float64(10), float64(20)}},
		{"$.arr[3:]", []any{float64(40), float64(50)}},
		{"$.arr[-2:]", []any{float64(40), float64(50)}},
		{"$.arr[:]", []any{float64(10), float64(20), float64(30), float64(40), float64(50)}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := JSONProcess(doc, tc.path, "")
			if err != nil {
				t.Fatalf("JSONProcess(%q) error: %v", tc.path, err)
			}
			arr, ok := got.([]any)
			if !ok {
				t.Fatalf("got %T, want []any: %v", got, got)
			}
			if len(arr) != len(tc.want) {
				t.Fatalf("len = %d, want %d (%v)", len(arr), len(tc.want), arr)
			}
			for i := range arr {
				if arr[i] != tc.want[i] {
					t.Errorf("[%d] = %v, want %v", i, arr[i], tc.want[i])
				}
			}
		})
	}
}

func TestJSONQueryWildcard(t *testing.T) {
	doc := `{"store": {"book": [{"title": "A"}, {"title": "B"}], "bike": {"color": "red"}}}`
	got, err := JSONProcess(doc, "$.store.*", "")
	if err != nil {
		t.Fatalf("JSONProcess error: %v", err)
	}
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(arr) != 2 {
		t.Errorf("wildcard returned %d items, want 2", len(arr))
	}
}

func TestJSONQueryChained(t *testing.T) {
	doc := `{"store": {"book": [{"title": "A", "price": 10}, {"title": "B", "price": 20}]}}`
	got, err := JSONProcess(doc, "$.store.book[0].title", "")
	if err != nil {
		t.Fatalf("JSONProcess error: %v", err)
	}
	if got != "A" {
		t.Errorf("got %v, want A", got)
	}
}

func TestJSONQueryErrors(t *testing.T) {
	cases := []struct {
		doc  string
		path string
		err  string
	}{
		{`{"a":1}`, "$.missing", "field \"missing\" not found"},
		{`{"a":1}`, "$.a.b", "cannot access field"},
		{`[1,2]`, "$[5]", "out of range"},
		{`{"a":1}`, "$.a[0]", "cannot index into"},
		{`{`, "", "parse error"},
		{`{"a":1}`, "$[", "unmatched '['"},
		{`{"a":1}`, "$..a", "invalid field character"},
	}
	for _, tc := range cases {
		t.Run(tc.path+"/"+tc.err, func(t *testing.T) {
			_, err := JSONProcess(tc.doc, tc.path, "")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.err)
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.err)
			}
		})
	}
}

func TestJSONProcessInvalidJSON(t *testing.T) {
	_, err := JSONProcess("not json", "", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestJSONProcessParseOp(t *testing.T) {
	got, err := JSONProcess(`{"a":1}`, "", "parse")
	if err != nil {
		t.Fatalf("JSONProcess error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want map", got)
	}
	if m["a"] != float64(1) {
		t.Errorf("a = %v, want 1", m["a"])
	}
}

func TestJSONProcessorExecutorSuccess(t *testing.T) {
	a := NewJSONProcessorAction()
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"json": `{"x": {"y": 42}}`,
		"path": "$.x.y",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Result != float64(42) {
		t.Errorf("Result = %v, want 42", res.Result)
	}
}

func TestJSONProcessorExecutorMissingJSON(t *testing.T) {
	a := NewJSONProcessorAction()
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing json")
	}
}

func TestJSONProcessorExecutorBadJSON(t *testing.T) {
	a := NewJSONProcessorAction()
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"json": "broken",
	})
	if res.OK {
		t.Errorf("expected OK=false for invalid JSON")
	}
}

func TestJSONProcessorActionRegistration(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewJSONProcessorAction()); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(JSONProcessorActionID) {
		t.Errorf("registry missing %q", JSONProcessorActionID)
	}
}
