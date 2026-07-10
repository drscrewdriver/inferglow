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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// AuditChain is an append-only, hash-chained audit log. The zero value
// is not usable; obtain an instance via NewAuditChain.
//
// All public methods are safe for concurrent use by multiple goroutines.
type AuditChain struct { //nolint:revive
	mu       sync.RWMutex
	cfg      AuditConfig
	entries  []*AuditEntry
	lastHash string
	storage  Storage
	seq      int64
	clock    func() time.Time // injectable for deterministic tests
}

// NewAuditChain constructs an AuditChain from cfg. If cfg.Enabled is
// false the chain is still usable but Append is a no-op that returns
// ("", nil), matching the spec's "default off, zero overhead" rule.
//
// The Storage implementation is selected from cfg.StorageBackend:
//   - "memory" (default): in-memory slice, no persistence
//   - "json_file":        daily-rotated JSONL files under cfg.StoragePath
//
// Unknown backends fall back to "memory" so a misconfigured chain never
// panics at construction time.
func NewAuditChain(cfg AuditConfig) (*AuditChain, error) {
	c := &AuditChain{
		cfg:   cfg,
		clock: time.Now,
	}
	c.storage = newStorageFor(cfg)
	return c, nil
}

// newStorageFor picks the concrete Storage implementation for the chain
// based on the (already-normalized) StorageBackend field. It is called
// once at construction; tests can replace c.storage afterwards.
func newStorageFor(cfg AuditConfig) Storage {
	switch storageBackendFor(cfg) {
	case "json_file":
		return NewJSONFileStorage(cfg.StoragePath)
	default:
		return NewMemoryStorage()
	}
}

// SetClock replaces the function used to stamp new entries' Timestamp.
// Intended for tests that need deterministic IDs and hashes.
func (c *AuditChain) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock = now
}

// IsEnabled reports whether Append will actually record entries.
func (c *AuditChain) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.Enabled
}

// Append records entry on the chain. It:
//   - Returns ("", nil) immediately if cfg.Enabled is false (zero overhead).
//   - Auto-fills ID, Timestamp, PrevHash and Hash if they are zero.
//   - Computes an HMAC-SHA256 Signature if cfg.SignatureKey is non-empty.
//   - Updates lastHash to entry.Hash.
//   - Calls storage.Save(entry) for persistence (if storage is non-nil).
//
// Returns the entry's Hash (which callers can use as a correlation ID)
// and any error from the storage layer.
func (c *AuditChain) Append(entry *AuditEntry) (string, error) {
	if entry == nil {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cfg.Enabled {
		return "", nil
	}

	// Snapshot the clock once so ID and Timestamp share the same instant.
	now := c.clock()
	// Fill in ID if missing.
	if entry.ID == "" {
		seq := atomic.AddInt64(&c.seq, 1)
		entry.ID = fmt.Sprintf("audit-%d-%d", now.UnixNano(), seq)
	}
	// Fill in Timestamp if missing.
	if entry.Timestamp.IsZero() {
		entry.Timestamp = now.UTC()
	}
	// Fill in PrevHash (chain to previous entry).
	if entry.PrevHash == "" {
		entry.PrevHash = c.lastHash
	}
	// Compute Hash last so it covers all chained fields.
	if entry.Hash == "" {
		entry.Hash = ComputeHash(entry)
	}
	// Sign if a key is configured.
	if len(c.cfg.SignatureKey) > 0 {
		SignEntry(entry, c.cfg.SignatureKey)
	}

	c.entries = append(c.entries, entry)
	c.lastHash = entry.Hash

	// Honor MaxEntries by dropping the oldest (this is a soft cap; it
	// does not rewrite history — the dropped entry's hash chain is
	// preserved via lastHash which points to the new tail).
	if c.cfg.MaxEntries > 0 && len(c.entries) > c.cfg.MaxEntries {
		drop := len(c.entries) - c.cfg.MaxEntries
		c.entries = c.entries[drop:]
	}

	if c.storage != nil {
		if err := c.storage.Save(entry); err != nil {
			return entry.Hash, err
		}
	}
	return entry.Hash, nil
}

// Len returns the number of entries currently held in memory.
func (c *AuditChain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// snapshot returns a defensive copy of the entries slice under the read
// lock. Callers may iterate the returned slice without holding the lock.
func (c *AuditChain) snapshot() []*AuditEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*AuditEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

// Snapshot returns a defensive copy of all audit entries.
// This is the exported version for external consumers (e.g. REST API).
func (c *AuditChain) Snapshot() []*AuditEntry {
	return c.snapshot()
}

// lastEntry returns a pointer to the most recently appended entry, or
// nil if the chain is empty. Caller must hold the read lock (or write
// lock) — this helper does not take the lock itself.
func (c *AuditChain) lastEntry() *AuditEntry {
	if len(c.entries) == 0 {
		return nil
	}
	return c.entries[len(c.entries)-1]
}

// ErrChainDisabled is returned by operations that require an enabled
// chain but were invoked on a disabled one. (Currently informational —
// most ops silently no-op instead.)
var ErrChainDisabled = errors.New("audit chain disabled")

// compile-time interface satisfaction.
var _ AuditHook = (*AuditChain)(nil)
