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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignEntry computes HMAC-SHA256(key, entry.Hash), assigns the result to
// entry.Signature, and returns the hex-encoded signature. The signature
// covers only the entry's Hash because the Hash itself is a SHA-256
// digest of the full entry content — by transitively signing the hash,
// SignEntry transitively signs every field that contributed to ComputeHash.
//
// If key is nil/empty, hmac.New still operates (treating it as a zero
// length key); callers should treat an empty key as "signing disabled".
func SignEntry(entry *AuditEntry, key []byte) string {
	if entry == nil {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(entry.Hash))
	sig := hex.EncodeToString(mac.Sum(nil))
	entry.Signature = sig
	return sig
}

// VerifyEntry returns true iff the entry's Signature is a valid
// HMAC-SHA256(key, entry.Hash). A constant-time comparison is used so
// the verification is not vulnerable to timing leaks.
func VerifyEntry(entry *AuditEntry, key []byte) bool {
	if entry == nil || entry.Signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(entry.Hash))
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(entry.Signature)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}
