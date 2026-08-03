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
	"testing"
	"time"
)

func TestSpanKind_Values(t *testing.T) {
	tests := []struct {
		kind SpanKind
		want string
	}{
		{SpanKindLLM, "llm"},
		{SpanKindTool, "tool"},
		{SpanKindAgent, "agent"},
		{SpanKindCompress, "compress"},
		{SpanKindRetrieval, "retrieval"},
		{SpanKindInternal, "internal"},
	}
	for _, tt := range tests {
		if got := string(tt.kind); got != tt.want {
			t.Errorf("SpanKind(%v) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestSpanCollector_Record(t *testing.T) {
	c := NewSpanCollector(10)
	now := time.Now()
	s := SpanSummary{
		Name:     "test-span",
		Kind:     SpanKindLLM,
		Duration: 100 * time.Millisecond,
		EndTime:  now,
	}
	c.OnEnd(s)

	spans := c.Snapshot()
	if len(spans) != 1 {
		t.Fatalf("Snapshot() returned %d spans, want 1", len(spans))
	}
	if spans[0].Name != "test-span" {
		t.Errorf("span Name = %q, want %q", spans[0].Name, "test-span")
	}
	if spans[0].Kind != SpanKindLLM {
		t.Errorf("span Kind = %v, want %v", spans[0].Kind, SpanKindLLM)
	}
	if spans[0].Duration != 100*time.Millisecond {
		t.Errorf("span Duration = %v, want %v", spans[0].Duration, 100*time.Millisecond)
	}
}

func TestSpanCollector_Query(t *testing.T) {
	c := NewSpanCollector(100)

	spans := []SpanSummary{
		{Name: "llm-1", Kind: SpanKindLLM, Duration: 50 * time.Millisecond},
		{Name: "tool-1", Kind: SpanKindTool, Duration: 30 * time.Millisecond},
		{Name: "llm-2", Kind: SpanKindLLM, Duration: 70 * time.Millisecond},
		{Name: "agent-1", Kind: SpanKindAgent, Duration: 100 * time.Millisecond},
		{Name: "tool-2", Kind: SpanKindTool, Duration: 20 * time.Millisecond, HasError: true},
		{Name: "llm-3", Kind: SpanKindLLM, Duration: 90 * time.Millisecond},
	}
	for _, s := range spans {
		c.OnEnd(s)
	}

	stats := c.Aggregate()

	if stats.TotalSpans != 6 {
		t.Errorf("Aggregate().TotalSpans = %d, want 6", stats.TotalSpans)
	}

	// Check LLM stats (3 spans, no errors).
	llmStats, ok := stats.ByKind[SpanKindLLM]
	if !ok {
		t.Fatal("ByKind missing SpanKindLLM")
	}
	if llmStats.Count != 3 {
		t.Errorf("LLM Count = %d, want 3", llmStats.Count)
	}
	if llmStats.Errors != 0 {
		t.Errorf("LLM Errors = %d, want 0", llmStats.Errors)
	}
	if llmStats.Avg != 70*time.Millisecond {
		t.Errorf("LLM Avg = %v, want %v", llmStats.Avg, 70*time.Millisecond)
	}

	// Check Tool stats (2 spans, 1 error).
	toolStats, ok := stats.ByKind[SpanKindTool]
	if !ok {
		t.Fatal("ByKind missing SpanKindTool")
	}
	if toolStats.Count != 2 {
		t.Errorf("Tool Count = %d, want 2", toolStats.Count)
	}
	if toolStats.Errors != 1 {
		t.Errorf("Tool Errors = %d, want 1", toolStats.Errors)
	}
	if toolStats.Avg != 25*time.Millisecond {
		t.Errorf("Tool Avg = %v, want %v", toolStats.Avg, 25*time.Millisecond)
	}

	// Check Agent stats (1 span, no errors).
	agentStats, ok := stats.ByKind[SpanKindAgent]
	if !ok {
		t.Fatal("ByKind missing SpanKindAgent")
	}
	if agentStats.Count != 1 {
		t.Errorf("Agent Count = %d, want 1", agentStats.Count)
	}
	if agentStats.Errors != 0 {
		t.Errorf("Agent Errors = %d, want 0", agentStats.Errors)
	}

	if stats.RecentErrors != 1 {
		t.Errorf("Aggregate().RecentErrors = %d, want 1", stats.RecentErrors)
	}
}

func TestSpanCollector_Clear(t *testing.T) {
	c := NewSpanCollector(10)

	// A fresh collector should be empty.
	spans := c.Snapshot()
	if len(spans) != 0 {
		t.Errorf("fresh collector Snapshot() returned %d spans, want 0", len(spans))
	}

	// Record a span and verify it is stored.
	c.OnEnd(SpanSummary{Name: "s1", Kind: SpanKindLLM, Duration: 1 * time.Millisecond})
	if len(c.Snapshot()) != 1 {
		t.Errorf("after recording one span, Snapshot() returned %d, want 1", len(c.Snapshot()))
	}
}

func TestSpanCollector_Overflow(t *testing.T) {
	capacity := 4
	c := NewSpanCollector(capacity)

	// Record more spans than capacity.
	for i := 0; i < 10; i++ {
		c.OnEnd(SpanSummary{
			Name:     "span",
			Kind:     SpanKindInternal,
			Duration: time.Duration(i) * time.Millisecond,
		})
	}

	spans := c.Snapshot()
	if len(spans) != capacity {
		t.Fatalf("Snapshot() after overflow = %d, want %d", len(spans), capacity)
	}

	// The ring buffer retains the last `capacity` spans (durations: 6, 7, 8, 9 ms).
	for i, s := range spans {
		expected := time.Duration(i+6) * time.Millisecond
		if s.Duration != expected {
			t.Errorf("spans[%d].Duration = %v, want %v", i, s.Duration, expected)
		}
	}
}
