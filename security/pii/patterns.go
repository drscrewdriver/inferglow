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

package pii

import "regexp"

// PIIType enumerates the categories of personally identifiable information
// that the masker can recognize and redact.
type PIIType string //nolint:revive

const (
	// Email matches an email address.
	Email PIIType = "email"
	// Phone matches a phone number.
	Phone PIIType = "phone"
	// IDCard matches a national ID card number.
	IDCard PIIType = "id_card"
	// CreditCard matches a credit card number.
	CreditCard PIIType = "credit_card"
	// BankAccount matches a bank account number.
	BankAccount PIIType = "bank_account"
	// IPAddress matches an IPv4 address.
	IPAddress PIIType = "ip_address"
	// Custom marks a caller-supplied pattern.
	Custom PIIType = "custom"
)

// defaultPatternOrder defines the deterministic iteration order used when
// masking. More specific patterns come first so that they take priority
// over the broader BankAccount pattern (which would otherwise swallow
// credit-card and ID-card numbers). Callers that supply a Custom pattern
// should be aware that it is applied last.
var defaultPatternOrder = []PIIType{
	Email,
	Phone,
	IDCard,
	CreditCard,
	IPAddress,
	BankAccount,
	Custom,
}

// DefaultPatterns returns a fresh map of PIIType → compiled regexp covering
// the built-in PII categories. The returned map is a new allocation on
// every call so callers may safely mutate it.
//
// The patterns are intentionally conservative regular expressions; they
// favor precision over recall to avoid false positives on ordinary text.
func DefaultPatterns() map[PIIType]*regexp.Regexp {
	return map[PIIType]*regexp.Regexp{
		Email:       regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`),
		Phone:       regexp.MustCompile(`1[3-9]\d{9}`),
		IDCard:      regexp.MustCompile(`\d{17}[\dXx]`),
		CreditCard:  regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),
		BankAccount: regexp.MustCompile(`\b\d{16,19}\b`),
		IPAddress:   regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
	}
}

// orderedPIITypes returns the PII types present in patterns sorted so that
// the most specific patterns are applied first. This guarantees stable
// masking output regardless of map iteration order.
func orderedPIITypes(patterns map[PIIType]*regexp.Regexp) []PIIType {
	out := make([]PIIType, 0, len(patterns))
	for _, t := range defaultPatternOrder {
		if re, ok := patterns[t]; ok && re != nil {
			out = append(out, t)
		}
	}
	return out
}
