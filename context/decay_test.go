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

import "testing"

func approx(a, b float64) bool {
	// 1e-9 tolerance keeps cross-check against hand-computed values exact.
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-9
}

// groupCfg is the default-enabled cross-group config used for modulation tests.
func groupCfg() GroupModConfig {
	return GroupModConfig{Enabled: true, DistanceW: 0.3, CrossRefW: 0.2}
}

func heatCfg() HeatModConfig {
	return HeatModConfig{Enabled: true, RecallBoost: 20, SigZoneMin: 70, UnsettledMin: 40, SigMod: 0.7, DecayMod: 1.3}
}

// TestCrossGroupMod covers every A-5 acceptance point plus the disabled gate.
//   same-group = 1.0
//   d1/cross5 = 1.15
//   d1/cross0 = 1.30
//   d3/cross10 = 1.30
//   d3/cross0 = 1.90
func TestCrossGroupMod(t *testing.T) {
	g := groupCfg()
	cases := []struct {
		name       string
		refGroup   int
		curGroup   int
		crossRefs  int
		want       float64
	}{
		{"same-group-neutral", 3, 3, 0, 1.0},
		{"same-group-with-crossrefs", 3, 3, 7, 1.0},
		{"d1-cross5", 1, 2, 5, 1.15},
		{"d1-cross0", 1, 2, 0, 1.30},
		{"d3-cross10", 1, 4, 10, 1.30},
		{"d3-cross0", 1, 4, 0, 1.90},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := crossGroupMod(c.refGroup, c.curGroup, c.crossRefs, g)
			if !approx(got, c.want) {
				t.Fatalf("crossGroupMod(%d,%d,%d)=%v, want %v", c.refGroup, c.curGroup, c.crossRefs, got, c.want)
			}
		})
	}

	// Disabled gate returns neutral regardless of distance / cross refs.
	if got := crossGroupMod(1, 4, 0, GroupModConfig{}); got != 1.0 {
		t.Fatalf("disabled crossGroupMod=%v, want 1.0", got)
	}
	// Negative distance (current group older than ref group) is clamped to 0.
	if got := crossGroupMod(4, 1, 0, g); got != 1.0 {
		t.Fatalf("clamped-distance crossGroupMod(4,1,0)=%v, want 1.0", got)
	}
}

// TestHeatMod covers the three A-13 zones plus the zero / disabled gates.
//   0 or disabled -> 1.0
//   >= 70 (significant) -> 0.7
//   [40,70) (unsettled) -> 1.0
//   < 40 (decay) -> 1.3
func TestHeatMod(t *testing.T) {
	h := heatCfg()
	cases := []struct {
		heat int
		want float64
	}{
		{0, 1.0},  // no heat data: neutral, never faster
		{100, 0.7}, // significant zone
		{70, 0.7},  // significant zone boundary-inclusive
		{69, 1.0},  // unsettled zone
		{40, 1.0},  // unsettled zone boundary-inclusive
		{39, 1.3},  // decay zone
		{1, 1.3},   // decay zone lower region
	}
	for _, c := range cases {
		got := heatMod(c.heat, h)
		if !approx(got, c.want) {
			t.Fatalf("heatMod(%d)=%v, want %v", c.heat, got, c.want)
		}
	}

	// Disabled returns neutral even with heat data.
	if got := heatMod(100, HeatModConfig{}); got != 1.0 {
		t.Fatalf("disabled heatMod=%v, want 1.0", got)
	}
}

