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

package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/inferglow/orchestrator/actionruntime"
)

// Test 1: RepeatAction → Break
// Three rounds with identical ActionCalls should trigger Break with a reason
// mentioning "repeated action calls" and the action name.
func TestLoopGuard_RepeatActionBreak(t *testing.T) {
	cfg := LoopGuardConfig{
		RepeatActionWindow: 3,
	}
	g := NewLoopGuard(cfg)
	startedAt := time.Now()

	calls := []actionruntime.ActionCall{
		{Name: "search", Params: map[string]any{"q": "hello"}},
	}

	// Rounds 1 and 2: not enough samples yet → Continue.
	for i := 0; i < 2; i++ {
		v, err := g.Check(LoopGuardState{
			Round:       i + 1,
			ActionCalls: calls,
			LastOutput:  "output round " + string(rune('0'+i+1)),
			StartedAt:   startedAt,
		})
		if err != nil {
			t.Fatalf("round %d: unexpected error: %v", i+1, err)
		}
		if v.Action != VerdictContinue {
			t.Fatalf("round %d: expected continue, got %q (%s)", i+1, v.Action, v.Reason)
		}
	}

	// Round 3: three identical action lists → Break.
	v, err := g.Check(LoopGuardState{
		Round:       3,
		ActionCalls: calls,
		LastOutput:  "output round 3",
		StartedAt:   startedAt,
	})
	if err != nil {
		t.Fatalf("round 3: unexpected error: %v", err)
	}
	if v.Action != VerdictBreak {
		t.Fatalf("expected break, got %q (%s)", v.Action, v.Reason)
	}
	if !strings.Contains(v.Reason, "repeated action calls") {
		t.Errorf("expected reason to contain 'repeated action calls', got %q", v.Reason)
	}
	if !strings.Contains(v.Reason, "search") {
		t.Errorf("expected reason to contain action name 'search', got %q", v.Reason)
	}

	// Reset should allow the guard to be reused.
	g.Reset()
	if len(g.actionWindow) != 0 || len(g.outputWindow) != 0 {
		t.Fatalf("Reset did not clear internal windows")
	}
}

// Test 2: OutputStagnation → Break
// Three rounds with identical LastOutput (and varying action names to avoid
// triggering RepeatAction) should trigger Break with "output stagnation".
func TestLoopGuard_OutputStagnationBreak(t *testing.T) {
	cfg := LoopGuardConfig{
		OutputStagnationWindow:    3,
		OutputSimilarityThreshold: 0.9,
	}
	g := NewLoopGuard(cfg)
	startedAt := time.Now()

	identical := "the agent is stuck repeating itself"

	for i := 0; i < 3; i++ {
		v, err := g.Check(LoopGuardState{
			Round: i + 1,
			// Distinct action names per round so RepeatAction does not trigger.
			ActionCalls: []actionruntime.ActionCall{
				{Name: string(rune('a' + i)), Params: map[string]any{"i": i}},
			},
			LastOutput: identical,
			StartedAt:  startedAt,
		})
		if err != nil {
			t.Fatalf("round %d: unexpected error: %v", i+1, err)
		}
		if i < 2 {
			if v.Action != VerdictContinue {
				t.Fatalf("round %d: expected continue, got %q (%s)", i+1, v.Action, v.Reason)
			}
			continue
		}
		if v.Action != VerdictBreak {
			t.Fatalf("expected break on round 3, got %q (%s)", v.Action, v.Reason)
		}
		if !strings.Contains(v.Reason, "output stagnation") {
			t.Errorf("expected reason to contain 'output stagnation', got %q", v.Reason)
		}
	}
}

