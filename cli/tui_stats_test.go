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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"testing"
	"time"

	"github.com/inferglow/model"
)

func TestCacheHitRate(t *testing.T) {
	u := &model.UsageInfo{
		PromptTokens: 1000,
		PromptTokensDetails: map[string]int{
			"cached_tokens":  400,
			"cache_creation": 100,
		},
	}
	rate, read, write, ok := cacheHitRate(u)
	if !ok {
		t.Fatal("cacheHitRate should be ok with usage present")
	}
	// 400/(1000+400+100) = 400/1500 = 26.67%
	if rate < 26.6 || rate > 26.7 {
		t.Fatalf("rate = %.2f, want ~26.67", rate)
	}
	if read != 400 || write != 100 {
		t.Fatalf("read=%d write=%d, want 400/100", read, write)
	}
}

func TestCacheHitRateNilAndZero(t *testing.T) {
	if _, _, _, ok := cacheHitRate(nil); ok {
		t.Fatal("nil usage should not be ok")
	}
	// total <= 0 → not ok.
	u := &model.UsageInfo{}
	if _, _, _, ok := cacheHitRate(u); ok {
		t.Fatal("zero total should not be ok")
	}
	// Missing cached_tokens → read=0 but still ok when prompt>0.
	u2 := &model.UsageInfo{PromptTokens: 500}
	rate, read, _, ok := cacheHitRate(u2)
	if !ok {
		t.Fatal("usage without cached tokens should still be ok")
	}
	if read != 0 || rate != 0 {
		t.Fatalf("read=%d rate=%.1f, want 0/0", read, rate)
	}
}

func TestTPSTrackerOnToken(t *testing.T) {
	tr := &tpsTracker{}
	tr.OnToken(40) // 10 tokens at once
	// Allow a tiny time slice so elapsed > 0.
	time.Sleep(2 * time.Millisecond)
	tr.OnToken(0)
	if tr.current <= 0 {
		t.Fatalf("current tps should be positive after tokens, got %v", tr.current)
	}
	// Monotonic accumulation.
	prev := tr.current
	time.Sleep(10 * time.Millisecond)
	tr.OnToken(4000) // 1000 more tokens
	if tr.current <= prev {
		t.Fatalf("tps should grow: %v -> %v", prev, tr.current)
	}
}

func TestTPSTrackerSampleAndCap(t *testing.T) {
	tr := &tpsTracker{}
	for i := 0; i < tpsSampleMax+20; i++ {
		tr.OnToken(4)
		tr.OnRunEnd()
	}
	if len(tr.samples) != tpsSampleMax {
		t.Fatalf("samples = %d, want cap %d", len(tr.samples), tpsSampleMax)
	}
}

func TestTPSTrackerStats(t *testing.T) {
	tr := &tpsTracker{}
	// 100 samples of 10 tps each → avg60=mean=p95=10.
	for i := 0; i < 100; i++ {
		tr.samples = append(tr.samples, tpsSample{tps: 10, at: int64(i)})
	}
	avg60, mean, p95 := tr.Stats()
	if avg60 != 10 || mean != 10 || p95 != 10 {
		t.Fatalf("stats = %.1f/%.1f/%.1f, want 10/10/10", avg60, mean, p95)
	}
	// Empty tracker → zeros.
	empty := &tpsTracker{}
	a, mm, p := empty.Stats()
	if a != 0 || mm != 0 || p != 0 {
		t.Fatalf("empty stats = %v/%v/%v, want 0/0/0", a, mm, p)
	}
}

func TestTPSTrackerP95(t *testing.T) {
	tr := &tpsTracker{}
	// 19 samples: 18 × 10, 1 × 100 → p95 index ceil(19*0.95)-1 = 18 → 100.
	for i := 0; i < 18; i++ {
		tr.samples = append(tr.samples, tpsSample{tps: 10, at: int64(i)})
	}
	tr.samples = append(tr.samples, tpsSample{tps: 100, at: 99})
	p := tr.p95()
	if p != 100 {
		t.Fatalf("p95 = %.0f, want 100", p)
	}
}