// TestComputeDecayZeroCfgEquivalence proves that a zero DecayConfig exactly
// reproduces the historical (pre-refactor) EffectiveDecay formula:
//
//	effective = raw_decay * 1/(1+refcount*0.2) * file_mod / strength
//
// with group_mod and heat_mod both neutral (1.0). This is the registration
// guarantee that keeps the existing hybrid/compress call paths behaviour-identical
// while ComputeDecay adds richer modulation.
func TestComputeDecayZeroCfgEquivalence(t *testing.T) {
	legacy := func(ref RefRecord, rawDecay int, fileActive bool) float64 {
		fileMod := 1.0
		if fileActive {
			fileMod = 0.3
		}
		strength := ref.Strength
		if strength < 0.1 {
			strength = 0.1
		}
		return float64(rawDecay) * 1.0 / (1.0 + float64(ref.RefCount)*0.2) * fileMod / strength
	}

	ref := RefRecord{StepID: 1, RefCount: 4, Strength: 1.0, TaskGroupID: 2, CrossGroupRefs: 6, Heat: 100}
	raw := 5000
	for _, fileActive := range []bool{false, true} {
		decay, trace := ComputeDecay(ref, raw, fileActive, 2, DecayConfig{})
		want := legacy(ref, raw, fileActive)
		if !approx(decay, want) {
			t.Fatalf("zero-cfg decay(fileActive=%v)=%v, legacy=%v", fileActive, decay, want)
		}
		if trace.GroupMod != 1.0 || trace.HeatMod != 1.0 {
			t.Fatalf("zero-cfg trace must be neutral (group=%v heat=%v)", trace.GroupMod, trace.HeatMod)
		}
		// Heat==100 but zero cfg disables heat; ref count discount applies.
		if trace.RefMod <= 0 || trace.Strength < 0.1 {
			t.Fatalf("zero-cfg trace produced degenerate coefficients: %+v", trace)
		}
	}

	// Strength clamp: a strength just below threshold still yields finite decay.
	weak := RefRecord{StepID: 2, RefCount: 0, Strength: 0.05}
	decay, _ := ComputeDecay(weak, 1000, false, 0, DecayConfig{})
	if decay <= 0 || decay > 1000*10 { // clamped strength floor keeps it bounded
		t.Fatalf("strength-clamp decay=%v out of expected range", decay)
	}
	// Low RefCount with high strength should give decay very close to raw.
	strong := RefRecord{StepID: 3, RefCount: 0, Strength: 2.0}
	decay, _ = ComputeDecay(strong, 1000, false, 0, DecayConfig{})
	if !approx(decay, 500) { // 1000 * 1/1 * 1/2
		t.Fatalf("strong-rec decay=%v, want 500", decay)
	}
}

// TestComputeDecayEnabledChain asserts the full modulation chain when both
// cross-group and heat dimensions are enabled.
func TestComputeDecayEnabledChain(t *testing.T) {
	cfg := DecayConfig{
		RawDecayBase:    1.0,
		RefModWeight:    0.2,
		FileModWeight:   0.3,
		StrengthDivisor: 1.0,
		Group:           groupCfg(),
		Heat:            heatCfg(),
	}
	// ref #5, strength 1.0, group 1; current group 4 -> distance 3, no cross refs.
	ref := RefRecord{StepID: 9, RefCount: 5, Strength: 1.0, TaskGroupID: 1, CrossGroupRefs: 0, Heat: 100}
	decay, trace := ComputeDecay(ref, 1000, false, 4, cfg)

	refMod := 1.0 / (1.0 + 5.0*0.2) // 0.5
	groupMod := 1.90                 // d3/cross0
	heatM := 0.7                     // significant zone (heat 100)
	want := 1000.0 * refMod * 1.0 / 1.0 * groupMod * heatM
	if !approx(decay, want) {
		t.Fatalf("enabled-chain decay=%v, want %v", decay, want)
	}
	if !approx(trace.RefMod, refMod) || !approx(trace.GroupMod, groupMod) || !approx(trace.HeatMod, heatM) {
		t.Fatalf("enabled-chain trace mismatch: %+v", trace)
	}
}

// TestApplicationGate verifies EffectiveDecay (compat shell) is numerically
// identical to ComputeDecay with a zero config, so callers that kept the old
// signature observe zero behaviour change.
func TestEffectiveDecayCompatShell(t *testing.T) {
	ref := RefRecord{StepID: 4, RefCount: 3, Strength: 1.0, TaskGroupID: 7, CrossGroupRefs: 2, Heat: 80}
	legacy := EffectiveDecay(ref, 2000, false, false)
	direct, _ := ComputeDecay(ref, 2000, false, ref.TaskGroupID, DecayConfig{})
	if !approx(legacy, direct) {
		t.Fatalf("EffectiveDecay=%v != ComputeDecay=%v", legacy, direct)
	}
	// groupCompleted is deprecated/ignored: passing true must not change result.
	if got := EffectiveDecay(ref, 2000, false, true); !approx(got, legacy) {
		t.Fatalf("EffectiveDecay ignores groupCompleted: %v != %v", got, legacy)
	}
}

