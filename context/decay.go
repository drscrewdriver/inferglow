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

// EffectiveDecay computes the compression pressure for a given step, expressed
// as a plain float64.
//
// This is a thin compatibility shell over ComputeDecay preserving the historical
// signature. It passes a zero-valued DecayConfig (whose zero values fall back to
// the historical magic numbers) and treats the ref's own group as the current
// group, so cross-group and heat modulation are neutral here.
//
// Deprecated: groupCompleted is ignored and retained only for call-site
// compatibility; no active caller passes true. New code should call
// ComputeDecay to get cross-group modulation, heat modulation, and a full trace.
func EffectiveDecay(ref RefRecord, rawDecay int, fileActive bool, groupCompleted bool) float64 {
	d, _ := ComputeDecay(ref, rawDecay, fileActive, ref.TaskGroupID, DecayConfig{})
	return d
}

// ComputeDecay computes the effective compression pressure for a step with the
// full modulation chain (A-5 cross-group, A-6 trace + externalised coefficients,
// A-13 heat). It returns the effective_decay float64 plus a DecayTrace for
// observability/auditing.
//
// Formula (from spec §6.2):
//
//	effective_decay = raw_decay × ref_mod × file_mod / strength × group_mod × heat_mod
//
// Zero-valued fields in cfg fall back to the historical constants, so passing a
// zero DecayConfig reproduces the pre-refactor behaviour exactly.
func ComputeDecay(ref RefRecord, rawDecay int, fileActive bool, currentGroupID int, cfg DecayConfig) (float64, DecayTrace) {
	rawBase := cfg.RawDecayBase
	if rawBase == 0 {
		rawBase = 1.0
	}
	refW := cfg.RefModWeight
	if refW == 0 {
		refW = 0.2
	}
	fileW := cfg.FileModWeight
	if fileW == 0 {
		fileW = 0.3
	}
	strDiv := cfg.StrengthDivisor
	if strDiv == 0 {
		strDiv = 1.0
	}

	refMod := 1.0 / (1.0 + float64(ref.RefCount)*refW)

	fileMod := 1.0
	if fileActive {
		fileMod = fileW
	}

	// Decay of accumulated access strength across many references is amplified by
	// /strength below; clamp to avoid a division-by-zero blow-up.
	strength := ref.Strength / strDiv
	if strength < 0.1 {
		strength = 0.1
	}

	groupMod := crossGroupMod(ref.TaskGroupID, currentGroupID, ref.CrossGroupRefs, cfg.Group)
	heatM := heatMod(ref.Heat, cfg.Heat)

	rawDecayVal := rawBase * float64(rawDecay)
	effective := rawDecayVal * refMod * fileMod / strength * groupMod * heatM

	trace := DecayTrace{
		StepID:    ref.StepID,
		RawDecay:  rawDecay,
		RefMod:    refMod,
		FileMod:   fileMod,
		Strength:  strength,
		GroupMod:  groupMod,
		HeatMod:   heatM,
		Effective: effective,
	}
	return effective, trace
}

// DecayTrace captures every coefficient that contributed to an effective-decay
// computation, making the decay pipeline transparent and auditable (A-6).
// TargetLevel is populated by the caller because it needs the step type +
// thresholds that ComputeDecay does not hold.
type DecayTrace struct {
	StepID      int     // step being decayed
	RawDecay    int     // summed token count since last reference
	RefMod      float64 // reference-count discount factor
	FileMod     float64 // related-file edit factor
	Strength    float64 // normalised accumulated access strength
	GroupMod    float64 // cross-group aging modulation (A-5)
	HeatMod     float64 // heat-dimension modulation (A-13)
	Effective   float64 // final effective_decay
	TargetLevel int     // compression target level (set by caller)
	Reason      string  // human-readable reason (audit)
}

