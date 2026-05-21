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

package actionruntime

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DecisionJSON is an intermediate struct for parsing LLM structured output.
type decisionJSON struct {
	NextAction    string       `json:"next_action"`
	ActionCalls   []ActionCall `json:"action_calls,omitempty"`
	FinalResponse string       `json:"final_response,omitempty"`
}

// RepairLLMJSON cleans up common LLM JSON mistakes so that json.Unmarshal can
// succeed on otherwise-invalid output. It applies the following repair
// pipeline in order:
//
//  1. Markdown code-fence stripping: ```json ... ``` (or ``` ... ```) → inner
//     content. Only the first fenced block is extracted; if the input does
//     not contain a fence, this step is a no-op.
//  2. Noise extraction: locate the first '{' and its matching '}' (taking
//     string literals and nested objects into account) and return only the
//     substring between them. This handles cases where the LLM prepends
//     prose like "Sure! Here is the decision:" and/or appends trailing
//     remarks.
//  3. Trailing-comma removal: replace ",}" with "}" and ",]" with "]" so
//     that JSON emitted with a dangling comma before a closing brace still
//     parses. Repeated until no more replacements are made.
//
// For input that is already valid JSON the function is a no-op. For empty
// or non-JSON input it returns the input unchanged so that json.Unmarshal
// can return its usual error.
func RepairLLMJSON(input string) string {
	s := input

	// 1. Strip markdown code fences. We look for the first occurrence of
	// "```" and extract up to the next "```". The opening fence may be
	// followed by a language tag like "json" or "JSON" on the same line.
	if openIdx := strings.Index(s, "```"); openIdx != -1 {
		rest := s[openIdx+3:]
		// Skip an optional language tag up to and including the next newline.
		// If the fence is immediately followed by a newline (no tag), keep
		// the content from that newline.
		if nl := strings.IndexByte(rest, '\n'); nl != -1 {
			// Only treat the segment before the newline as a language tag
			// if it is short and contains no closing fence marker.
			tag := rest[:nl]
			trimmed := strings.TrimSpace(tag)
			// Heuristic: a language tag is alphanumeric (json, JSON, etc.).
			// If the trimmed tag is empty or looks like a tag, advance past
			// the newline. Otherwise leave rest unchanged (the fence may
			// have no tag and the content starts immediately).
			if trimmed == "" || isLikelyCodeFenceTag(trimmed) {
				rest = rest[nl+1:]
			}
		}
		if closeIdx := strings.Index(rest, "```"); closeIdx != -1 {
			s = rest[:closeIdx]
		} else {
			// No closing fence — take the rest of the input.
			s = rest
		}
	}

	// 2. Extract the first balanced {...} object, ignoring braces inside
	// string literals. This strips leading/trailing prose noise.
	if obj, ok := extractFirstJSONObject(s); ok {
		s = obj
	}

	// 3. Remove trailing commas before closing braces/brackets. Repeat so
	// that patterns like ",,}" (which shouldn't occur but might) also
	// resolve. We use a small loop with a cap to avoid pathological input.
	for i := 0; i < 16; i++ {
		next := removeTrailingCommas(s)
		if next == s {
			break
		}
		s = next
	}

	return s
}

// isLikelyCodeFenceTag reports whether s looks like a markdown code-fence
// language tag (e.g. "json", "JSON", "go", "python"). A tag is recognized
// when it is short (≤16 runes) and consists only of ASCII letters/digits.
func isLikelyCodeFenceTag(s string) bool {
	if len(s) == 0 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '+' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// extractFirstJSONObject scans s for the first '{' and returns the
// substring from that '{' through its matching '}', accounting for string
// literals and escape sequences. Returns ok=false if no balanced object
// was found.
func extractFirstJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start == -1 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	// No matching close brace. Return the rest from start so callers can
	// still attempt to parse (json.Unmarshal will return its own error).
	return s[start:], true
}

// removeTrailingCommas replaces ",}" with "}" and ",]" with "]" in s,
// also handling whitespace between the comma and the closing delimiter
// (e.g. ", }" or ",\n}"). It does not modify commas inside string
// literals.
func removeTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			b.WriteByte(c)
			continue
		}
		if c == ',' {
			// Look ahead: skip whitespace, then expect '}' or ']'.
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				// Skip the comma; emit the whitespace and the closing
				// delimiter by advancing i accordingly.
				b.WriteString(s[i+1 : j+1])
				i = j
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ParseDecision parses a (possibly messy) LLM output string into a Decision.
//
// It expects a JSON object (after cleaning) with:
//   - next_action: "execute" or "response"
//   - action_calls: (optional) array of {name, params} for execute decisions
//   - final_response: (optional) the response text for response decisions
//
// The input is first run through RepairLLMJSON to handle markdown code
// fences, surrounding prose noise, and trailing commas — all of which are
// common LLM mistakes that would otherwise cause json.Unmarshal to fail.
func ParseDecision(content string) (*Decision, error) {
	repaired := RepairLLMJSON(content)
	var j decisionJSON
	if err := json.Unmarshal([]byte(repaired), &j); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if j.NextAction == "" {
		return nil, fmt.Errorf("missing next_action field")
	}
	if j.NextAction != "execute" && j.NextAction != "response" {
		return nil, fmt.Errorf("invalid next_action: %q", j.NextAction)
	}
	decision := Decision{
		NextAction:    j.NextAction,
		ActionCalls:   j.ActionCalls,
		FinalResponse: j.FinalResponse,
	}
	return &decision, nil
}
