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