// TestDecayTraceFields ensures every trace field is populated by ComputeDecay
// and that TargetLevel is left ready for the caller to backfill.
func TestDecayTraceFields(t *testing.T) {
	ref := RefRecord{StepID: 42, RefCount: 2, Strength: 1.5, TaskGroupID: 1, CrossGroupRefs: 3}
	_, trace := ComputeDecay(ref, 777, true, 1, DecayConfig{})
	if trace.StepID != 42 {
		t.Fatalf("trace.StepID=%d, want 42", trace.StepID)
	}
	if trace.RawDecay != 777 {
		t.Fatalf("trace.RawDecay=%d, want 777", trace.RawDecay)
	}
	if trace.FileMod != 0.3 {
		t.Fatalf("trace.FileMod=%v, want 0.3 (fileActive)", trace.FileMod)
	}
	if trace.Strength != 1.5 {
		t.Fatalf("trace.Strength=%v, want 1.5", trace.Strength)
	}
	if trace.GroupMod != 1.0 || trace.HeatMod != 1.0 {
		t.Fatalf("neutral modes expected, got group=%v heat=%v", trace.GroupMod, trace.HeatMod)
	}
	if trace.Effective <= 0 {
		t.Fatalf("trace.Effective=%v must be positive", trace.Effective)
	}
	if trace.TargetLevel != 0 || trace.Reason != "" {
		t.Fatalf("TargetLevel/Reason should be backfilled by caller, got lvl=%d reason=%q", trace.TargetLevel, trace.Reason)
	}
}

// TestApplyRecallBoost verifies the gated citation write-path accumulation.
func TestApplyRecallBoost(t *testing.T) {
	cfgEnabled := DecayConfig{Heat: heatCfg()}
	cfgDisabled := DecayConfig{} // Heat.Enabled == false

	// Disabled: record untouched even with cross-group hit.
	ref := RefRecord{TaskGroupID: 1, Heat: 0, CrossGroupRefs: 0}
	applyRecallBoost(&ref, 2, cfgDisabled)
	if ref.Heat != 0 || ref.CrossGroupRefs != 0 {
		t.Fatalf("disabled applyRecallBoost mutated record: %+v", ref)
	}

	// Enabled + same group: heat capped boost, no cross-group increment.
	ref = RefRecord{TaskGroupID: 1, Heat: 0, CrossGroupRefs: 0}
	applyRecallBoost(&ref, 1, cfgEnabled)
	if ref.Heat != 20 || ref.CrossGroupRefs != 0 {
		t.Fatalf("enabled same-group result=%+v, want heat 20 cross 0", ref)
	}

	// Enabled + cross group: heat boost and cross++.
	ref = RefRecord{TaskGroupID: 1, Heat: 0, CrossGroupRefs: 0}
	applyRecallBoost(&ref, 5, cfgEnabled)
	if ref.Heat != 20 || ref.CrossGroupRefs != 1 {
		t.Fatalf("enabled cross-group result=%+v, want heat 20 cross 1", ref)
	}

	// Heat cap at 100.
	ref = RefRecord{TaskGroupID: 1, Heat: 95, CrossGroupRefs: 0}
	applyRecallBoost(&ref, 1, cfgEnabled)
	if ref.Heat != 100 {
		t.Fatalf("heat cap failed: %d, want 100", ref.Heat)
	}
}

// TestTargetLevelBackfill verifies the caller's backfill pattern used in the
// hybrid migration is internally consistent.
func TestTargetLevelBackfill(t *testing.T) {
	ref := RefRecord{StepID: 1, RefCount: 0, Strength: 1.0, TaskGroupID: 1}
	decay, trace := ComputeDecay(ref, 50000, false, 1, DefaultDecayConfig())
	lvl := TargetLevel(decay, "reasoning", DefaultConfig().Thresholds)
	trace.TargetLevel = lvl
	if trace.TargetLevel < 0 || trace.TargetLevel > 4 {
		t.Fatalf("backfilled TargetLevel=%d out of range", trace.TargetLevel)
	}
	if !approx(decay, trace.Effective) {
		t.Fatalf("Effective mismatch: decay=%v trace=%v", decay, trace.Effective)
	}
}