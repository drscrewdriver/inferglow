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
	"time"
)

// AuditEntry is a single record in the audit chain. Each entry's Hash
// chains to the previous entry's Hash via PrevHash, producing an
// append-only, tamper-evident log.
type AuditEntry struct { //nolint:revive
	PrevHash  string            `json:"prev_hash"`
	Hash      string            `json:"hash"`
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Source    string            `json:"source"`           // "agent"|"action"|"model"|"flow"
	Action    string            `json:"action"`           // "decision"|"execute"|"request"
	Input     any               `json:"input,omitempty"`  // arbitrary JSON-serializable value
	Output    any               `json:"output,omitempty"` // arbitrary JSON-serializable value
	Duration  time.Duration     `json:"duration"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Signature string            `json:"signature,omitempty"`
}

// AuditConfig controls an AuditChain's runtime behavior. Zero-value
// AuditConfig disables auditing (Enabled defaults to false).
type AuditConfig struct { //nolint:revive
	Enabled        bool
	SignatureKey   []byte // optional HMAC-SHA256 key, nil disables signing
	StorageBackend string // "memory" (default) or "json_file"
	StoragePath    string // directory used when StorageBackend == "json_file"
	MaxEntries     int    // 0 = unlimited
}

// AuditHook is the lightweight interface that call sites (agent engine,
// dispatcher, etc.) depend on to avoid pulling the full audit.AuditChain
// type into their packages. NoOpHook is the zero-overhead default.
type AuditHook interface { //nolint:revive
	Append(entry *AuditEntry) (string, error)
	IsEnabled() bool
}

// QueryFilter narrows the entries returned by AuditChain.Query. A zero
// value on any field means "do not filter on this field".
type QueryFilter struct {
	Source   string
	Action   string
	From     time.Time
	To       time.Time
	Metadata map[string]string
}

// ExportFormat selects the serialization format used by AuditChain.Export.
type ExportFormat string

const (
	// ExportJSON serializes audit entries as a JSON array.
	ExportJSON ExportFormat = "json"
	// ExportCSV serializes audit entries as comma-separated values.
	ExportCSV ExportFormat = "csv"
	// ExportText serializes audit entries as human-readable text.
	ExportText ExportFormat = "text"
)

// VerifyError is returned by AuditChain.VerifyChain when an entry fails
// recomputation or chain-continuity checks. Index is the position of
// the offending entry within the chain.
type VerifyError struct {
	Index  int
	Reason string
}

func (e *VerifyError) Error() string {
	return "audit verify error at index " + itoa(e.Index) + ": " + e.Reason
}

// Storage persists audit entries. Implementations must be safe for
// concurrent use by multiple goroutines.
type Storage interface {
	Save(entry *AuditEntry) error
	LoadAll() ([]*AuditEntry, error)
}

// itoa is a small stdlib-only int → string helper so types.go does not
// need to import strconv just for one Error() formatting call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
