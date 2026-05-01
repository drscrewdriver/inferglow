package audit

import (
	"testing"
	"time"
)

func TestSignEntry_VerifyRoundTrip(t *testing.T) {
	key := []byte("super-secret-key")
	e := &AuditEntry{
		Hash:      "deadbeef",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Source:    "agent",
		Action:    "decision",
		Input:     "ping",
		Output:    "pong",
	}
	sig := SignEntry(e, key)
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if !VerifyEntry(e, key) {
		t.Fatal("VerifyEntry should accept freshly-signed entry")
	}
}

func TestSignEntry_TamperDetection(t *testing.T) {
	key := []byte("k")
	e := &AuditEntry{
		Hash:   "h1",
		Source: "agent",
		Action: "decision",
	}
	e.Signature = SignEntry(e, key)

	// Mutate Hash → signature must no longer verify.
	orig := e.Hash
	e.Hash = "tampered"
	if VerifyEntry(e, key) {
		t.Fatal("VerifyEntry must reject entry with mutated Hash")
	}
	e.Hash = orig

	// Mutate signature.
	e.Signature = "00"
	if VerifyEntry(e, key) {
		t.Fatal("VerifyEntry must reject entry with mutated Signature")
	}
}

func TestSignEntry_DifferentKeys(t *testing.T) {
	e := &AuditEntry{Hash: "h", Source: "agent"}
	sig1 := SignEntry(e, []byte("key1"))
	state1 := e.Signature
	sig2 := SignEntry(e, []byte("key2"))
	state2 := e.Signature
	if sig1 == sig2 || state1 == state2 {
		t.Fatal("different keys must produce different signatures")
	}
	// Verify each signature with the matching key by temporarily restoring
	// the signature on the entry — SignEntry mutates entry.Signature.
	e.Signature = state1
	if !VerifyEntry(e, []byte("key1")) {
		t.Fatal("verify with key1 should pass")
	}
	e.Signature = state2
	if VerifyEntry(e, []byte("key1")) {
		t.Fatal("verify with wrong key should fail")
	}
}

func TestSignEntry_EmptyKey(t *testing.T) {
	e := &AuditEntry{Hash: "h", Source: "agent"}
	// Empty key: SignEntry still produces a value (HMAC over empty key),
	// and VerifyEntry with the same empty key must accept it.
	sig := SignEntry(e, nil)
	if sig == "" {
		t.Fatal("expected non-empty signature even with nil key")
	}
	if !VerifyEntry(e, nil) {
		t.Fatal("verify with nil key should match sign with nil key")
	}
}
