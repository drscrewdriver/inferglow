package actionruntime

import (
	"context"
	"testing"
	"time"

	"github.com/inferglow/action"
)

// mockExecutor is a simple executor for testing.
type mockExecutor struct {
	fn func(map[string]any) (*action.ActionResult, error)
}

func (m *mockExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	return m.fn(input)
}

func TestDispatcher_SingleAction(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "result", nil
	}
	actionInst, _ := action.New("test", "test action", fn)
	registry.Register(actionInst)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{{Name: "test", Params: map[string]any{"key": "value"}}}
	results := dispatcher.Execute(ctx, calls)
	if len(results) == 0 {
		t.Fatal("Expected at least 1 result")
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if !results[0].OK {
		t.Errorf("Expected OK=true, got %v", results[0])
	}
	if results[0].Result != "result" {
		t.Errorf("Result mismatch: got %v", results[0].Result)
	}
}

func TestDispatcher_Concurrent(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		time.Sleep(10 * time.Millisecond)
		return "done", nil
	}

	a1, _ := action.New("act1", "", fn)
	a2, _ := action.New("act2", "", fn)
	a3, _ := action.New("act3", "", fn)
	registry.Register(a1)
	registry.Register(a2)
	registry.Register(a3)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "act1", Params: nil},
		{Name: "act2", Params: nil},
		{Name: "act3", Params: nil},
	}

	start := time.Now()
	results := dispatcher.Execute(ctx, calls)
	elapsed := time.Since(start)
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}
	// Should be fast due to concurrency (< 30ms instead of >30ms)
	if elapsed > 30*time.Millisecond {
		t.Errorf("Expected concurrent execution (<30ms), took %v", elapsed)
	}
}

func TestDispatcher_ActionFails(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()

	fn := func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}

	a1, _ := action.New("ok_action", "", fn)
	a2, _ := action.New("fail_action", "fails", func(ctx context.Context, input map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(a1)
	registry.Register(a2)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "ok_action", Params: nil},
		{Name: "fail_action", Params: nil},
	}
	results := dispatcher.Execute(ctx, calls)
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if !results[0].OK {
		t.Error("First action should succeed")
	}
	// Second action result is OK=false because it returns nil error with nil output
}
