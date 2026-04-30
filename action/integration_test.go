package action

import (
	"context"
	"errors"
	"testing"
)

// AddInput / AddOutput are the payload types used by the P1 integration
// tests for the Action Runtime. They are intentionally distinct from the
// TestInput / TestOutput types defined in local_executor_test.go so the
// integration test exercises its own end-to-end schema path.
type AddInput struct {
	A int `json:"a"`
	B int `json:"b"`
}

type AddOutput struct {
	Sum int `json:"sum"`
}

// TestP1Integration_ActionRegistryWithLocalFunction exercises the full
// Action Runtime MVP happy path: define a typed Go function, wrap it as an
// Action via action.New, register it with an ActionRegistry, and execute
// it through the registry with a loosely-typed map[string]any input.
//
// Verifies that:
//   - action.New accepts a func(ctx, AddInput) (AddOutput, error) signature
//   - Registry.Register stores the action
//   - Registry.Execute returns ActionResult{OK: true, Status: "success"}
//   - Result is the strongly-typed AddOutput with the correct Sum
func TestP1Integration_ActionRegistryWithLocalFunction(t *testing.T) {
	// 1. Define the underlying Go function.
	fn := func(ctx context.Context, in AddInput) (AddOutput, error) {
		return AddOutput{Sum: in.A + in.B}, nil
	}

	// 2. Wrap it as an Action via action.New.
	a, err := New("add", "加法", fn)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if a.Name != "add" {
		t.Errorf("a.Name = %q, want \"add\"", a.Name)
	}
	if a.Description != "加法" {
		t.Errorf("a.Description = %q, want \"加法\"", a.Description)
	}

	// 3. Register it with a fresh ActionRegistry.
	registry := NewRegistry()
	if err := registry.Register(a); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 4. Execute via the registry with a map[string]any input.
	result, err := registry.Execute(context.Background(), "add", map[string]any{
		"a": 3,
		"b": 5,
	})
	if err != nil {
		t.Fatalf("Execute returned infrastructure error: %v", err)
	}

	// 5. Verify ActionResult.OK == true.
	if !result.OK {
		t.Fatalf("expected OK=true, got false; Status=%q Error=%q", result.Status, result.Error)
	}
	if result.Status != "success" {
		t.Errorf("Status = %q, want \"success\"", result.Status)
	}

	// 6. Verify Result is strongly-typed AddOutput with Sum == 8.
	out, ok := result.Result.(AddOutput)
	if !ok {
		t.Fatalf("expected Result to be AddOutput, got %T (%v)", result.Result, result.Result)
	}
	if out.Sum != 8 {
		t.Errorf("expected Sum=8, got %d", out.Sum)
	}
}

// TestP1Integration_ActionExecuteError verifies that when the wrapped
// function returns a non-nil error, the ActionRegistry.Execute path
// surfaces it as a structured ActionResult with OK=false.
func TestP1Integration_ActionExecuteError(t *testing.T) {
	// 1. Define a function that always returns an error.
	fn := func(ctx context.Context, in AddInput) (AddOutput, error) {
		return AddOutput{}, errors.New("intentional integration failure")
	}

	// 2. Wrap and register it.
	a, err := New("failing", "失败函数", fn)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	registry := NewRegistry()
	if err := registry.Register(a); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 3. Execute — the wrapped error should become a structured result.
	result, err := registry.Execute(context.Background(), "failing", map[string]any{
		"a": 1,
		"b": 2,
	})
	if err != nil {
		t.Fatalf("Execute returned infrastructure error: %v", err)
	}

	// 4. Verify ActionResult.OK == false.
	if result.OK {
		t.Errorf("expected OK=false, got true")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want \"error\"", result.Status)
	}
	if result.Error != "intentional integration failure" {
		t.Errorf("Error = %q, want \"intentional integration failure\"", result.Error)
	}
}
