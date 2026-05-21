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
	"strings"
	"testing"
	"time"
)

func newTestChain(t *testing.T, n int) *AuditChain {
	t.Helper()
	cfg := AuditConfig{Enabled: true, StorageBackend: "memory"}
	c, err := NewAuditChain(cfg)
	if err != nil {
		t.Fatalf("NewAuditChain: %v", err)
	}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i := 0
	c.SetClock(func() time.Time {
		tt := t0.Add(time.Duration(i) * time.Second)
		i++
		return tt
	})
	for k := 0; k < n; k++ {
		if _, err := c.Append(&AuditEntry{
			Source: "agent",
			Action: "decision",
			Input:  k,
			Output: k * 10,
		}); err != nil {
			t.Fatalf("Append[%d]: %v", k, err)
		}
	}
	return c
}

func TestVerifyChain_ValidChain(t *testing.T) {
	c := newTestChain(t, 5)
	if err := c.VerifyChain(); err != nil {
		t.Fatalf("VerifyChain on valid chain: %v", err)
	}
}

func TestVerifyChain_EmptyChain(t *testing.T) {
	c := newTestChain(t, 0)
	if err := c.VerifyChain(); err != nil {
		t.Fatalf("VerifyChain on empty chain should be nil, got %v", err)
	}
}

func TestVerifyChain_DetectsHashTamper(t *testing.T) {
	c := newTestChain(t, 5)
	// Tamper with the 3rd entry's Hash.
	c.mu.Lock()
	c.entries[2].Hash = "deadbeef"
	c.mu.Unlock()

	err := c.VerifyChain()
	if err == nil {
		t.Fatal("VerifyChain must report tamper")
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected *VerifyError, got %T: %v", err, err)
	}
	if ve.Index != 2 {
		t.Fatalf("expected Index=2, got %d", ve.Index)
	}
	if !strings.Contains(ve.Reason, "hash") {
		t.Fatalf("expected reason containing 'hash', got %q", ve.Reason)
	}
}

func TestVerifyChain_DetectsPrevHashMismatch(t *testing.T) {
	c := newTestChain(t, 4)
	// Mutate entry 2's PrevHash so it no longer matches entry 1's Hash.
	c.mu.Lock()
	c.entries[2].PrevHash = "not-the-real-prev"
	c.mu.Unlock()

	err := c.VerifyChain()
	if err == nil {
		t.Fatal("VerifyChain must report prev_hash mismatch")
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected *VerifyError, got %T", err)
	}
	if ve.Index != 2 {
		t.Fatalf("expected Index=2, got %d", ve.Index)
	}
	if !strings.Contains(ve.Reason, "prev_hash") {
		t.Fatalf("expected reason containing 'prev_hash', got %q", ve.Reason)
	}
}

func TestVerifyChain_DetectsContentTamper(t *testing.T) {
	c := newTestChain(t, 3)
	// Tamper with entry 1's content but leave its Hash unchanged —
	// recomputed Hash will differ and verification should fail.
	c.mu.Lock()
	c.entries[1].Input = "tampered"
	c.mu.Unlock()

	err := c.VerifyChain()
	if err == nil {
		t.Fatal("VerifyChain must detect content tamper via hash recomputation")
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected *VerifyError, got %T", err)
	}
	if ve.Index != 1 {
		t.Fatalf("expected Index=1, got %d", ve.Index)
	}
}

func TestVerifyChain_SignatureRoundTrip(t *testing.T) {
	cfg := AuditConfig{
		Enabled:      true,
		SignatureKey: []byte("secret"),
	}
	c, _ := NewAuditChain(cfg)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return t0 })
	for i := 0; i < 3; i++ {
		_, _ = c.Append(&AuditEntry{
			Source: "agent",
			Action: "decision",
			Input:  i,
		})
	}
	if err := c.VerifyChain(); err != nil {
		t.Fatalf("VerifyChain with valid signatures: %v", err)
	}
}

func TestVerifyChain_SignatureTamper(t *testing.T) {
	cfg := AuditConfig{
		Enabled:      true,
		SignatureKey: []byte("secret"),
	}
	c, _ := NewAuditChain(cfg)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return t0 })
	for i := 0; i < 3; i++ {
		_, _ = c.Append(&AuditEntry{Source: "agent", Action: "decision", Input: i})
	}
	// Mutate the 2nd entry's signature.
	c.mu.Lock()
	c.entries[1].Signature = "bad"
	c.mu.Unlock()

	err := c.VerifyChain()
	if err == nil {
		t.Fatal("VerifyChain must reject bad signature")
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected *VerifyError, got %T", err)
	}
	if !strings.Contains(ve.Reason, "signature") {
		t.Fatalf("expected reason containing 'signature', got %q", ve.Reason)
	}
}

func TestVerifyError_Format(t *testing.T) {
	e := &VerifyError{Index: 7, Reason: "hash mismatch"}
	s := e.Error()
	if !strings.Contains(s, "7") || !strings.Contains(s, "hash mismatch") {
		t.Fatalf("Error() = %q, want index and reason", s)
	}
}
