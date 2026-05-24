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

import "testing"

// numericEquals reports whether got is a numeric value (int or float64)
// equal to want. Used so ExtractByPath tests accept both Go-literal ints
// and JSON-unmarshaled float64s.
func numericEquals(got any, want int) bool {
	switch n := got.(type) {
	case int:
		return n == want
	case int64:
		return int(n) == want
	case float64:
		return int(n) == want
	}
	return false
}

// TestExtractByPath covers the dot/slash + array-index path resolver.
// These are the seven contractual scenarios from the spec:
//   1. nil data           → (nil, false)
//   2. path not found     → (nil, false)
//   3. dot path (a.b.c)   → nested object access
//   4. slash path (a/b)   → equivalent to dot
//   5. array index        → choices[0].delta.content
//   6. mixed dot+index    → covered by #5
//   7. DefaultOpenAIContentMapping has keys "reasoning" and "delta"
func TestExtractByPath(t *testing.T) {
	t.Run("nil_data_returns_false", func(t *testing.T) {
		got, ok := ExtractByPath(nil, "a.b.c")
		if ok {
			t.Errorf("ExtractByPath(nil, ...) ok = true, want false")
		}
		if got != nil {
			t.Errorf("ExtractByPath(nil, ...) = %v, want nil", got)
		}
	})

	t.Run("path_not_found_returns_false", func(t *testing.T) {
		data := map[string]any{"a": 1}
		got, ok := ExtractByPath(data, "x.y")
		if ok {
			t.Errorf("ok = true, want false for missing path")
		}
		if got != nil {
			t.Errorf("got = %v, want nil for missing path", got)
		}
	})

	t.Run("dot_path_nested_objects", func(t *testing.T) {
		data := map[string]any{"a": map[string]any{"b": map[string]any{"c": 42}}}
		got, ok := ExtractByPath(data, "a.b.c")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		// Numbers may appear as int (Go literals) or float64 (JSON unmarshal);
		// accept both and compare by numeric value.
		if !numericEquals(got, 42) {
			t.Errorf("got = %v (%T), want 42", got, got)
		}
	})

	t.Run("slash_path_equivalent_to_dot", func(t *testing.T) {
		data := map[string]any{"a": map[string]any{"b": 1}}
		got, ok := ExtractByPath(data, "a/b")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if !numericEquals(got, 1) {
			t.Errorf("got = %v, want 1", got)
		}
	})

	t.Run("array_index_access", func(t *testing.T) {
		data := map[string]any{
			"choices": []any{
				map[string]any{"delta": map[string]any{"content": "hi"}},
			},
		}
		got, ok := ExtractByPath(data, "choices[0].delta.content")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if s, ok := got.(string); !ok || s != "hi" {
			t.Errorf("got = %v, want \"hi\"", got)
		}
	})

	t.Run("array_index_only", func(t *testing.T) {
		data := map[string]any{"choices": []any{"first", "second"}}
		got, ok := ExtractByPath(data, "choices[1]")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if s, ok := got.(string); !ok || s != "second" {
			t.Errorf("got = %v, want \"second\"", got)
		}
	})

	t.Run("out_of_range_index_returns_false", func(t *testing.T) {
		data := map[string]any{"choices": []any{"only"}}
		_, ok := ExtractByPath(data, "choices[5]")
		if ok {
			t.Error("ok = true for out-of-range index, want false")
		}
	})

	t.Run("index_into_non_array_returns_false", func(t *testing.T) {
		data := map[string]any{"choices": "not-an-array"}
		_, ok := ExtractByPath(data, "choices[0]")
		if ok {
			t.Error("ok = true when indexing non-array, want false")
		}
	})

	t.Run("empty_path_returns_false", func(t *testing.T) {
		data := map[string]any{"a": 1}
		_, ok := ExtractByPath(data, "")
		if ok {
			t.Error("ok = true for empty path, want false")
		}
	})
}

// TestDefaultOpenAIContentMapping verifies the default mapping covers the two
// semantic keys OpenAI Provider needs: reasoning and delta.
func TestDefaultOpenAIContentMapping(t *testing.T) {
	if _, ok := DefaultOpenAIContentMapping["reasoning"]; !ok {
		t.Errorf("DefaultOpenAIContentMapping missing 'reasoning' key: %v", DefaultOpenAIContentMapping)
	}
	if _, ok := DefaultOpenAIContentMapping["delta"]; !ok {
		t.Errorf("DefaultOpenAIContentMapping missing 'delta' key: %v", DefaultOpenAIContentMapping)
	}
	// Sanity: the default reasoning path should match the documented OpenAI
	// reasoning_content field location.
	if got := DefaultOpenAIContentMapping["reasoning"]; got != "choices[0].delta.reasoning_content" {
		t.Errorf("DefaultOpenAIContentMapping[reasoning] = %q, want %q", got, "choices[0].delta.reasoning_content")
	}
	if got := DefaultOpenAIContentMapping["delta"]; got != "choices[0].delta.content" {
		t.Errorf("DefaultOpenAIContentMapping[delta] = %q, want %q", got, "choices[0].delta.content")
	}
}
