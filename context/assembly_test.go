package contextmgr

import (
	"context"
	"testing"
)

// TestAssemblyManagerImplementsInterface asserts the skeleton satisfies the
// frozen ContextManager interface at compile time.
func TestAssemblyManagerImplementsInterface(t *testing.T) {
	var _ ContextManager = (*AssemblyManager)(nil)
}

// TestAssemblyManagerFactoryAndMode covers the factory wiring path: the
// manager is created, exposes ModeAssembly, and wires the shared step store.
func TestAssemblyManagerFactoryAndMode(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()

	mgr, err := NewAssemblyManager(cfg, store)
	if err != nil {
		t.Fatalf("NewAssemblyManager: %v", err)
	}
	if mgr.Mode() != ModeAssembly {
		t.Errorf("Mode() = %q, want %q", mgr.Mode(), ModeAssembly)
	}
	am, ok := mgr.(*AssemblyManager)
	if !ok || am.store != store {
		t.Errorf("factory must wire the shared step store")
	}
}

// TestAssemblyManagerIngestBuildContextRoundTrip covers the assembly path:
// ingested steps survive into BuildContext at L0 (skeleton render).
func TestAssemblyManagerIngestBuildContextRoundTrip(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()

	mgr, err := NewAssemblyManager(cfg, store)
	if err != nil {
		t.Fatalf("NewAssemblyManager: %v", err)
	}

	if err := mgr.Ingest(StepRecord{StepID: 1, Type: "user", Content: "hello"});
		err != nil {
		t.Fatalf("Ingest user: %v", err)
	}
	if err := mgr.Ingest(StepRecord{StepID: 2, Type: "reasoning", Content: "think"}); err != nil {
		t.Fatalf("Ingest reasoning: %v", err)
	}

	blocks, err := mgr.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("BuildContext len = %d, want 2 (assembly render must surface all L0 steps)", len(blocks))
	}
	if blocks[0].Content != "hello" || blocks[1].Content != "think" {
		t.Errorf("unexpected block order/content: %+v", blocks)
	}

	// Stats reflect the wired store.
	stats := mgr.Stats()
	if stats.TotalSteps != 2 || stats.ActiveSteps != 2 {
		t.Errorf("Stats = %+v, want TotalSteps=2 ActiveSteps=2", stats)
	}
}

// TestAssemblyManagerSearchNotImplemented documents the wp-a1 empty stubs:
// unsupported paths return explicit errors rather than panicking or silently
// returning garbage.
func TestAssemblyManagerSearchNotImplemented(t *testing.T) {
	store := newFakeStore()
	mgr, err := NewAssemblyManager(DefaultConfig(), store)
	if err != nil {
		t.Fatalf("NewAssemblyManager: %v", err)
	}
	_, err = mgr.Search(context.Background(), SearchQuery{Query: "x"})
	if err == nil {
		t.Errorf("Search should return not-implemented error at wp-a1")
	}
}

// TestAssemblyManagerRegistrySwitch exercises the Registry hot-switch path
// used by cli/memory_bridge.go SwitchMode: assembly shares the step store and
// is selectable as a target mode.
func TestAssemblyManagerRegistrySwitch(t *testing.T) {
	store := newFakeStore()
	reg := NewRegistry()
	reg.Register(ModeAssembly, func(cfg Config, s StepStoreLike) (ContextManager, error) {
		return NewAssemblyManager(cfg, s)
	})
	reg.Register(ModeHybrid, func(cfg Config, s StepStoreLike) (ContextManager, error) {
		return NewHybridManager(cfg, s)
	})

	hy, err := NewHybridManager(DefaultConfig(), store)
	if err != nil {
		t.Fatalf("NewHybridManager: %v", err)
	}
	if err := hy.Ingest(StepRecord{StepID: 1, Type: "user", Content: "hello"}); err != nil {
		t.Fatalf("hybrid ingest: %v", err)
	}

	// Switch hybrid → assembly; store content must survive the switch.
	mgr, err := reg.SwitchMode(ModeHybrid, ModeAssembly, DefaultConfig(), store)
	if err != nil {
		t.Fatalf("SwitchMode hybrid->assembly: %v", err)
	}
	if mgr.Mode() != ModeAssembly {
		t.Errorf("after switch Mode() = %q, want %q", mgr.Mode(), ModeAssembly)
	}

	// Unregistered mode must fail cleanly (defensive).
	if _, err := reg.SwitchMode(ModeAssembly, Mode("nonexistent"), DefaultConfig(), store); err == nil {
		t.Errorf("SwitchMode to unregistered mode should return error")
	} else if "" == err.Error() {
		t.Errorf("unexpected empty error: %v", err)
	}
}