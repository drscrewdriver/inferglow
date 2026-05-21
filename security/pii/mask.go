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

// MaskOn is a bit-flag describing when masking should be applied. Multiple
// flags can be combined with the bitwise OR operator (e.g. MaskOnInput |
// MaskOnOutput) to enable masking on both sides of the conversation.
type MaskOn int

const (
	// MaskOnInput applies masking to messages before they enter the
	// session history (user input, assistant output stored for context).
	MaskOnInput MaskOn = 1 << iota
	// MaskOnOutput applies masking to the final response before it is
	// returned to the caller.
	MaskOnOutput
)

// MaskConfig controls how a Masker redacts PII. The zero value is a safe
// no-op configuration: no patterns, the default mask character, and
// ApplyOn == 0 (masking disabled).
type MaskConfig struct {
	// Patterns maps a PII type to its compiled regex. When nil or empty,
	// DefaultPatterns() is used at construction time.
	Patterns map[PIIType]*regexp.Regexp
	// MaskChar is the replacement string written in place of the
	// redacted tail. Defaults to "***" when empty.
	MaskChar string
	// KeepPrefix is the number of leading characters preserved from each
	// match; the remainder is replaced with MaskChar. A zero value
	// replaces the entire match. A value greater than the match length
	// leaves the match untouched.
	KeepPrefix int
	// ApplyOn selects whether masking runs on input, output, or both.
	// When zero, NewMasker defaults to MaskOnInput | MaskOnOutput so that
	// a masker constructed with only Patterns set is immediately active
	// on both sides.
	ApplyOn MaskOn
}

// Masker applies PII redaction to text according to a MaskConfig. A Masker
// also implements the session.MessageMasker interface (MaskInput /
// MaskOutput) so it can be wired into the session and agent as a hook.
type Masker struct {
	cfg      MaskConfig
	patterns map[PIIType]*regexp.Regexp
}

// NewMasker constructs a Masker from cfg. If cfg.Patterns is empty,
// DefaultPatterns() is used. If cfg.MaskChar is empty, it defaults to
// "***". If cfg.ApplyOn is zero, it defaults to MaskOnInput | MaskOnOutput
// so the masker is active on both sides without extra configuration.
func NewMasker(cfg MaskConfig) *Masker {
	patterns := cfg.Patterns
	if len(patterns) == 0 {
		patterns = DefaultPatterns()
	}
	maskChar := cfg.MaskChar
	if maskChar == "" {
		maskChar = "***"
	}
	applyOn := cfg.ApplyOn
	if applyOn == 0 {
		applyOn = MaskOnInput | MaskOnOutput
	}
	return &Masker{
		cfg: MaskConfig{
			Patterns:   patterns,
			MaskChar:   maskChar,
			KeepPrefix: cfg.KeepPrefix,
			ApplyOn:    applyOn,
		},
		patterns: patterns,
	}
}

// Config returns the effective configuration used by the masker (with
// defaults applied). The returned value is a copy; mutating it does not
// affect the masker.
func (m *Masker) Config() MaskConfig {
	return m.cfg
}

// Mask redacts every configured PII type from text and returns the
// scrubbed string. Patterns are applied in a deterministic order (most
// specific first) so that overlapping patterns such as BankAccount do not
// swallow matches that belong to IDCard or CreditCard.
func (m *Masker) Mask(text string) string {
	for _, t := range orderedPIITypes(m.patterns) {
		re := m.patterns[t]
		if re == nil {
			continue
		}
		text = re.ReplaceAllStringFunc(text, func(match string) string {
			return maskMatch(match, m.cfg.KeepPrefix, m.cfg.MaskChar)
		})
	}
	return text
}

// MaskInput masks text for the input side of the conversation. It is a
// no-op (returns text unchanged) when the masker's ApplyOn does not
// include MaskOnInput. This method satisfies session.MessageMasker.
func (m *Masker) MaskInput(text string) string {
	if m.cfg.ApplyOn&MaskOnInput == 0 {
		return text
	}
	return m.Mask(text)
}

// MaskOutput masks text for the output side of the conversation. It is a
// no-op (returns text unchanged) when the masker's ApplyOn does not
// include MaskOnOutput. This method satisfies session.MessageMasker.
func (m *Masker) MaskOutput(text string) string {
	if m.cfg.ApplyOn&MaskOnOutput == 0 {
		return text
	}
	return m.Mask(text)
}

// maskMatch returns the redacted form of a single PII match: the first
// keepPrefix characters are preserved and the rest is replaced with
// maskChar. When keepPrefix is zero the whole match is replaced; when
// keepPrefix is greater than or equal to the match length the match is
// returned unchanged.
func maskMatch(match string, keepPrefix int, maskChar string) string {
	if keepPrefix <= 0 {
		return maskChar
	}
	if keepPrefix >= len(match) {
		return match
	}
	return match[:keepPrefix] + maskChar
}
