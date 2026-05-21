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

package audit

import (
	"testing"
	"time"
)

func TestQuery_BySourceAndAction(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})

	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: 1})
	_, _ = c.Append(&AuditEntry{Source: "action", Action: "execute", Input: 2})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: 3})
	_, _ = c.Append(&AuditEntry{Source: "model", Action: "request", Input: 4})

	got, err := c.Query(QueryFilter{Source: "agent", Action: "decision"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 agent/decision entries, got %d", len(got))
	}
	// Should be in chronological order.
	if got[0].Input.(int) != 1 || got[1].Input.(int) != 3 {
		t.Fatalf("Query result order wrong: %v %v", got[0].Input, got[1].Input)
	}
}

func TestQuery_ByTimeRange(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})

	for k := 0; k < 5; k++ {
		_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: k})
	}

	from := t0.Add(1 * time.Second)
	to := t0.Add(3 * time.Second)
	got, err := c.Query(QueryFilter{From: from, To: to})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Inclusive on both ends: entries at seconds 1, 2, 3 → 3 entries.
	if len(got) != 3 {
		t.Fatalf("expected 3 entries in [1s,3s], got %d", len(got))
	}
}

func TestQuery_ByMetadata(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})

	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Metadata: map[string]string{"session_id": "s1"}})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Metadata: map[string]string{"session_id": "s2"}})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Metadata: map[string]string{"session_id": "s1", "round": "1"}})

	got, err := c.Query(QueryFilter{Metadata: map[string]string{"session_id": "s1"}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries with session_id=s1, got %d", len(got))
	}
}

func TestQuery_EmptyFilter(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})

	for k := 0; k < 3; k++ {
		_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision"})
	}
	got, err := c.Query(QueryFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("empty filter should return all entries, got %d", len(got))
	}
}

func TestQuery_NoMatches(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision"})
	got, err := c.Query(QueryFilter{Source: "nonexistent"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(got))
	}
}
