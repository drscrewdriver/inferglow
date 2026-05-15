package pii

import "regexp"

// PIIType enumerates the categories of personally identifiable information
// that the masker can recognize and redact.
type PIIType string

const (
	Email       PIIType = "email"
	Phone       PIIType = "phone"
	IDCard      PIIType = "id_card"
	CreditCard  PIIType = "credit_card"
	BankAccount PIIType = "bank_account"
	IPAddress   PIIType = "ip_address"
	Custom      PIIType = "custom"
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