// crossGroupMod replaces the historical binary groupMod (1.0/1.5) with a
// distance + cross-group-reference modulation (A-5): same group is neutral, and
// older/uncited groups decay faster.
//
//	group_mod = 1.0 + DistanceW × distance / (1.0 + CrossRefW × crossRefs)
func crossGroupMod(refGroupID, currentGroupID, crossRefs int, g GroupModConfig) float64 {
	if !g.Enabled || refGroupID == currentGroupID {
		return 1.0
	}
	distance := currentGroupID - refGroupID
	if distance < 0 {
		distance = 0
	}
	crossW := g.CrossRefW
	if crossW == 0 {
		crossW = 0.2
	}
	distW := g.DistanceW
	if distW == 0 {
		distW = 0.3
	}
	return 1.0 + distW*float64(distance)/(1.0+crossW*float64(crossRefs))
}

// heatMod returns the heat-dimension modulation (A-13). When the heat dimension
// is disabled, or the record carries no heat data (0), it returns a neutral 1.0
// so existing records are never decayed faster by default.
func heatMod(heat int, h HeatModConfig) float64 {
	if !h.Enabled || heat == 0 {
		return 1.0
	}
	// Resolve zone thresholds against documented defaults.
	sigMin := h.SigZoneMin
	if sigMin == 0 {
		sigMin = 70
	}
	sigMod := h.SigMod
	if sigMod == 0 {
		sigMod = 0.7
	}
	unsettledMin := h.UnsettledMin
	if unsettledMin == 0 {
		unsettledMin = 40
	}
	decayMod := h.DecayMod
	if decayMod == 0 {
		decayMod = 1.3
	}

	switch {
	case heat >= sigMin:
		return sigMod // significant zone: decay slower
	case heat < unsettledMin:
		return decayMod // decay zone: decay faster
	default:
		return 1.0 // unsettled zone: normal
	}
}

// applyRecallBoost mutates a record's heat / cross-group counts when a citation
// or recall hits (A-13 RecallBoost). It is gated on the heat dimension being
// enabled; when disabled it returns early leaving the record untouched.
func applyRecallBoost(ref *RefRecord, currentGroupID int, d DecayConfig) {
	if !d.Heat.Enabled {
		return
	}
	boost := d.Heat.RecallBoost
	if boost == 0 {
		boost = 20
	}
	h := ref.Heat + boost
	if h > 100 {
		h = 100
	}
	ref.Heat = h
	if ref.TaskGroupID != currentGroupID {
		ref.CrossGroupRefs++
	}
}

// TargetLevel determines the compression level a step should reach
// given its effective_decay and type, based on the threshold table (§6.3).
func TargetLevel(decay float64, stepType string, thresholds ThresholdConfig) int {
	if stepType == "tool" {
		switch {
		case decay >= float64(thresholds.L3Tool):
			return 3
		case decay >= float64(thresholds.L2Tool):
			return 2
		case decay >= float64(thresholds.L1Tool):
			return 1
		default:
			return 0
		}
	}

	// reasoning / plan / failed / user
	switch {
	case stepType == "user" && decay >= float64(thresholds.L2Reasoning):
		// user capped at L2
		return 2
	case stepType == "user":
		if decay >= float64(thresholds.L1Reasoning) {
			return 1
		}
		return 0
	case stepType == "reasoning" || stepType == "plan" || stepType == "failed":
		switch {
		case decay >= float64(thresholds.L4Reasoning):
			return 4 // discard
		case decay >= float64(thresholds.L3Reasoning):
			return 3
		case decay >= float64(thresholds.L2Reasoning):
			return 2
		case decay >= float64(thresholds.L1Reasoning):
			return 1
		default:
			return 0
		}
	default:
		// tool result capped at L3
		switch {
		case decay >= float64(thresholds.L3Tool):
			return 3
		case decay >= float64(thresholds.L2Tool):
			return 2
		case decay >= float64(thresholds.L1Tool):
			return 1
		default:
			return 0
		}
	}
}

// MaxLevelForType returns the maximum compression level allowed for a step type (§2.2).
func MaxLevelForType(stepType string) int {
	switch stepType {
	case "user":
		return 2 // user intent cannot be discarded
	case "tool":
		return 3 // tool result capped at L3
	case "reasoning", "plan", "failed":
		return 4 // can be discarded
	default:
		return 3
	}
}
