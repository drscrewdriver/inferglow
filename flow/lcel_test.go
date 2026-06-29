// Copyright 2026 InferGlow Authors

package flow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestLCEL_Pipe_Invoke(t *testing.T) {
	upper := func(_ context.Context, input any) (any, error) {
		return strings.ToUpper(input.(string)), nil
	}
	exclaim := func(_ context.Context, input any) (any, error) {
		return input.(string) + "!", nil
	}

	chain := LCEL("upper", upper).Pipe("exclaim", exclaim)

	if chain.Len() != 2 {
		t.Fatalf("expected 2 steps, got %d", chain.Len())
	}
	if got, want := chain.Names(), []string{"upper", "exclaim"}; got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("names = %v, want %v", got, want)
	}

	result, err := chain.Invoke(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.(string), "HELLO!"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
}

func TestLCEL_Invoke_Error(t *testing.T) {
	fail := func(_ context.Context, _ any) (any, error) {
		return nil, fmt.Errorf("boom")
	}
	chain := LCEL("fail", fail)
	_, err := chain.Invoke(context.Background(), "input")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error containing 'boom', got %v", err)
	}
}

func TestLCEL_Build_Execute(t *testing.T) {
	double := func(_ context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	}
	addTen := func(_ context.Context, input any) (any, error) {
		return input.(int) + 10, nil
	}

	chain := LCEL("double", double).Pipe("addTen", addTen)
	f := chain.Build()

	exec := f.Execute(context.Background(), 5)
	if len(exec.State.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", exec.State.Errors)
	}
	if got, want := exec.State.Result.(int), 20; got != want {
		t.Fatalf("result = %d, want %d", got, want)
	}
}

func TestMapChain(t *testing.T) {
	double := func(_ context.Context, input any) (any, error) {
		return input.(int) * 2, nil
	}
	mapFn := MapChain("double", double)

	result, err := mapFn(context.Background(), []any{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	got := result.([]any)
	if len(got) != 3 || got[0].(int) != 2 || got[1].(int) != 4 || got[2].(int) != 6 {
		t.Fatalf("got %v, want [2 4 6]", got)
	}
}

func TestMapChain_InvalidInput(t *testing.T) {
	mapFn := MapChain("test", func(_ context.Context, input any) (any, error) {
		return input, nil
	})
	_, err := mapFn(context.Background(), "not a slice")
	if err == nil {
		t.Fatal("expected error for non-slice input")
	}
}

func TestBranchChain(t *testing.T) {
	isPositive := func(input any) bool { return input.(int) > 0 }
	posFn := func(_ context.Context, input any) (any, error) { return "positive", nil }
	negFn := func(_ context.Context, input any) (any, error) { return "non-positive", nil }

	branch := BranchChain(isPositive, posFn, negFn)

	r1, err := branch(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if r1.(string) != "positive" {
		t.Fatalf("got %q, want 'positive'", r1)
	}

	r2, err := branch(context.Background(), -1)
	if err != nil {
		t.Fatal(err)
	}
	if r2.(string) != "non-positive" {
		t.Fatalf("got %q, want 'non-positive'", r2)
	}
}

func TestParallelChain(t *testing.T) {
	chainA := LCEL("a", func(_ context.Context, input any) (any, error) {
		return input.(int) + 1, nil
	})
	chainB := LCEL("b", func(_ context.Context, input any) (any, error) {
		return input.(int) * 10, nil
	})

	par := ParallelChain(chainA, chainB)
	result, err := par(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["a"].(int) != 6 {
		t.Fatalf("a = %d, want 6", m["a"])
	}
	if m["b"].(int) != 50 {
		t.Fatalf("b = %d, want 50", m["b"])
	}
}

func TestConstChain(t *testing.T) {
	fn := ConstChain(42)
	result, err := fn(context.Background(), "anything")
	if err != nil {
		t.Fatal(err)
	}
	if result.(int) != 42 {
		t.Fatalf("got %d, want 42", result)
	}
}

func TestChain_Empty(t *testing.T) {
	chain := &Chain{}
	if chain.Len() != 0 {
		t.Fatalf("expected 0 steps, got %d", chain.Len())
	}
	// Invoke on empty chain should return the input unchanged.
	result, err := chain.Invoke(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result.(string) != "hello" {
		t.Fatalf("got %q, want 'hello'", result)
	}
}
