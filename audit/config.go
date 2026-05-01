package audit

// DefaultAuditConfig returns the zero-impact baseline configuration:
// auditing disabled, in-memory storage, no signature key, unlimited
// entries. Callers may override individual fields before passing the
// config to NewAuditChain.
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:        false,
		SignatureKey:   nil,
		StorageBackend: "memory",
		StoragePath:    "",
		MaxEntries:     0,
	}
}

// storageBackendFor normalizes the StorageBackend field: an empty string
// is treated as "memory". Any unrecognized value also falls back to
// "memory" so a misconfigured chain never panics at construction time.
func storageBackendFor(cfg AuditConfig) string {
	switch cfg.StorageBackend {
	case "memory", "":
		return "memory"
	case "json_file":
		return "json_file"
	default:
		return "memory"
	}
}
