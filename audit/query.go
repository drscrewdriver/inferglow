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

// Query returns every entry in the chain matching filter, in
// chronological (append) order. A zero-valued QueryFilter matches all
// entries.
//
// Matching rules:
//   - Source: non-empty filter value must equal entry.Source.
//   - Action: non-empty filter value must equal entry.Action.
//   - From:   non-zero filter value → entry.Timestamp >= From.
//   - To:     non-zero filter value → entry.Timestamp <= To.
//   - Metadata: every key/value pair in filter.Metadata must be present
//     in entry.Metadata with the same value.
func (c *AuditChain) Query(filter QueryFilter) ([]*AuditEntry, error) {
	entries := c.snapshot()

	out := make([]*AuditEntry, 0, len(entries))
	for _, e := range entries {
		if !matchFilter(e, filter) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// matchFilter returns true iff e satisfies every non-zero field of f.
func matchFilter(e *AuditEntry, f QueryFilter) bool {
	if f.Source != "" && e.Source != f.Source {
		return false
	}
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if !f.From.IsZero() && e.Timestamp.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && e.Timestamp.After(f.To) {
		return false
	}
	for k, v := range f.Metadata {
		if e.Metadata == nil {
			return false
		}
		got, ok := e.Metadata[k]
		if !ok || got != v {
			return false
		}
	}
	return true
}
