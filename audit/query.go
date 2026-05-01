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
