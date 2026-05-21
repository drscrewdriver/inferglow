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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Export writes all entries in the chain to w in the requested format.
// Supported formats: ExportJSON (a JSON array), ExportCSV (one row per
// entry with fixed columns), ExportText (human-readable aligned rows).
//
// Export does not lock the chain for the duration of the write — it
// snapshots the entries first and then serializes the snapshot, so a
// concurrent Append will not corrupt the output.
func (c *AuditChain) Export(format ExportFormat, w io.Writer) error {
	entries := c.snapshot()
	switch format {
	case ExportJSON, "":
		return exportJSON(entries, w)
	case ExportCSV:
		return exportCSV(entries, w)
	case ExportText:
		return exportText(entries, w)
	default:
		return fmt.Errorf("audit: unknown export format %q", format)
	}
}

func exportJSON(entries []*AuditEntry, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// exportCSV emits one row per entry with the columns:
// ID, Timestamp, Source, Action, Hash, PrevHash.
// Non-scalar fields (Input, Output, Metadata) are omitted for clarity —
// they may contain commas/newlines that complicate CSV consumption.
func exportCSV(entries []*AuditEntry, w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"ID", "Timestamp", "Source", "Action", "Hash", "PrevHash"}); err != nil {
		return err
	}
	for _, e := range entries {
		row := []string{
			e.ID,
			formatTimestamp(e.Timestamp),
			e.Source,
			e.Action,
			e.Hash,
			e.PrevHash,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func exportText(entries []*AuditEntry, w io.Writer) error {
	const hdr = "%-20s %-25s %-10s %-12s %s\n"
	if _, err := fmt.Fprintf(w, hdr, "ID", "Timestamp", "Source", "Action", "Hash"); err != nil {
		return err
	}
	for _, e := range entries {
		id := e.ID
		if len(id) > 20 {
			id = id[:17] + "..."
		}
		if _, err := fmt.Fprintf(w, hdr,
			id,
			formatTimestamp(e.Timestamp),
			e.Source,
			e.Action,
			e.Hash,
		); err != nil {
			return err
		}
	}
	return nil
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
