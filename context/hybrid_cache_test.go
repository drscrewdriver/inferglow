package contextmgr

import (
	"sync"
	"testing"
)

func TestUpdateCacheBudget_RaisesSweetSpot(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 100 // small initial value so cache feedback exceeds it
	cfg.TailKeepSteps = 3
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	// Add some steps to simulate existing context.
	_ = h.Ingest(StepRecord{StepID: 1, Type: "reasoning", TokenCount: 100})
	_ = h.Ingest(StepRecord{StepID: 2, Type: "tool", TokenCount: 200})

	before := h.sweetSpotTokens

	// Report cached tokens — should raise sweetSpotTokens.
	h.UpdateCacheBudget(500)

	h.toleranceMu.Lock()
	after := h.sweetSpotTokens
	tol := h.sweetSpotTolerance
	h.toleranceMu.Unlock()

	if after <= before {
		t.Errorf("sweetSpotTokens should increase: before=%d, after=%d", before, after)
	}
	// effective = cachedTokens(500) + estimateTotalTokens() >= 500
	// cap = 1.5 * 100 = 150, so after should be capped at 150.
	cap := int(float64(cfg.SweetSpotTokens) * 1.5)
	if after > cap {
		t.Errorf("sweetSpotTokens = %d, exceeds cap %d", after, cap)
	}
	if tol <= 1.0 {
		t.Errorf("tolerance = %f, expected > 1.0", tol)
	}
}

func TestUpdateCacheBudget_CapsAt1_5x(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 1000
	cfg.TailKeepSteps = 3
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	// Report a very large cached_tokens value.
	h.UpdateCacheBudget(100000)

	h.toleranceMu.Lock()
	after := h.sweetSpotTokens
	h.toleranceMu.Unlock()

	cap := int(float64(cfg.SweetSpotTokens) * 1.5)
	if after > cap {
		t.Errorf("sweetSpotTokens = %d, should be capped at %d (1.5x original)", after, cap)
	}
}

func TestUpdateCacheBudget_NoOpWhenZero(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 1000
	cfg.TailKeepSteps = 3
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	before := h.sweetSpotTokens
	h.UpdateCacheBudget(0)
	after := h.sweetSpotTokens

	if before != after {
		t.Errorf("sweetSpotTokens changed from %d to %d with cachedTokens=0", before, after)
	}
}

func TestUpdateCacheBudget_ConcurrentSafe(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.SweetSpotTokens = 1000
	cfg.TailKeepSteps = 3
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.UpdateCacheBudget(n * 100)
		}(i)
	}
	wg.Wait()

	// Just verify no race/panic and value is within bounds.
	h.toleranceMu.Lock()
	final := h.sweetSpotTokens
	h.toleranceMu.Unlock()

	cap := int(float64(cfg.SweetSpotTokens) * 1.5)
	if final > cap {
		t.Errorf("sweetSpotTokens = %d, exceeds cap %d", final, cap)
	}
}
