package audit

// VerifyError reasons returned by VerifyChain.
const (
	reasonHashMismatch      = "hash mismatch"
	reasonPrevHashMismatch  = "prev_hash mismatch"
	reasonSignatureMismatch = "signature mismatch"
)

// VerifyChain walks every entry in the chain and verifies:
//  1. The recomputed ComputeHash(entry) equals entry.Hash.
//  2. For i > 0, entry.PrevHash equals the previous entry's Hash.
//  3. If cfg.SignatureKey is non-empty, entry.Signature is a valid
//     HMAC-SHA256(key, entry.Hash).
//
// On the first failure VerifyChain returns a *VerifyError identifying
// the offending entry index and the failure reason. A nil error means
// every entry passed all checks.
func (c *AuditChain) VerifyChain() error {
	entries := c.snapshot()
	hasKey := len(c.cfg.SignatureKey) > 0

	for i, e := range entries {
		if e == nil {
			return &VerifyError{Index: i, Reason: "nil entry"}
		}
		// (1) prev_hash continuity — checked BEFORE hash recomputation so
		// that tampering with PrevHash is reported as a prev_hash mismatch
		// rather than as a hash mismatch (PrevHash is part of the hash
		// preimage, so tampering with it would also fail the hash check,
		// but the more specific error message is more useful to callers).
		if i > 0 {
			prev := entries[i-1]
			if e.PrevHash != prev.Hash {
				return &VerifyError{Index: i, Reason: reasonPrevHashMismatch}
			}
		}
		// For the head entry (i == 0) PrevHash may legitimately be "" (the
		// chain root) — no continuity check applies. A non-empty PrevHash
		// on the head would indicate the head was sliced off by MaxEntries,
		// which we accept since the hash check below still validates the
		// entry's internal integrity.
		// (2) hash recomputation
		if got := ComputeHash(e); got != e.Hash {
			return &VerifyError{Index: i, Reason: reasonHashMismatch}
		}
		// (3) signature
		if hasKey {
			if !VerifyEntry(e, c.cfg.SignatureKey) {
				return &VerifyError{Index: i, Reason: reasonSignatureMismatch}
			}
		}
	}
	return nil
}