func TestTPSColorThresholds(t *testing.T) {
	if got := tpsColor(60); got == "" {
		t.Fatal("tpsColor(60) should color green")
	}
	if got := tpsColor(30); got == "" {
		t.Fatal("tpsColor(30) should color yellow")
	}
	if got := tpsColor(5); got == "" {
		t.Fatal("tpsColor(5) should color red")
	}
}

func TestRenderTPSGauge(t *testing.T) {
	if got := renderTPSGauge(100, 100); got != "█" {
		t.Fatalf("full gauge = %q, want █", got)
	}
	if got := renderTPSGauge(0, 100); got != "▏" {
		t.Fatalf("empty gauge = %q, want ▏", got)
	}
}

func TestRenderTPSSparkline(t *testing.T) {
	samples := []tpsSample{{tps: 0}, {tps: 100}, {tps: 50}}
	out := renderTPSSparkline(samples, 3)
	if len([]rune(out)) != 3 {
		t.Fatalf("sparkline rune length = %d, want 3", len([]rune(out)))
	}
	if renderTPSSparkline(nil, 3) != "" {
		t.Fatal("empty samples should render empty")
	}
	if renderTPSSparkline(samples, 0) != "" {
		t.Fatal("zero width should render empty")
	}
}

func TestTrackToolEndCountsAndDurations(t *testing.T) {
	m := &chatTUI{}
	m.receipt.toolDurationsMs = nil
	start := time.Now()
	m.trackToolStart("bash", start)
	m.trackToolEnd("bash", start.Add(1500*time.Millisecond))
	if m.receipt.toolCalls != 1 {
		t.Fatalf("toolCalls = %d, want 1", m.receipt.toolCalls)
	}
	if got := m.receipt.toolDurationsMs["bash"]; got < 1400 || got > 1600 {
		t.Fatalf("bash duration = %dms, want ~1500", got)
	}
	// Same tool twice accumulates.
	m.trackToolStart("bash", time.Now())
	m.trackToolEnd("bash", time.Now().Add(500*time.Millisecond))
	if got := m.receipt.toolDurationsMs["bash"]; got < 1900 || got > 2100 {
		t.Fatalf("accumulated bash = %dms, want ~2000", got)
	}
	if m.receipt.toolCalls != 2 {
		t.Fatalf("toolCalls = %d, want 2", m.receipt.toolCalls)
	}
}

func TestResetTurnStats(t *testing.T) {
	m := &chatTUI{}
	m.receipt.turnNum = 5
	m.receipt.toolDurationsMs = map[string]int64{"bash": 100}
	m.thinkingStart = time.Now()
	m.resetTurnStats(time.Now())
	if m.receipt.turnNum != 6 {
		t.Fatalf("turnNum = %d, want 6 (incremented)", m.receipt.turnNum)
	}
	if m.receipt.toolDurationsMs != nil || m.receipt.toolCalls != 0 || m.receipt.thinkingMs != 0 {
		t.Fatalf("reset failed: %+v", m.receipt)
	}
	if !m.thinkingStart.IsZero() {
		t.Fatal("thinkingStart should be zeroed")
	}
}

func TestEffortOptions(t *testing.T) {
	m := &chatTUI{}
	for _, lv := range []string{"low", "medium", "high"} {
		m.effort = lv
		opts := m.effortOptions()
		if opts == nil || opts["reasoning_effort"] != lv {
			t.Fatalf("effort %s → %v", lv, opts)
		}
	}
	for _, lv := range []string{"", "auto"} {
		m.effort = lv
		if opts := m.effortOptions(); opts != nil {
			t.Fatalf("effort %q should inject nothing, got %v", lv, opts)
		}
	}
}
