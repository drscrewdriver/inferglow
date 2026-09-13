package contextmgr

import (
	"context"
	"strings"
	"testing"
)

// TestTriggerCompressionFiresCompactionSink verifies the P2 telemetry hook:
// a TriggerCompression that compresses ≥1 step settles through the optional
// sink carrying the stable CompactionID and the shadowed step range
// [first, last] of the compressed span.
func TestTriggerCompressionFiresCompactionSink(t *testing.T) {
	store := newFakeStore()
	long := strings.Repeat("old reasoning content that decayed long ago ", 10)
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: long}
	store.refs[1] = RefRecord{StepID: 1, Level: 0, Strength: 1.0}
	store.steps[2] = StepRecord{StepID: 2, Type: "reasoning", Content: long}
	store.refs[2] = RefRecord{StepID: 2, Level: 0, Strength: 1.0}
	store.steps[3] = StepRecord{StepID: 3, Type: "reasoning", Content: long}
	store.refs[3] = RefRecord{StepID: 3, Level: 0, Strength: 1.0}

	cfg := DefaultConfig()
	// Zero thresholds force every reasoning step to upgrade (target ≥ L1).
	cfg.Thresholds = ThresholdConfig{}
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	var sinkCalls int
	var sinkResult *CompressResult
	h.SetCompactionSink(func(res *CompressResult) {
		sinkCalls++
		sinkResult = res
	})

	res, err := h.TriggerCompression(context.Background(), CompressOpts{Force: true})
	if err != nil {
		t.Fatalf("TriggerCompression: %v", err)
	}
	if res.StepsCompressed == 0 {
		t.Fatalf("expected steps to be compressed, got %+v", res)
	}
	if sinkCalls != 1 || sinkResult == nil {
		t.Fatalf("expected exactly 1 sink call, got %d (result=%+v)", sinkCalls, sinkResult)
	}
	if sinkResult.CompactionID == "" {
		t.Error("sink result must carry a stable CompactionID")
	}
	if sinkResult.ShadowedRange.StartStep != 1 || sinkResult.ShadowedRange.EndStep != 3 {
		t.Errorf("shadowed range = %+v; want [1,3]", sinkResult.ShadowedRange)
	}

	// A second compression with nothing left to compress must not fire.
	if _, err := h.TriggerCompression(context.Background(), CompressOpts{Force: true}); err != nil {
		t.Fatalf("second TriggerCompression: %v", err)
	}
	if sinkCalls != 1 {
		t.Errorf("sink must not fire when nothing compressed, got %d calls", sinkCalls)
	}
}
