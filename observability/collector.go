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

package observability

import (
	"sort"
	"sync"
	"time"
)

// SpanKind identifies the type of span (mirrors otel SpanKind).
type SpanKind string

const (
	SpanKindLLM       SpanKind = "llm"
	SpanKindTool      SpanKind = "tool"
	SpanKindAgent     SpanKind = "agent"
	SpanKindCompress  SpanKind = "compress"
	SpanKindRetrieval SpanKind = "retrieval"
	SpanKindInternal  SpanKind = "internal"
)

// SpanSummary is a lightweight representation of a completed span for
// in-memory collection and dashboard display (OT-13).
type SpanSummary struct {
	Name     string            `json:"name"`
	Kind     SpanKind          `json:"kind"`
	Duration time.Duration     `json:"duration_ns"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	EndTime  time.Time         `json:"end_time"`
	HasError bool              `json:"has_error"`
}

// AggregatedStats holds per-kind aggregated metrics.
type AggregatedStats struct {
	TotalSpans   int                    `json:"total_spans"`
	ByKind       map[SpanKind]KindStats `json:"by_kind"`
	RecentErrors int                    `json:"recent_errors"`
}

// KindStats holds aggregated stats for a single SpanKind.
type KindStats struct {
	Count  int           `json:"count"`
	P50    time.Duration `json:"p50_ns"`
	P95    time.Duration `json:"p95_ns"`
	Avg    time.Duration `json:"avg_ns"`
	Errors int           `json:"errors"`
}

// SpanCollector is a bounded in-memory ring buffer that collects finished
// spans for the observability dashboard (OT-13). It implements a subset
// of the OTel SpanProcessor interface (OnEnd).
type SpanCollector struct {
	mu   sync.Mutex
	buf  []SpanSummary
	size int
	pos  int
	full bool
}

// NewSpanCollector creates a ring buffer collector with the given capacity.
// Default capacity is 4096 if n <= 0.
func NewSpanCollector(n int) *SpanCollector {
	if n <= 0 {
		n = 4096
	}
	return &SpanCollector{
		buf:  make([]SpanSummary, n),
		size: n,
	}
}

// OnEnd records a finished span into the ring buffer.
// Compatible with OTel sdktrace.SpanProcessor.OnEnd signature pattern.
func (c *SpanCollector) OnEnd(s SpanSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf[c.pos] = s
	c.pos++
	if c.pos >= c.size {
		c.pos = 0
		c.full = true
	}
}

// Snapshot returns a copy of all collected spans (most recent last).
func (c *SpanCollector) Snapshot() []SpanSummary {
	c.mu.Lock()
	defer c.mu.Unlock()

	var count int
	if c.full {
		count = c.size
	} else {
		count = c.pos
	}

	out := make([]SpanSummary, count)
	if c.full {
		// Ring buffer wrapped: read from pos to end, then 0 to pos.
		n := copy(out, c.buf[c.pos:])
		copy(out[n:], c.buf[:c.pos])
	} else {
		copy(out, c.buf[:c.pos])
	}
	return out
}

// Recent returns the last n spans (or all if fewer exist).
func (c *SpanCollector) Recent(n int) []SpanSummary {
	all := c.Snapshot()
	if n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

// Aggregate computes per-kind statistics from collected spans.
func (c *SpanCollector) Aggregate() AggregatedStats {
	spans := c.Snapshot()
	stats := AggregatedStats{
		TotalSpans: len(spans),
		ByKind:     make(map[SpanKind]KindStats),
	}

	// Group durations by kind.
	kindDurations := make(map[SpanKind][]time.Duration)
	kindErrors := make(map[SpanKind]int)

	for _, s := range spans {
		kindDurations[s.Kind] = append(kindDurations[s.Kind], s.Duration)
		if s.HasError {
			kindErrors[s.Kind]++
			stats.RecentErrors++
		}
	}

	for kind, durations := range kindDurations {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		count := len(durations)
		var sum time.Duration
		for _, d := range durations {
			sum += d
		}
		ks := KindStats{
			Count:  count,
			P50:    durations[count*50/100],
			P95:    durations[min(count*95/100, count-1)],
			Avg:    sum / time.Duration(count),
			Errors: kindErrors[kind],
		}
		stats.ByKind[kind] = ks
	}

	return stats
}
