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
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inferglow/model"
)

// ---------------------------------------------------------------------------
// RF-6: per-turn reasoning / tool timing (ingested in tui_model.ingestEvent).
// ---------------------------------------------------------------------------

// beginThinking starts a reasoning timing window (idempotent).
func (m *chatTUI) beginThinking(now time.Time) {
	if m.thinkingStart.IsZero() {
		m.thinkingStart = now
	}
}

// endThinking closes the reasoning timing window and accumulates it.
func (m *chatTUI) endThinking(now time.Time) {
	if !m.thinkingStart.IsZero() {
		m.receipt.thinkingMs += now.Sub(m.thinkingStart).Milliseconds()
		m.thinkingStart = time.Time{}
	}
}

// trackToolStart records a tool's start timestamp.
func (m *chatTUI) trackToolStart(name string, now time.Time) {
	if m.toolStart == nil {
		m.toolStart = map[string]time.Time{}
	}
	m.toolStart[name] = now
}

// trackToolEnd closes a tool's timing window and counts it. Returns the tool's
// elapsed duration in ms (0 when start was not recorded).
func (m *chatTUI) trackToolEnd(name string, now time.Time) int64 {
	var elapsed int64
	if st, ok := m.toolStart[name]; ok {
		elapsed = now.Sub(st).Milliseconds()
		delete(m.toolStart, name)
		if m.receipt.toolDurationsMs == nil {
			m.receipt.toolDurationsMs = map[string]int64{}
		}
		m.receipt.toolDurationsMs[name] += elapsed
	}
	m.receipt.toolCalls++
	return elapsed
}

// resetTurnStats clears per-turn timing/metrics before a new turn.
func (m *chatTUI) resetTurnStats(now time.Time) {
	m.receipt.turnNum++
	m.receipt.duration = 0
	m.receipt.llmRounds = 0
	m.receipt.toolCalls = 0
	m.receipt.thinkingMs = 0
	m.receipt.reasoningTokens = 0
	m.receipt.toolDurationsMs = nil
	m.receipt.totalOutputChars = 0
	m.receipt.usage = nil
	m.thinkingStart = time.Time{}
	m.toolStart = nil
	_ = now
}

// ---------------------------------------------------------------------------
// RF-7: TPS (tokens per second) tracker — mirrors dsh-tui StatusMetrics.ts.
// Token count is estimated as outputChars / 4.
// ---------------------------------------------------------------------------

const (
	tpsSampleMax = 500
	tpsCharsPerTok = 4
)

// tpsSample is one per-turn TPS sample.
type tpsSample struct {
	tps float64
	at  int64 // unix ms
}

// tpsTracker computes live and historical TPS stats.
type tpsTracker struct {
	current    float64
	samples    []tpsSample
	firstToken time.Time
	firstSet   bool
	chars      int
}

// OnToken accumulates output characters and updates the live TPS from the
// first token to now.
func (t *tpsTracker) OnToken(chars int) {
	now := time.Now()
	if !t.firstSet {
		t.firstToken = now
		t.firstSet = true
		t.chars = 0
	}
	t.chars += chars
	elapsed := now.Sub(t.firstToken).Seconds()
	if elapsed > 0 {
		t.current = float64(t.chars) / tpsCharsPerTok / elapsed
	}
}

// OnRunEnd records a per-turn sample and resets the live window.
func (t *tpsTracker) OnRunEnd() {
	t.samples = append(t.samples, tpsSample{tps: t.current, at: time.Now().UnixMilli()})
	if len(t.samples) > tpsSampleMax {
		t.samples = t.samples[len(t.samples)-tpsSampleMax:]
	}
	t.current = 0
	t.firstSet = false
	t.chars = 0
}

// avg60 returns the average TPS over the last 60 samples (or all when fewer).
func (t *tpsTracker) avg60() float64 {
	n := len(t.samples)
	if n == 0 {
		return 0
	}
	if n > 60 {
		n = 60
	}
	sum := 0.0
	for _, s := range t.samples[len(t.samples)-n:] {
		sum += s.tps
	}
	return sum / float64(n)
}

