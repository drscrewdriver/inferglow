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

package contextmgr

import (
	"context"
	"strings"
	"testing"
)

// testCompressEngine is a CompressEngine stub returning a valid merged
// reorganize JSON decision.
type testCompressEngine struct{}

func (testCompressEngine) Call(ctx context.Context, prompt string) (string, error) {
	return `{"q1_constitutional_append":[],"q2_new_head_summary":"","q3_step_decisions":[]}`, nil
}

func TestReorganize_NilEngineReturnsError(t *testing.T) {
	mgr, err := NewHybridManager(DefaultConfig(), newFakeStore())
	if err != nil {
		t.Fatalf("NewHybridManager error: %v", err)
	}
	h := mgr.(*HybridManager)

	// No attached engine, nil passed: must fail clearly, not panic.
	if _, err := h.Reorganize(context.Background(), nil, "focus"); err == nil {
		t.Fatal("expected error when no engine is available")
	} else if !strings.Contains(err.Error(), "no compression engine") {
		t.Errorf("expected clear engine-missing error, got %v", err)
	}
}

func TestReorganize_FallsBackToAttachedEngine(t *testing.T) {
	mgr, err := NewHybridManager(DefaultConfig(), newFakeStore())
	if err != nil {
		t.Fatalf("NewHybridManager error: %v", err)
	}
	h := mgr.(*HybridManager)
	h.SetReorganizeEngine(testCompressEngine{})

	// Nil engine must fall back to the attached one and succeed.
	res, err := h.Reorganize(context.Background(), nil, "focus")
	if err != nil {
		t.Fatalf("Reorganize with attached engine error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
}

// mechanicalTestManager builds a manager with the given steps and config.
func mechanicalTestManager(t *testing.T, cfg Config, steps map[int]StepRecord) *HybridManager {
	t.Helper()
	store := newFakeStore()
	for id, s := range steps {
		store.steps[id] = s
		store.refs[id] = RefRecord{StepID: id, Level: 0, Strength: 1.0}
	}
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatalf("NewHybridManager error: %v", err)
	}
	return mgr.(*HybridManager)
}

func TestMechanicalReorganize_GeneratesLevelDecisions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds = ThresholdConfig{} // all-zero thresholds => any decay hits L3
	cfg.OverflowCapTokens = 0
	h := mechanicalTestManager(t, cfg, map[int]StepRecord{
		1: {StepID: 1, Type: "reasoning", Content: "old", TokenCount: 10000},
		2: {StepID: 2, Type: "reasoning", Content: "old", TokenCount: 10000},
		3: {StepID: 3, Type: "reasoning", Content: "old", TokenCount: 10000},
	})
	h.currentStep = 3

	res, err := h.MechanicalReorganize(context.Background(), true)
	if err != nil {
		t.Fatalf("MechanicalReorganize error: %v", err)
	}
	if res.StepsAdjusted == 0 {
		t.Error("expected at least one step adjusted by mechanical reorganize")
	}
	// Aggressive mode must have raised levels.
	ref, _ := h.store.GetRef(1)
	if ref.Level == 0 {
		t.Error("expected step 1 level raised in aggressive mode")
	}
}

func TestMechanicalReorganize_RespectsRetainedTail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds = ThresholdConfig{}
	cfg.RetainTokens = 2
	cfg.OverflowCapTokens = 0
	h := mechanicalTestManager(t, cfg, map[int]StepRecord{
		1: {StepID: 1, Type: "reasoning", Content: "old", TokenCount: 10000},
		2: {StepID: 2, Type: "reasoning", Content: "old", TokenCount: 10000},
		3: {StepID: 3, Type: "reasoning", Content: "old", TokenCount: 10000},
		4: {StepID: 4, Type: "reasoning", Content: "recent", TokenCount: 10000},
		5: {StepID: 5, Type: "reasoning", Content: "recent", TokenCount: 10000},
	})
	h.currentStep = 5

	if _, err := h.MechanicalReorganize(context.Background(), false); err != nil {
		t.Fatalf("MechanicalReorganize error: %v", err)
	}
	for _, id := range []int{4, 5} {
		ref, _ := h.store.GetRef(id)
		if ref.Level != 0 {
			t.Errorf("retained-tail step %d must stay L0, got L%d", id, ref.Level)
		}
	}
	ref1, _ := h.store.GetRef(1)
	if ref1.Level == 0 {
		t.Error("non-tail step 1 should have been compressed")
	}
}

func TestMechanicalReorganize_OverflowBypassesTail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds = ThresholdConfig{}
	cfg.RetainTokens = 4
	cfg.OverflowCapTokens = 5000 // total tokens (40000) exceed the cap
	h := mechanicalTestManager(t, cfg, map[int]StepRecord{
		1: {StepID: 1, Type: "reasoning", Content: "old", TokenCount: 10000},
		2: {StepID: 2, Type: "reasoning", Content: "old", TokenCount: 10000},
		3: {StepID: 3, Type: "reasoning", Content: "old", TokenCount: 10000},
		4: {StepID: 4, Type: "reasoning", Content: "recent", TokenCount: 10000},
	})
	h.currentStep = 4

	res, err := h.MechanicalReorganize(context.Background(), false)
	if err != nil {
		t.Fatalf("MechanicalReorganize error: %v", err)
	}
	// Overflow must bypass the retained tail: recent steps get compressed too.
	ref4, _ := h.store.GetRef(4)
	if ref4.Level == 0 {
		t.Error("overflow bypass must compress retained-tail step 4")
	}
	if res.StepsAdjusted != 4 {
		t.Errorf("StepsAdjusted = %d, want 4 (all steps bypassed)", res.StepsAdjusted)
	}
}
