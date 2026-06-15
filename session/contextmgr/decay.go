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

// EffectiveDecay computes the compression pressure for a given step.
//
// Formula (from spec §6.2):
//
//	effective_decay = raw_decay × ref_mod × file_mod / strength × task_group_mod
//
//	raw_decay      = Σ token_count from (last_ref_step+1 → current_step)
//	ref_mod        = 1.0 / (1.0 + ref_count × 0.2)
//	file_mod       = 0.3 (related file actively edited) | 1.0 (no change)
//	strength       = accumulated access strength (init 1.0, +0.1/ref)
//	task_group_mod = 1.5 (completed group) | 1.0 (active group)
func EffectiveDecay(ref RefRecord, rawDecay int, fileActive bool, groupCompleted bool) float64 {
	refMod := 1.0 / (1.0 + float64(ref.RefCount)*0.2)

	fileMod := 1.0
	if fileActive {
		fileMod = 0.3
	}

	strength := ref.Strength
	if strength < 0.1 {
		strength = 0.1 // avoid division by zero
	}

	groupMod := 1.0
	if groupCompleted {
		groupMod = 1.5
	}

	return float64(rawDecay) * refMod * fileMod / strength * groupMod
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