// Test 3: TimeBudget → Break
// A single Check call with StartedAt 1 second in the past and a 1ms budget
// should immediately break with reason "time budget exceeded".
func TestLoopGuard_TimeBudgetBreak(t *testing.T) {
	cfg := LoopGuardConfig{
		TimeBudget: 1 * time.Millisecond,
	}
	g := NewLoopGuard(cfg)

	v, err := g.Check(LoopGuardState{
		Round:       1,
		ActionCalls: []actionruntime.ActionCall{{Name: "x"}},
		LastOutput:  "fresh output",
		TotalTokens: 0,
		StartedAt:   time.Now().Add(-1 * time.Second),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Action != VerdictBreak {
		t.Fatalf("expected break, got %q (%s)", v.Action, v.Reason)
	}
	if v.Reason != "time budget exceeded" {
		t.Errorf("expected reason 'time budget exceeded', got %q", v.Reason)
	}
}

// Test 4: TokenBudget → Break
// A single Check call with TotalTokens over the budget should break with
// reason "token budget exceeded".
func TestLoopGuard_TokenBudgetBreak(t *testing.T) {
	cfg := LoopGuardConfig{
		TokenBudget: 100,
	}
	g := NewLoopGuard(cfg)

	v, err := g.Check(LoopGuardState{
		Round:       1,
		ActionCalls: []actionruntime.ActionCall{{Name: "x"}},
		LastOutput:  "fresh output",
		TotalTokens: 200,
		StartedAt:   time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Action != VerdictBreak {
		t.Fatalf("expected break, got %q (%s)", v.Action, v.Reason)
	}
	if v.Reason != "token budget exceeded" {
		t.Errorf("expected reason 'token budget exceeded', got %q", v.Reason)
	}
}

// Test 5: Continue
// A normal single-round state with no triggers should return Continue.
func TestLoopGuard_Continue(t *testing.T) {
	cfg := LoopGuardConfig{}
	g := NewLoopGuard(cfg)
	startedAt := time.Now()

	v, err := g.Check(LoopGuardState{
		Round:       1,
		ActionCalls: []actionruntime.ActionCall{{Name: "x", Params: map[string]any{"k": "v"}}},
		LastOutput:  "a unique response",
		TotalTokens: 10,
		StartedAt:   startedAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Action != VerdictContinue {
		t.Fatalf("expected continue, got %q (%s)", v.Action, v.Reason)
	}
	if v.Reason != "" {
		t.Errorf("expected empty reason for continue, got %q", v.Reason)
	}
}

// Test 6: Disabled
// With Disabled=true, Check must always return Continue even when state would
// normally trigger every break condition.
func TestLoopGuard_Disabled(t *testing.T) {
	cfg := LoopGuardConfig{
		Disabled:    true,
		TokenBudget: 100,
	}
	g := NewLoopGuard(cfg)

	v, err := g.Check(LoopGuardState{
		Round:       1,
		ActionCalls: []actionruntime.ActionCall{{Name: "x"}},
		LastOutput:  "x",
		TotalTokens: 999999,
		StartedAt:   time.Now().Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Action != VerdictContinue {
		t.Fatalf("expected continue when disabled, got %q (%s)", v.Action, v.Reason)
	}
}

// Test 7: jaccardSimilarity unit
// Verifies identical strings → 1.0, disjoint strings → 0.0, partial overlap
// falls in (0, 1), and the empty-string edge cases.
func TestLoopGuard_JaccardSimilarity(t *testing.T) {
	// Identical strings → 1.0
	if got := jaccardSimilarity("hello world", "hello world"); got != 1.0 {
		t.Errorf("identical strings: expected 1.0, got %v", got)
	}

	// Disjoint strings → 0.0
	if got := jaccardSimilarity("alpha", "beta"); got != 0.0 {
		t.Errorf("disjoint strings: expected 0.0, got %v", got)
	}

	// Partial overlap → strictly between 0 and 1
	got := jaccardSimilarity("hello world foo", "hello world bar")
	if got <= 0.0 || got >= 1.0 {
		t.Errorf("partial overlap: expected (0,1), got %v", got)
	}
	// intersection={hello,world}=2, union={hello,world,foo,bar}=4 → 0.5
	if got != 0.5 {
		t.Errorf("partial overlap: expected 0.5, got %v", got)
	}

	// Both empty → 1.0
	if got := jaccardSimilarity("", ""); got != 1.0 {
		t.Errorf("both empty: expected 1.0, got %v", got)
	}

	// Exactly one empty → 0.0
	if got := jaccardSimilarity("hello", ""); got != 0.0 {
		t.Errorf("one empty: expected 0.0, got %v", got)
	}
	if got := jaccardSimilarity("", "hello"); got != 0.0 {
		t.Errorf("one empty: expected 0.0, got %v", got)
	}
}
