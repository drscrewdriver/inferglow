package audit

// NoOpHook is the default AuditHook implementation used when auditing is
// disabled. Both methods are zero-overhead no-ops.
type NoOpHook struct{}

// Append returns ("", nil) immediately without recording the entry.
func (h *NoOpHook) Append(entry *AuditEntry) (string, error) { return "", nil }

// IsEnabled always returns false.
func (h *NoOpHook) IsEnabled() bool { return false }