// mean returns the average TPS across all samples.
func (t *tpsTracker) mean() float64 {
	n := len(t.samples)
	if n == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range t.samples {
		sum += s.tps
	}
	return sum / float64(n)
}

// p95 returns the 95th-percentile TPS across all samples.
func (t *tpsTracker) p95() float64 {
	n := len(t.samples)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	for i, s := range t.samples {
		sorted[i] = s.tps
	}
	sort.Float64s(sorted)
	idx := int(math.Ceil(float64(n)*0.95)) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

// Stats returns (avg60, mean, p95).
func (t *tpsTracker) Stats() (avg60, mean, p95 float64) {
	return t.avg60(), t.mean(), t.p95()
}

// tpsColor returns the ANSI-wrapped TPS value colored by the speed threshold
// (≥50 green, ≥20 yellow, <20 red).
func tpsColor(tps float64) string {
	s := fmt.Sprintf("%.0f", tps)
	switch {
	case tps >= 50:
		return successText(s)
	case tps >= 20:
		return warnText(s)
	default:
		return errorText(s)
	}
}

// renderTPSGauge returns a 1/8-segment gauge (▏▎▍▌▋▊▉█) for live streaming.
func renderTPSGauge(tps float64, maxTPS float64) string {
	if maxTPS <= 0 {
		maxTPS = 100
	}
	segs := []rune("▏▎▍▌▋▊▉█")
	filled := int(math.Round(tps / maxTPS * 8))
	if filled < 0 {
		filled = 0
	}
	if filled > 8 {
		filled = 8
	}
	if filled == 0 {
		return "▏"
	}
	return string(segs[filled-1])
}

// renderTPSSparkline returns a compact history sparkline (▁▃▅▇).
func renderTPSSparkline(samples []tpsSample, width int) string {
	n := len(samples)
	if n == 0 || width <= 0 {
		return ""
	}
	levels := []rune("▁▃▅▇")
	step := float64(n) / float64(width)
	if step < 1 {
		step = 1
	}
	maxT := 0.0
	for _, s := range samples {
		if s.tps > maxT {
			maxT = s.tps
		}
	}
	var sb strings.Builder
	for i := 0; i < width; i++ {
		idx := int(float64(i) * step)
		if idx >= n {
			idx = n - 1
		}
		v := samples[idx].tps
		lvl := 0
		if maxT > 0 {
			lvl = int(v / maxT * 4)
			if lvl > 3 {
				lvl = 3
			}
		}
		sb.WriteRune(levels[lvl])
	}
	return sb.String()
}

// renderLive returns the status-bar live TPS segment during streaming.
func (m *chatTUI) renderLiveTPS() string {
	if !m.cfg.Features.TPS {
		return ""
	}
	cur := m.tps.current
	if cur <= 0 {
		return ""
	}
	return dim("tps ") + tpsColor(cur) + " " + renderTPSGauge(cur, 100)
}

// renderHistoryTPS returns the status-bar history TPS segment when idle.
func (m *chatTUI) renderHistoryTPS() string {
	if !m.cfg.Features.TPS {
		return ""
	}
	if len(m.tps.samples) == 0 {
		return ""
	}
	last := m.tps.samples[len(m.tps.samples)-1].tps
	spark := renderTPSSparkline(m.tps.samples, 6)
	return dim("tps ") + tpsColor(last) + " " + spark
}

// tuiHandleTPS handles /tps (RF-7): reports avg60/mean/p95 + current.
func tuiHandleTPS(m *chatTUI, args string) (tea.Cmd, bool) {
	m.commitLine("")
	avg60, mean, p95 := m.tps.Stats()
	m.commitLine(accent("TPS statistics:"))
	m.commitLine(dim("  avg60 ") + footerInfo(fmt.Sprintf("%.1f", avg60)) +
		dim(" · mean ") + footerInfo(fmt.Sprintf("%.1f", mean)) +
		dim(" · p95  ") + footerInfo(fmt.Sprintf("%.1f", p95)))
	m.commitLine(dim("  current ") + tpsColor(m.tps.current) +
		dim(fmt.Sprintf(" · samples %d", len(m.tps.samples))))
	return nil, false
}

// ---------------------------------------------------------------------------
// RF-8: cache hit rate from UsageInfo.
// ---------------------------------------------------------------------------

// cacheHitRate computes the prompt-cache hit rate:
// cacheRead/(input+cacheRead+cacheWrite). ok=false when usage is nil, totals
// are non-positive, or no cached token data is present.
func cacheHitRate(u *model.UsageInfo) (rate float64, cacheRead, cacheWrite int, ok bool) {
	if u == nil {
		return 0, 0, 0, false
	}
	if u.PromptTokensDetails != nil {
		cacheRead = u.PromptTokensDetails["cached_tokens"]
		cacheWrite = u.PromptTokensDetails["cache_creation"]
	}
	total := u.PromptTokens + cacheRead + cacheWrite
	if total <= 0 {
		return 0, 0, 0, false
	}
	return float64(cacheRead) / float64(total) * 100, cacheRead, cacheWrite, true
}

// formatCacheHit renders the cache-hit field for the status bar / receipt.
func (m *chatTUI) renderCacheHit() string {
	if !m.cfg.Features.CacheHit {
		return ""
	}
	rate, _, _, ok := cacheHitRate(m.receipt.usage)
	if !ok {
		return ""
	}
	return dim("cache ") + footerInfo(fmt.Sprintf("%.1f%%", rate))
}

// renderReceiptDetail renders the extended per-turn receipt lines (RF-6/8):
//
//	LLM rounds: 2 · Tools: 4 (bash 2.1s, file_read 0.4s)
//	Tokens: in 1.2k · out 567 · reasoning 345
//	Thinking: 3.2s · Cache hit 42.3% (read 12.3k / write 3.1k)
func (m *chatTUI) renderReceiptDetail() []string {
	var lines []string
	if !m.cfg.Features.TurnStats {
		return lines
	}
	// Tools line with per-tool durations.
	tools := fmt.Sprintf("Tools: %d", m.receipt.toolCalls)
	if len(m.receipt.toolDurationsMs) > 0 {
		var parts []string
		// Deterministic order: longest duration first.
		type kv struct {
			name string
			ms   int64
		}
		var kvs []kv
		for name, ms := range m.receipt.toolDurationsMs {
			kvs = append(kvs, kv{name, ms})
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].ms > kvs[j].ms })
		for _, e := range kvs {
			parts = append(parts, fmt.Sprintf("%s %.1fs", e.name, float64(e.ms)/1000))
		}
		tools += " (" + strings.Join(parts, ", ") + ")"
	}
	lines = append(lines, fmt.Sprintf("LLM rounds: %d · %s", m.receipt.llmRounds, tools))

	// Tokens line.
	toks := fmt.Sprintf("in %s · out %s",
		shortTokens(m.receipt.promptTokens), shortTokens(m.receipt.completionTokens))
	if m.receipt.reasoningTokens > 0 {
		toks += fmt.Sprintf(" · reasoning %s", shortTokens(m.receipt.reasoningTokens))
	}
	lines = append(lines, "Tokens: "+toks)

	// Thinking + cache line.
	thinking := fmt.Sprintf("Thinking: %.1fs", float64(m.receipt.thinkingMs)/1000)
	if rate, cacheRead, cacheWrite, ok := cacheHitRate(m.receipt.usage); ok {
		thinking += fmt.Sprintf(" · Cache hit %.1f%% (read %s / write %s)",
			rate, shortTokens(cacheRead), shortTokens(cacheWrite))
	}
	lines = append(lines, thinking)
	return lines
}
