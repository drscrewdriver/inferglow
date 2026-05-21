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
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExport_JSON(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: "a"})
	_, _ = c.Append(&AuditEntry{Source: "action", Action: "execute", Input: "b"})

	var buf bytes.Buffer
	if err := c.Export(ExportJSON, &buf); err != nil {
		t.Fatalf("Export JSON: %v", err)
	}

	// Must be a valid JSON array of 2 entries.
	var got []*AuditEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Source != "agent" || got[1].Source != "action" {
		t.Fatalf("unexpected sources: %+v", got)
	}
}

func TestExport_CSV(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision"})
	_, _ = c.Append(&AuditEntry{Source: "action", Action: "execute"})

	var buf bytes.Buffer
	if err := c.Export(ExportCSV, &buf); err != nil {
		t.Fatalf("Export CSV: %v", err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d:\n%s", len(lines), out)
	}
	// Header should contain the expected columns.
	for _, col := range []string{"ID", "Timestamp", "Source", "Action", "Hash", "PrevHash"} {
		if !strings.Contains(lines[0], col) {
			t.Fatalf("header missing column %q: %s", col, lines[0])
		}
	}
	// Rows should reference the sources we appended.
	if !strings.Contains(lines[1], "agent") {
		t.Fatalf("row 1 missing 'agent': %s", lines[1])
	}
	if !strings.Contains(lines[2], "action") {
		t.Fatalf("row 2 missing 'action': %s", lines[2])
	}
}

func TestExport_Text(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})
	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision"})

	var buf bytes.Buffer
	if err := c.Export(ExportText, &buf); err != nil {
		t.Fatalf("Export Text: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "Timestamp") {
		t.Fatalf("text output missing header: %s", out)
	}
	if !strings.Contains(out, "agent") || !strings.Contains(out, "decision") {
		t.Fatalf("text output missing entry data: %s", out)
	}
}

func TestExport_EmptyChain(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	for _, fmt := range []ExportFormat{ExportJSON, ExportCSV, ExportText} {
		var buf bytes.Buffer
		if err := c.Export(fmt, &buf); err != nil {
			t.Fatalf("Export %s on empty chain: %v", fmt, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("Export %s produced empty output", fmt)
		}
	}
}

func TestExport_UnknownFormat(t *testing.T) {
	c, _ := NewAuditChain(AuditConfig{Enabled: true})
	var buf bytes.Buffer
	if err := c.Export(ExportFormat("yaml"), &buf); err == nil {
		t.Fatal("Export with unknown format should error")
	}
}
