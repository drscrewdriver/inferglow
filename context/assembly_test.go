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

// TestAssemblyManagerFactoryAndMode covers the factory wiring path.
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
	if am.engine == nil {
		t.Error("factory must create embedded HybridManager engine")
	}
}

// TestAssemblyManagerIngestBuildContextRoundTrip covers the delegated path:
// ingested steps survive into BuildContext via the hybrid engine.
func TestAssemblyManagerIngestBuildContextRoundTrip(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.LongMem.Enabled = false

	mgr, err := NewAssemblyManager(cfg, store)
	if err != nil {
		t.Fatalf("NewAssemblyManager: %v", err)
	}

	if err := mgr.Ingest(StepRecord{Type: "user", Content: "hello"}); err != nil {
		t.Fatalf("Ingest user: %v", err)
	}
	if err := mgr.Ingest(StepRecord{Type: "reasoning", Content: "think"}); err != nil {
		t.Fatalf("Ingest reasoning: %v", err)
	}

	blocks, err := mgr.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	// Hybrid engine produces blocks (at minimum the 2 steps + hint).
	if len(blocks) < 2 {
		t.Fatalf("BuildContext len = %d, want >= 2", len(blocks))
	}

	// Stats reflect the wired store.
	stats := mgr.Stats()
	if stats.TotalSteps != 2 || stats.ActiveSteps != 2 {
		t.Errorf("Stats = %+v, want TotalSteps=2 ActiveSteps=2", stats)
	}
}

// TestAssemblyManagerSearchDelegated verifies Search delegates to hybrid engine.
func TestAssemblyManagerSearchDelegated(t *testing.T) {
	store := newFakeStore()
	mgr, err := NewAssemblyManager(DefaultConfig(), store)
	if err != nil {
		t.Fatalf("NewAssemblyManager: %v", err)
	}
	// Should not error (delegates to hybrid which returns empty results).
	_, err = mgr.Search(context.Background(), SearchQuery{Query: "x"})
	if err != nil {
		t.Errorf("Search should delegate to engine, got error: %v", err)
	}
}

// TestAssemblyManagerRegistrySwitch exercises the Registry hot-switch path.
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
	if err := hy.Ingest(StepRecord{Type: "user", Content: "hello"}); err != nil {
		t.Fatalf("hybrid ingest: %v", err)
	}

	mgr, err := reg.SwitchMode(ModeHybrid, ModeAssembly, DefaultConfig(), store)
	if err != nil {
		t.Fatalf("SwitchMode hybrid->assembly: %v", err)
	}
	if mgr.Mode() != ModeAssembly {
		t.Errorf("after switch Mode() = %q, want %q", mgr.Mode(), ModeAssembly)
	}

	if _, err := reg.SwitchMode(ModeAssembly, Mode("nonexistent"), DefaultConfig(), store); err == nil {
		t.Errorf("SwitchMode to unregistered mode should return error")
	}
}

// --- A-2: Two-phase assembly tests ---

func TestAssembleSetupProducesL1ToL5(t *testing.T) {
	store := newFakeStore()
	mgr, _ := NewAssemblyManager(DefaultConfig(), store)
	am := mgr.(*AssemblyManager)

	out, err := am.AssembleSetup(context.Background(), &SetupRequest{
		SafetyPolicy:    "no harm",
		Identity:        "I am an agent",
		Protocol:        "follow rules",
		TaskDescription: "build a widget",
		Prohibitions:    []string{"no rm", "no sudo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Layers) != 5 {
		t.Fatalf("expected 5 layers from Setup, got %d", len(out.Layers))
	}
	// Verify L1-L5 stable.
	for _, l := range out.Layers {
		if !l.Stable {
			t.Errorf("layer %d should be stable", l.ID)
		}
	}
	if out.Layers[0].Content != "no harm" {
		t.Errorf("L1 content = %q", out.Layers[0].Content)
	}
	if out.Layers[3].Content != "build a widget" {
		t.Errorf("L4 content = %q", out.Layers[3].Content)
	}
}

func TestAssembleExecuteProducesL1ToL9(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.LongMem.Enabled = false
	cfg.Backtrack.Enabled = false
	mgr, _ := NewAssemblyManager(cfg, store)
	am := mgr.(*AssemblyManager)

	// Setup first.
	_, _ = am.AssembleSetup(context.Background(), &SetupRequest{
		SafetyPolicy:    "safe",
		Identity:        "agent",
		Protocol:        "proto",
		TaskDescription: "task bg",
		Prohibitions:    []string{"no rm"},
	})

	// Ingest some steps.
	_ = am.Ingest(StepRecord{Type: "user", Content: "hello"})
	_ = am.Ingest(StepRecord{Type: "reasoning", Content: "think"})

	out, err := am.AssembleExecute(context.Background(), &ExecuteRequest{RoundID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	// Should have L1-L5 (stable) + at least L7 (compressed hist) + L9 (hint).
	if len(out.Layers) < 7 {
		t.Fatalf("expected >= 7 layers from Execute, got %d: %+v", len(out.Layers), out.Layers)
	}
	// L1 should be from cache.
	if out.Layers[0].ID != LayerSystemSafety || out.Layers[0].Content != "safe" {
		t.Errorf("L1 mismatch: %+v", out.Layers[0])
	}
}

func TestAssembleExecuteRecordsAudit(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.LongMem.Enabled = false
	mgr, _ := NewAssemblyManager(cfg, store)
	am := mgr.(*AssemblyManager)

	_, _ = am.AssembleSetup(context.Background(), &SetupRequest{TaskDescription: "bg"})
	_ = am.Ingest(StepRecord{Type: "user", Content: "x"})
	_, _ = am.AssembleExecute(context.Background(), &ExecuteRequest{RoundID: "round-1"})

	am.audit.mu.Lock()
	count := len(am.audit.entries)
	last := am.audit.entries[count-1]
	am.audit.mu.Unlock()

	if count < 2 {
		t.Fatalf("expected >= 2 audit entries (setup + execute), got %d", count)
	}
	if last.Phase != "execute" || last.RoundID != "round-1" {
		t.Errorf("last audit = %+v", last)
	}
}

func TestInvalidateLayerForcesRebuild(t *testing.T) {
	store := newFakeStore()
	mgr, _ := NewAssemblyManager(DefaultConfig(), store)
	am := mgr.(*AssemblyManager)

	_, _ = am.AssembleSetup(context.Background(), &SetupRequest{
		SafetyPolicy: "policy v1",
	})

	// L1 populated.
	l, err := am.GetLayer(LayerSystemSafety)
	if err != nil || l.Content != "policy v1" {
		t.Fatalf("GetLayer L1: %v, %v", l, err)
	}

	// Invalidate L1.
	am.InvalidateLayer(LayerSystemSafety)
	if _, err := am.GetLayer(LayerSystemSafety); err == nil {
		t.Error("GetLayer after Invalidate should return error")
	}
}

func TestBuildContextInterfaceCompat(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.LongMem.Enabled = false

	// Use via interface only.
	var mgr ContextManager
	mgr, _ = NewAssemblyManager(cfg, store)

	_ = mgr.Ingest(StepRecord{Type: "user", Content: "test"})
	blocks, err := mgr.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		t.Error("BuildContext via interface should produce blocks")
	}
}
