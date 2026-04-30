package action

import (
	"context"
	"errors"
	"testing"
)

// mockExecutor is a minimal ActionExecutor used to exercise the registry
// without depending on LocalFunctionExecutor.
type mockExecutor struct {
	result *ActionResult
	err    error
}

func (m *mockExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result == nil {
		return &ActionResult{OK: true, Status: "success", Result: nil}, nil
	}
	return m.result, nil
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	a := &Action{
		Name:        "greet",
		Description: "say hello",
		Schema:      map[string]any{"type": "object"},
		Executor:    &mockExecutor{result: &ActionResult{OK: true, Status: "success", Result: "hi"}},
	}
	if err := r.Register(a); err != nil {
		t.Fatalf("Register returned unexpected error: %v", err)
	}
	got, err := r.Get("greet")
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if got == nil || got.Name != "greet" {
		t.Fatalf("Get returned wrong action: %+v", got)
	}
	if got.Description != "say hello" {
		t.Errorf("Description mismatch: got %q", got.Description)
	}
}

func TestDuplicateRegister(t *testing.T) {
	r := NewRegistry()
	a := &Action{
		Name:     "dup",
		Executor: &mockExecutor{},
	}
	if err := r.Register(a); err != nil {
		t.Fatalf("first Register returned unexpected error: %v", err)
	}
	err := r.Register(&Action{Name: "dup", Executor: &mockExecutor{}})
	if !errors.Is(err, ErrActionAlreadyRegistered) {
		t.Fatalf("expected ErrActionAlreadyRegistered, got %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(nil); err == nil {
		t.Fatal("expected error registering nil action")
	}
	if err := r.Register(&Action{Name: "", Executor: &mockExecutor{}}); err == nil {
		t.Fatal("expected error registering action with empty name")
	}
	if err := r.Register(&Action{Name: "no-exec"}); err == nil {
		t.Fatal("expected error registering action with nil executor")
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("missing")
	if !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("expected ErrActionNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if err := r.Register(&Action{Name: name, Executor: &mockExecutor{}}); err != nil {
			t.Fatalf("Register(%q) returned error: %v", name, err)
		}
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d (%v)", len(got), got)
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestListEmpty(t *testing.T) {
	r := NewRegistry()
	got := r.List()
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestExecuteNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "ghost", nil)
	if !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("expected ErrActionNotFound, got %v", err)
	}
}

func TestExecuteSuccess(t *testing.T) {
	r := NewRegistry()
	wantResult := &ActionResult{OK: true, Status: "success", Result: 42}
	if err := r.Register(&Action{
		Name:     "compute",
		Executor: &mockExecutor{result: wantResult},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := r.Execute(context.Background(), "compute", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !got.OK {
		t.Fatalf("expected OK=true, got %+v", got)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want %q", got.Status, "success")
	}
	if got.Result != 42 {
		t.Errorf("Result = %v, want 42", got.Result)
	}
}

func TestExecuteError(t *testing.T) {
	r := NewRegistry()
	execErr := errors.New("boom")
	if err := r.Register(&Action{
		Name:     "failing",
		Executor: &mockExecutor{err: execErr},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	got, err := r.Execute(context.Background(), "failing", nil)
	if err != nil {
		t.Fatalf("Execute returned unexpected infrastructure error: %v", err)
	}
	if got.OK {
		t.Errorf("expected OK=false, got %+v", got)
	}
	if got.Status != "error" {
		t.Errorf("Status = %q, want %q", got.Status, "error")
	}
	if got.Error != execErr.Error() {
		t.Errorf("Error = %q, want %q", got.Error, execErr.Error())
	}
}
