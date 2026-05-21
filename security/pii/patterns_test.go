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

import (
	"testing"
)

func TestDefaultPatterns_ContainsAllBuiltinTypes(t *testing.T) {
	p := DefaultPatterns()
	required := []PIIType{Email, Phone, IDCard, CreditCard, BankAccount, IPAddress}
	for _, pt := range required {
		re, ok := p[pt]
		if !ok {
			t.Errorf("DefaultPatterns missing %q", pt)
			continue
		}
		if re == nil {
			t.Errorf("DefaultPatterns[%q] is nil", pt)
		}
	}
	// Custom is intentionally not present in the default set.
	if _, ok := p[Custom]; ok {
		t.Errorf("DefaultPatterns should not include Custom")
	}
}

func TestDefaultPatterns_ReturnsIndependentMap(t *testing.T) {
	a := DefaultPatterns()
	b := DefaultPatterns()
	if len(a) != len(b) {
		t.Fatalf("DefaultPatterns returned maps of different size: %d vs %d", len(a), len(b))
	}
	// Mutating one must not affect the other (fresh allocation per call).
	a[Custom] = a[Email]
	if _, ok := b[Custom]; ok {
		t.Fatal("mutating one DefaultPatterns map leaked into another call")
	}
}

func TestDefaultPatterns_Email(t *testing.T) {
	p := DefaultPatterns()
	cases := []struct {
		in    string
		match bool
	}{
		{"user@example.com", true},
		{"first.last+tag@sub.domain.co", true},
		{"not-an-email", false},
		{"plain text without email", false},
	}
	for _, c := range cases {
		if p[Email].MatchString(c.in) != c.match {
			t.Errorf("Email match %q: got %v, want %v", c.in, !c.match, c.match)
		}
	}
}

func TestDefaultPatterns_Phone(t *testing.T) {
	p := DefaultPatterns()
	valid := []string{"13812345678", "15901234567", "18600000000"}
	for _, s := range valid {
		if !p[Phone].MatchString(s) {
			t.Errorf("expected Phone to match %q", s)
		}
	}
	invalid := []string{"12345678901", "1234567890", "1381234567"}
	for _, s := range invalid {
		if p[Phone].MatchString(s) {
			t.Errorf("expected Phone to NOT match %q", s)
		}
	}
}

func TestDefaultPatterns_IDCard(t *testing.T) {
	p := DefaultPatterns()
	valid := []string{"11010119900307888X", "11010119900307888x", "110101199003078888"}
	for _, s := range valid {
		if !p[IDCard].MatchString(s) {
			t.Errorf("expected IDCard to match %q", s)
		}
	}
}

func TestDefaultPatterns_CreditCard(t *testing.T) {
	p := DefaultPatterns()
	valid := []string{
		"1234567812345678",
		"1234-5678-1234-5678",
		"1234 5678 1234 5678",
	}
	for _, s := range valid {
		if !p[CreditCard].MatchString(s) {
			t.Errorf("expected CreditCard to match %q", s)
		}
	}
}

func TestDefaultPatterns_BankAccount(t *testing.T) {
	p := DefaultPatterns()
	valid := []string{
		"6222020200011111",
		"6222020200011111222",
	}
	for _, s := range valid {
		if !p[BankAccount].MatchString(s) {
			t.Errorf("expected BankAccount to match %q", s)
		}
	}
}

func TestDefaultPatterns_IPAddress(t *testing.T) {
	p := DefaultPatterns()
	valid := []string{"192.168.1.1", "10.0.0.1", "255.255.255.255"}
	for _, s := range valid {
		if !p[IPAddress].MatchString(s) {
			t.Errorf("expected IPAddress to match %q", s)
		}
	}
}

func TestOrderedPIITypes_DeterministicOrder(t *testing.T) {
	p := DefaultPatterns()
	order := orderedPIITypes(p)
	// Email must come before BankAccount so the broad digit pattern does
	// not interfere with more specific matches.
	emailIdx, bankIdx := -1, -1
	for i, t := range order {
		if t == Email {
			emailIdx = i
		}
		if t == BankAccount {
			bankIdx = i
		}
	}
	if emailIdx < 0 || bankIdx < 0 {
		t.Fatalf("expected both Email and BankAccount in order, got %v", order)
	}
	if emailIdx > bankIdx {
		t.Errorf("Email should be applied before BankAccount; got order %v", order)
	}
}
