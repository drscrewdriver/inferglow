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
	"sync"
	"testing"
	"time"
)

func TestNewAuditChain_DefaultDisabled(t *testing.T) {
	c, err := NewAuditChain(DefaultAuditConfig())
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	if c.IsEnabled() {
		t.Fatal("default config must be disabled")
	}
	// Append must be a no-op returning ("", nil).
	hash, err := c.Append(&AuditEntry{Source: "agent", Action: "decision"})
	if err != nil {
		t.Fatalf("Append on disabled chain: %v", err)
	}
	if hash != "" {
		t.Fatalf("Append on disabled chain should return empty hash, got %q", hash)
	}
	if c.Len() != 0 {
		t.Fatalf("Append on disabled chain should not record: Len=%d", c.Len())
	}
}

func TestAuditChain_AppendAndLen(t *testing.T) {
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory"}
	c, err := NewAuditChain(cfg)
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	// Use a deterministic clock so IDs and hashes are reproducible.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return t0 })

	const N = 5
	hashes := make([]string, 0, N)
	for i := 0; i < N; i++ {
		h, err := c.Append(&AuditEntry{
			Source: "agent",
			Action: "decision",
			Input:  i,
			Output: i * 2,
		})
		if err != nil {
			t.Fatalf("Append[%d]: %v", i, err)
		}
		if h == "" {
			t.Fatalf("Append[%d] returned empty hash", i)
		}
		hashes = append(hashes, h)
	}

	if c.Len() != N {
		t.Fatalf("Len=%d want %d", c.Len(), N)
	}
}

func TestAuditChain_ChainContinuity(t *testing.T) {
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory"}
	c, _ := NewAuditChain(cfg)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return t0 })

	var prevHash string
	for i := 0; i < 4; i++ {
		h, _ := c.Append(&AuditEntry{
			Source: "agent",
			Action: "decision",
			Input:  i,
		})
		if h == "" {
			t.Fatalf("Append[%d] returned empty hash", i)
		}
		// Manually verify the chain continuity invariant.
		snap := c.snapshot()
		last := snap[len(snap)-1]
		if last.PrevHash != prevHash {
			t.Fatalf("entry %d: PrevHash=%q want %q", i, last.PrevHash, prevHash)
		}
		if last.Hash != h {
			t.Fatalf("entry %d: stored Hash=%q != Append return %q", i, last.Hash, h)
		}
		prevHash = h
	}
}

func TestAuditChain_IDUniqueness(t *testing.T) {
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory"}
	c, _ := NewAuditChain(cfg)
	// Distinct clock ticks → distinct IDs.
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		t := t0.Add(time.Duration(i) * time.Second)
		i++
		return t
	})

	seen := make(map[string]bool)
	for n := 0; n < 10; n++ {
		_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision"})
	}
	snap := c.snapshot()
	for i, e := range snap {
		if e.ID == "" {
			t.Fatalf("entry %d: empty ID", i)
		}
		if seen[e.ID] {
			t.Fatalf("duplicate ID: %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestAuditChain_AuditHookInterface(t *testing.T) {
	// AuditChain must satisfy AuditHook.
	var _ AuditHook = (*AuditChain)(nil)
}

func TestAuditChain_SignatureOnAppend(t *testing.T) {
	cfg := AuditConfig{
		Enabled:      true,
		SignatureKey: []byte("topsecret"),
	}
	c, _ := NewAuditChain(cfg)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return t0 })

	_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: "x"})
	snap := c.snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(snap))
	}
	if snap[0].Signature == "" {
		t.Fatal("Append with SignatureKey should populate entry.Signature")
	}
	if !VerifyEntry(snap[0], cfg.SignatureKey) {
		t.Fatal("VerifyEntry should accept Append-produced signature")
	}
}

func TestAuditChain_ConcurrentAppend(t *testing.T) {
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory"}
	c, _ := NewAuditChain(cfg)

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(n int) {
			defer wg.Done()
			_, _ = c.Append(&AuditEntry{
				Source: "agent",
				Action: "decision",
				Input:  n,
			})
		}(i)
	}
	wg.Wait()

	if c.Len() != N {
		t.Fatalf("expected Len=%d after concurrent Append, got %d", N, c.Len())
	}
	if err := c.VerifyChain(); err != nil {
		t.Fatalf("VerifyChain after concurrent Append: %v", err)
	}
}

func TestAuditChain_MaxEntriesSoftCap(t *testing.T) {
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory", MaxEntries: 3}
	c, _ := NewAuditChain(cfg)
	for i := 0; i < 5; i++ {
		_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: i})
	}
	if c.Len() != 3 {
		t.Fatalf("MaxEntries=3 should cap Len to 3, got %d", c.Len())
	}
}
