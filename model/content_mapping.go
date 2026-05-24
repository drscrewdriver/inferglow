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

package model

import (
	"regexp"
	"strings"
)

// ContentMapping maps semantic keys (e.g. "reasoning", "delta") to SSE chunk
// field paths. When set on a Provider, it lets callers extract reasoning and
// content from non-standard vendor variants without forking the provider.
//
// Path syntax:
//   - dot or slash separator: "a.b.c" or "a/b/c" are equivalent
//   - array index: "choices[0].delta.content"
//
// Example: a vendor that returns reasoning under data.thinking instead of
// choices[0].delta.reasoning_content can be configured with:
//
//	ContentMapping{"reasoning": "data.thinking", "delta": "choices[0].delta.content"}
//
// Spec: model-parity Phase 3, P1 — content_mapping 字段路径自定义.
type ContentMapping map[string]string

// DefaultOpenAIContentMapping is the canonical OpenAI Chat Completions SSE
// field layout. It is provided as a reference; OpenAICompatibleProvider uses
// it only when ContentMapping is explicitly set to this value. When
// Provider.ContentMapping is nil, the provider falls back to its struct-based
// parsing (preserving the legacy behavior).
var DefaultOpenAIContentMapping = ContentMapping{
	"reasoning": "choices[0].delta.reasoning_content",
	"delta":     "choices[0].delta.content",
}

// arrayIndexPattern matches "[N]" suffixes used to index into JSON arrays.
// Examples: "choices[0]" → ("choices", 0); "[3]" → ("", 3).
var arrayIndexPattern = regexp.MustCompile(`^(.*?)\[(\d+)\]$`)

// ExtractByPath resolves a dot/slash + array-index path against arbitrary
// JSON-derived data (map[string]any / []any / scalars). Returns (value, true)
// on success or (nil, false) if any path segment is missing or has the wrong
// type.
//
// Examples:
//
//	ExtractByPath({"a":{"b":{"c":42}}}, "a.b.c")        → (42.0, true)
//	ExtractByPath({"a":{"b":1}}, "a/b")                 → (1.0, true)
//	ExtractByPath({"choices":[{"delta":{"content":"hi"}}]}, "choices[0].delta.content")
//	                                                    → ("hi", true)
//	ExtractByPath({"a":1}, "x.y")                       → (nil, false)
//	ExtractByPath(nil, "a")                             → (nil, false)
//
// Spec: model-parity Phase 3 — content_mapping 字段路径自定义.
func ExtractByPath(data any, path string) (any, bool) {
	if data == nil || path == "" {
		return nil, false
	}

	// Unify separators: slash → dot.
	normalized := strings.ReplaceAll(path, "/", ".")

	// Split on dot, then expand any "key[N]" segments into two hops
	// (key → array index N). This keeps the traversal loop simple.
	segments := strings.Split(normalized, ".")
	var current any = data
	for _, seg := range segments {
		if seg == "" {
			return nil, false
		}
		// Check whether this segment carries an array index.
		if m := arrayIndexPattern.FindStringSubmatch(seg); m != nil {
			key := m[1]
			idx := m[2]
			// First hop: map lookup (unless key is empty, e.g. "[3]" at root).
			if key != "" {
				mp, ok := current.(map[string]any)
				if !ok {
					return nil, false
				}
				v, ok := mp[key]
				if !ok {
					return nil, false
				}
				current = v
			}
			// Second hop: array index.
			arr, ok := current.([]any)
			if !ok {
				return nil, false
			}
			// Parse the index. Pattern guarantees digits; guard anyway.
			var n int
			for _, ch := range idx {
				n = n*10 + int(ch-'0')
			}
			if n < 0 || n >= len(arr) {
				return nil, false
			}
			current = arr[n]
			continue
		}

		// Plain map-key segment.
		mp, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := mp[seg]
		if !ok {
			return nil, false
		}
		current = v
	}
	return current, true
}
