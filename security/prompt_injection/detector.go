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

package promptinjection

import (
	"regexp"
	"strings"
)

// Severity ranks how dangerous a detected pattern is. Higher values are
// more dangerous. The overall DetectionResult severity is the maximum
// severity among all individual matches.
type Severity int

const (
	// SeverityLow indicates a soft signal (e.g. a single borderline
	// keyword that can appear in benign text).
	SeverityLow Severity = iota
	// SeverityMedium indicates a likely injection attempt that is not
	// yet conclusive (e.g. role-markup tags, custom patterns).
	SeverityMedium
	// SeverityHigh indicates a clear injection attempt (e.g. explicit
	// "ignore previous instructions" phrasing).
	SeverityHigh
)

// String returns a human-readable label for the severity.
func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	default:
		return "unknown"
	}
}

// Match describes a single pattern hit within the scanned text.
type Match struct {
	// Pattern is the keyword or regex source that fired.
	Pattern string `json:"pattern"`
	// MatchedText is the substring of the input that matched.
	MatchedText string `json:"matched_text"`
	// Position is the byte offset within the input where the match
	// starts.
	Position int `json:"position"`
	// Severity is the per-match severity contribution.
	Severity Severity `json:"severity"`
}

// DetectionResult is the outcome of scanning a single text input.
type DetectionResult struct {
	// Detected is true when at least one pattern matched.
	Detected bool `json:"detected"`
	// Matches lists every individual hit, in order of appearance.
	Matches []Match `json:"matches,omitempty"`
	// Severity is the maximum severity among Matches (SeverityLow when
	// no matches exist).
	Severity Severity `json:"severity"`
}

// keywordRule pairs a case-insensitive keyword with its severity.
type keywordRule struct {
	keyword string
	sev     Severity
}

// patternRule pairs a compiled regex with its severity and source label.
type patternRule struct {
	re    *regexp.Regexp
	sev   Severity
	label string
}

// Detector is the L1 rule-based prompt-injection detector. It combines
// a curated keyword list with regex patterns that flag non-natural-
// language instruction formats embedded in user prompts. The detector
// is safe for concurrent use: all state is immutable after
// construction.
type Detector struct {
	keywords []keywordRule
	patterns []patternRule
	config   *Config
}

// built-in keywords. These are the canonical injection phrases observed
// in real attacks. Matching is case-insensitive and substring-based.
var defaultKeywords = []keywordRule{
	{"ignore previous", SeverityHigh},
	{"ignore the above", SeverityHigh},
	{"you are now", SeverityHigh},
	{"system:", SeverityHigh},
	{"new instructions:", SeverityHigh},
	{"disregard", SeverityMedium},
	{"override", SeverityMedium},
}

// built-in regex patterns. These catch structured instruction formats
// that do not appear in natural user language, such as chatml tags,
// [INST] markers, and explicit role labels.
var defaultPatternDefs = []struct {
	expr  string
	sev   Severity
	label string
}{
	// ChatML / tokenized system markers (e.g. "<|im_start|>system").
	{`(?i)<\|im_start\|>`, SeverityHigh, "chatml_im_start"},
	{`(?i)<\|im_end\|>`, SeverityMedium, "chatml_im_end"},
	// Llama [INST] / [/INST] instruction delimiters.
	{`(?i)\[/?INST\]`, SeverityMedium, "llama_inst_tag"},
	// Explicit role labels injected mid-prompt.
	{`(?i)\[\s*system\s*\]`, SeverityHigh, "bracket_system"},
	{`(?i)\[\s*assistant\s*\]`, SeverityMedium, "bracket_assistant"},
	// Markdown fenced "system" blocks used to smuggle instructions.
	{"(?i)```system", SeverityHigh, "markdown_system_block"},
	// "role: system" style declarations.
	{`(?i)\brole\s*:\s*system\b`, SeverityHigh, "role_system_decl"},
	// "###" section headers commonly used to override system prompts.
	{`(?i)^###\s*(system|instructions?)\s*$`, SeverityMedium, "hash_system_header"},
}

// NewDetector builds a Detector with the built-in rules and no config
// (equivalent to NewDefaultConfig). Use NewDetectorWithConfig to supply
// a custom Config.
func NewDetector() *Detector {
	return NewDetectorWithConfig(NewDefaultConfig())
}

// NewDetectorWithConfig builds a Detector with the given Config. When
// cfg.CustomPatterns is non-empty those patterns are appended to the
// built-in rules with Medium severity and a "custom" label. A nil cfg
// is replaced with the default Strict config.
func NewDetectorWithConfig(cfg *Config) *Detector {
	if cfg == nil {
		cfg = NewDefaultConfig()
	}
	d := &Detector{
		keywords: defaultKeywords,
		config:   cfg,
	}
	// Compile built-in patterns. A malformed built-in expression is a
	// programming error and would be caught at startup; we skip
	// un-compilable entries defensively rather than panicking.
	for _, def := range defaultPatternDefs {
		re, err := regexp.Compile(def.expr)
		if err != nil {
			continue
		}
		d.patterns = append(d.patterns, patternRule{re: re, sev: def.sev, label: def.label})
	}
	// Append user-supplied custom patterns (Medium severity).
	for i, cp := range cfg.CustomPatterns {
		if cp == nil {
			continue
		}
		d.patterns = append(d.patterns, patternRule{
			re:    cp,
			sev:   SeverityMedium,
			label: "custom_" + itoa(i),
		})
	}
	return d
}

// Config returns the detector's effective configuration.
func (d *Detector) Config() *Config { return d.config }

// Detect scans text against all keyword and regex rules and returns a
// DetectionResult. When the configured Level is Off, detection is
// skipped and an empty (non-detected) result is returned immediately.
func (d *Detector) Detect(text string) *DetectionResult {
	result := &DetectionResult{Detected: false, Severity: SeverityLow}
	if d == nil {
		return result
	}
	if d.config != nil && d.config.Level == LevelOff {
		return result
	}
	if text == "" {
		return result
	}
	lowered := strings.ToLower(text)
	for _, kw := range d.keywords {
		idx := strings.Index(lowered, kw.keyword)
		for idx >= 0 {
			result.Matches = append(result.Matches, Match{
				Pattern:     kw.keyword,
				MatchedText: text[idx : idx+len(kw.keyword)],
				Position:    idx,
				Severity:    kw.sev,
			})
			if kw.sev > result.Severity {
				result.Severity = kw.sev
			}
			next := idx + len(kw.keyword)
			if next >= len(lowered) {
				break
			}
			idx = strings.Index(lowered[next:], kw.keyword)
			if idx < 0 {
				break
			}
			idx += next
		}
	}
	for _, pr := range d.patterns {
		loc := pr.re.FindStringIndex(text)
		for loc != nil {
			result.Matches = append(result.Matches, Match{
				Pattern:     pr.label,
				MatchedText: text[loc[0]:loc[1]],
				Position:    loc[0],
				Severity:    pr.sev,
			})
			if pr.sev > result.Severity {
				result.Severity = pr.sev
			}
			next := loc[1]
			if next >= len(text) {
				break
			}
			loc = pr.re.FindStringIndex(text[next:])
			if loc == nil {
				break
			}
			loc[0] += next
			loc[1] += next
		}
	}
	if len(result.Matches) > 0 {
		result.Detected = true
	}
	return result
}

// itoa is a small stdlib-free int → string helper used for custom
// pattern labels so detector.go does not import strconv solely for one
// label-formatting call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
