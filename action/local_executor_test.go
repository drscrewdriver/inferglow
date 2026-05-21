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
	"context"
	"errors"
	"testing"
)

// TestInput / TestOutput are the shared payload types for the signature tests.
type TestInput struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type TestOutput struct {
	Greeting string `json:"greeting"`
}

func testFn1(ctx context.Context, in TestInput) (TestOutput, error) {
	return TestOutput{Greeting: "Hello " + in.Name}, nil
}

func testFn2(in TestInput) (TestOutput, error) {
	return TestOutput{Greeting: "Hi " + in.Name}, nil
}

func testFn3(ctx context.Context, in TestInput) TestOutput {
	return TestOutput{Greeting: "Hey " + in.Name}
}

func testFnErr(ctx context.Context, in TestInput) (TestOutput, error) {
	return TestOutput{}, errors.New("intentional failure")
}

func testFnPanic(ctx context.Context, in TestInput) (TestOutput, error) {
	panic("boom")
}

func testFnUnsupported() {
	// no-op — unsupported signature
}

func TestNewSig1(t *testing.T) {
	a, err := New("sig1", "func(ctx, InputT) (OutputT, error)", testFn1)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil action")
	}
	if a.Name != "sig1" {
		t.Errorf("Name = %q", a.Name)
	}
	if a.Executor == nil {
		t.Fatal("Executor is nil")
	}
}

func TestNewSig2(t *testing.T) {
	a, err := New("sig2", "func(InputT) (OutputT, error)", testFn2)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if a.Name != "sig2" {
		t.Errorf("Name = %q", a.Name)
	}
}

func TestNewSig3(t *testing.T) {
	a, err := New("sig3", "func(ctx, InputT) OutputT", testFn3)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if a.Name != "sig3" {
		t.Errorf("Name = %q", a.Name)
	}
}

func TestNewUnsupportedSignature(t *testing.T) {
	_, err := New("bad", "func() void", testFnUnsupported)
	if !errors.Is(err, ErrUnsupportedFunctionSignature) {
		t.Fatalf("expected ErrUnsupportedFunctionSignature, got %v", err)
	}
}

func TestNewNilFunction(t *testing.T) {
	_, err := New("nil", "nil fn", nil)
	if !errors.Is(err, ErrUnsupportedFunctionSignature) {
		t.Fatalf("expected ErrUnsupportedFunctionSignature, got %v", err)
	}
}

func TestNewNonFunction(t *testing.T) {
	_, err := New("notfunc", "string value", "i am a string")
	if !errors.Is(err, ErrUnsupportedFunctionSignature) {
		t.Fatalf("expected ErrUnsupportedFunctionSignature, got %v", err)
	}
}

func TestLocalExecutorExecuteSuccess(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		fn     any
		prefix string
	}{
		{"sig1", testFn1, "Hello"},
		{"sig2", testFn2, "Hi"},
		{"sig3", testFn3, "Hey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New(tc.name, tc.name, tc.fn)
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			res, err := a.Executor.Execute(ctx, map[string]any{
				"name": "Alice",
				"age":  30,
			})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if !res.OK {
				t.Fatalf("expected OK=true, got %+v", res)
			}
			if res.Status != "success" {
				t.Errorf("Status = %q, want %q", res.Status, "success")
			}
			out, ok := res.Result.(TestOutput)
			if !ok {
				t.Fatalf("Result is not TestOutput: %T (%v)", res.Result, res.Result)
			}
			if want := tc.prefix + " Alice"; out.Greeting != want {
				t.Errorf("Greeting = %q, want %q", out.Greeting, want)
			}
		})
	}
}

func TestExecuteFnError(t *testing.T) {
	a, err := New("err-fn", "returns error", testFnErr)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "Bob",
		"age":  25,
	})
	if err != nil {
		t.Fatalf("Execute returned infrastructure error: %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error != "intentional failure" {
		t.Errorf("Error = %q, want %q", res.Error, "intentional failure")
	}
}

func TestExecutePanic(t *testing.T) {
	a, err := New("panic-fn", "panics", testFnPanic)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"name": "Carol",
		"age":  40,
	})
	if err != nil {
		t.Fatalf("Execute returned infrastructure error: %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if res.Status != "error" {
		t.Errorf("Status = %q, want %q", res.Status, "error")
	}
	if res.Error == "" {
		t.Errorf("expected non-empty Error, got empty")
	}
}

func TestSchemaGeneration(t *testing.T) {
	a, err := New("schema-test", "schema gen", testFn1)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if a.Schema == nil {
		t.Fatal("Schema is nil")
	}
	if a.Schema["type"] != "object" {
		t.Errorf("Schema type = %v, want %q", a.Schema["type"], "object")
	}
	props, ok := a.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Schema properties not a map: %T", a.Schema["properties"])
	}
	if _, ok := props["name"]; !ok {
		t.Errorf("Schema missing field %q; props: %+v", "name", props)
	}
	if _, ok := props["age"]; !ok {
		t.Errorf("Schema missing field %q; props: %+v", "age", props)
	}
	nameSchema, _ := props["name"].(map[string]any)
	if nameSchema == nil || nameSchema["type"] != "string" {
		t.Errorf("name schema not string: %+v", nameSchema)
	}
	ageSchema, _ := props["age"].(map[string]any)
	if ageSchema == nil || ageSchema["type"] != "integer" {
		t.Errorf("age schema not integer: %+v", ageSchema)
	}
	required, ok := a.Schema["required"].([]string)
	if !ok {
		t.Fatalf("Schema required not a []string: %T", a.Schema["required"])
	}
	if len(required) != 2 {
		t.Errorf("expected 2 required fields, got %v", required)
	}
}

func TestSchemaGenerationForNonStruct(t *testing.T) {
	// A function taking a map[string]string (non-struct) should fall back
	// to the placeholder {"type": "object"} schema.
	fn := func(in map[string]string) (string, error) { return in["k"], nil }
	a, err := New("map-input", "map input", fn)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if a.Schema["type"] != "object" {
		t.Errorf("expected placeholder schema, got %+v", a.Schema)
	}
}

func TestExecuteIntegrationWithRegistry(t *testing.T) {
	r := NewRegistry()
	a, err := New("integration", "integration test", testFn1)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := r.Register(a); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	res, err := r.Execute(context.Background(), "integration", map[string]any{
		"name": "Dave",
		"age":  50,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	out, ok := res.Result.(TestOutput)
	if !ok {
		t.Fatalf("Result is not TestOutput: %T", res.Result)
	}
	if want := "Hello Dave"; out.Greeting != want {
		t.Errorf("Greeting = %q, want %q", out.Greeting, want)
	}
}
