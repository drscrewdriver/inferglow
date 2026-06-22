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
	"regexp"
	"sync/atomic"
)

// refCiteReLocal matches §N references in LLM output.
var refCiteReLocal = regexp.MustCompile(`§(\d+)`)

// ProcessCitationsWithTolerance parses §N references from LLM output,
// updates refs, and adjusts the sweet-spot tolerance based on the
// sliding-window reference rate.
//
// This replaces the simpler ProcessCitations method when sweet-spot is enabled.
func (h *HybridManager) ProcessCitationsWithTolerance(output string) {
	matches := refCiteReLocal.FindAllStringSubmatch(output, -1)
	currentStep := int(atomic.LoadInt32(&h.currentStep))

	uniqueRefs := make(map[int]bool)
	for _, m := range matches {
		stepID := 0
		if n, err := parseInt(m[1]); err == nil {
			stepID = n
		}
		if stepID <= 0 {
			continue
		}
		uniqueRefs[stepID] = true

		ref, err := h.store.GetRef(stepID)
		if err != nil {
			continue
		}
		ref.RefCount++
		ref.LastRefAtStep = &currentStep
		ref.Strength += 0.1
		_ = h.store.UpsertRef(*ref)
	}

	// Update sliding window with unique reference count
	if h.sweetSpotOriginal > 0 {
		h.updateSlidingWindow(len(uniqueRefs))
	}
}

// updateSlidingWindow records a reference count and adjusts tolerance
// based on the sliding-window reference rate.
func (h *HybridManager) updateSlidingWindow(uniqueRefs int) {
	h.toleranceMu.Lock()
	defer h.toleranceMu.Unlock()

	// Record into circular buffer
	h.recentRefCounts[h.recentRefIdx] = uniqueRefs
	h.recentRefIdx = (h.recentRefIdx + 1) % len(h.recentRefCounts)

	// Compute sliding average
	avgRefs := h.averageRecentRefs()

	// Compute reference rate against active steps
	ids, err := h.store.AllActiveStepIDs()
	if err != nil || len(ids) == 0 {
		return
	}
	totalActive := len(ids)
	refRate := avgRefs / float64(totalActive)

	switch {
	case refRate >= 0.30:
		// High reference rate: expand sweet spot by 15%
		h.adjustToleranceLocked(1.15)
	case refRate >= 0.15:
		// Medium reference rate: expand by 8%
		h.adjustToleranceLocked(1.08)
	case refRate < 0.05:
		// Low reference rate: shrink by 5%
		h.adjustToleranceLocked(0.95)
	}
	// 0.05 <= refRate < 0.15: stable zone, no change
}

// adjustToleranceLocked adjusts the tolerance multiplier.
// Caller must hold toleranceMu.
func (h *HybridManager) adjustToleranceLocked(factor float64) {
	newTol := h.sweetSpotTolerance * factor
	if newTol < 1.0 {
		newTol = 1.0
	}
	if newTol > 1.5 {
		newTol = 1.5
	}
	h.sweetSpotTolerance = newTol
	h.sweetSpotTokens = int(float64(h.sweetSpotOriginal) * newTol)
}

// averageRecentRefs computes the average of the sliding window.
func (h *HybridManager) averageRecentRefs() float64 {
	sum := 0
	count := 0
	for _, v := range h.recentRefCounts {
		if v > 0 || count > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

// parseInt is a simple string-to-int helper to avoid importing strconv.
func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &parseError{s}
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

type parseError struct{ s string }

func (e *parseError) Error() string { return "parseInt: invalid " + e.s }
