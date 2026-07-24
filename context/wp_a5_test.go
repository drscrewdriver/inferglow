package contextmgr

import (
	"context"
	"strings"
	"testing"
)

// wpA5Store wraps fakeStore to provide configurable HotFacts results.
type wpA5Store struct {
	*fakeStore
	hotFacts []L2Record
}

func (s *wpA5Store) HotFacts(int, float64) ([]L2Record, error) { return s.hotFacts, nil }

func newWpA5Store() *wpA5Store {
	return &wpA5Store{fakeStore: newFakeStore()}
}

// TestZone2NumericRankedFormat verifies A-8: hot facts are rendered ranked by
// strength descending with #N format, no bullets.
func TestZone2NumericRankedFormat(t *testing.T) {
	store := newWpA5Store()
	// Two L2 records with different strengths via refs.
	store.l2[1] = L2Record{StepID: 1, Facts: []string{"weak fact"}}
	store.l2[2] = L2Record{StepID: 2, Facts: []string{"strong fact"}}
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "raw1"}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "raw2"}
	store.refs[1] = RefRecord{StepID: 1, Level: 2, Strength: 1.5}
	store.refs[2] = RefRecord{StepID: 2, Level: 2, Strength: 2.3}
	store.hotFacts = []L2Record{store.l2[1], store.l2[2]}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	cfg.LongMem.Enabled = false
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	blocks, err := h.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatal(err)
	}

	// Find the Zone 2 ranked block (StepID -4).
	var zone2 *RenderedBlock
	for i := range blocks {
		if blocks[i].StepID == -4 {
			zone2 = &blocks[i]
			break
		}
	}
	if zone2 == nil {
		t.Fatal("expected Zone 2 ranked block (StepID -4)")
	}

	// Must contain ranked format.
	if !strings.Contains(zone2.Content, "#1 (strength: 2.3) strong fact") {
		t.Errorf("expected #1 to be strongest; got:\n%s", zone2.Content)
	}
	if !strings.Contains(zone2.Content, "#2 (strength: 1.5) weak fact") {
		t.Errorf("expected #2 to be weakest; got:\n%s", zone2.Content)
	}
	// #1 must appear before #2.
	idx1 := strings.Index(zone2.Content, "#1")
	idx2 := strings.Index(zone2.Content, "#2")
	if idx1 > idx2 {
		t.Error("expected #1 before #2 (descending strength order)")
	}
	// No bullets.
	if strings.Contains(zone2.Content, "• ") {
		t.Errorf("Zone 2 must not contain bullets; got:\n%s", zone2.Content)
	}
}

// TestBuildContextSkipsTransient verifies A-12: transient steps are excluded.
func TestBuildContextSkipsTransient(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "tool", Content: "tool output", Transient: true, TransientScope: "tool_call", TransientRound: 1}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "real content"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 5 // both in tail zone
	cfg.LongMem.Enabled = false
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	blocks, err := h.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatal(err)
	}

	for _, b := range blocks {
		if b.StepID == 1 {
			t.Error("transient step 1 must not appear in BuildContext output")
		}
	}
	// Step 2 must be present.
	found := false
	for _, b := range blocks {
		if b.StepID == 2 {
			found = true
		}
	}
	if !found {
		t.Error("non-transient step 2 must appear in BuildContext output")
	}
}

// TestMarkTransientAndClear verifies the full lifecycle: mark → skip → clear.
func TestMarkTransientAndClear(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "tool", Content: "fragment"}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: "keep"}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 5
	cfg.LongMem.Enabled = false
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	// Before mark: step 1 visible.
	blocks, _ := h.BuildContext(context.Background(), 128000)
	if !hasStep(blocks, 1) {
		t.Fatal("step 1 should be visible before MarkTransient")
	}

	// Mark transient.
	if err := h.MarkTransient(1, "tool_call", 1); err != nil {
		t.Fatal(err)
	}

	// After mark: step 1 hidden.
	blocks, _ = h.BuildContext(context.Background(), 128000)
	if hasStep(blocks, 1) {
		t.Error("step 1 should be hidden after MarkTransient")
	}

	// Clear stale transients (round 2 > round 1).
	count, err := h.ClearStaleTransients(2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 cleared, got %d", count)
	}

	// Step 1 removed from active set entirely.
	ids, _ := store.AllActiveStepIDs()
	for _, id := range ids {
		if id == 1 {
			t.Error("step 1 should be removed from active set after ClearStaleTransients")
		}
	}
}

func hasStep(blocks []RenderedBlock, stepID int) bool {
	for _, b := range blocks {
		if b.StepID == stepID {
			return true
		}
	}
	return false
}
