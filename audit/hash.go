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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ComputeHash returns the hex-encoded SHA-256 digest of the canonical
// representation of the entry. The preimage is the concatenation of
//
//	PrevHash + Timestamp(RFC3339Nano, UTC) + Source + Action +
//	canonicalJSON(Input) + canonicalJSON(Output)
//
// The canonical JSON encoding guarantees stable byte output regardless
// of map iteration order, so two semantically equal entries always
// produce the same hash.
func ComputeHash(entry *AuditEntry) string {
	if entry == nil {
		return ""
	}
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Time{}
	}
	preimage := entry.PrevHash +
		ts.UTC().Format(time.RFC3339Nano) +
		entry.Source +
		entry.Action +
		canonicalJSON(entry.Input) +
		canonicalJSON(entry.Output)
	sum := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(sum[:])
}

// canonicalJSON serializes v into a deterministic JSON byte slice:
//   - map[string]X keys are sorted ascending
//   - nested maps and slices are recursively canonicalized
//   - nil values serialize as "null"
//
// For values that are not JSON-serializable, canonicalJSON falls back to
// fmt.Sprint(v) so the hash is still defined (and stable) rather than
// panicking. The export is intentionally minimal — it covers what audit
// entries are expected to contain (maps, slices, primitives, structs).
func canonicalJSON(v any) string {
	b, err := marshalCanonical(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return []byte("null"), nil
	case map[string]any:
		return marshalCanonicalMap(t)
	case map[string]string:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = val
		}
		return marshalCanonicalMap(m)
	case []any:
		return marshalCanonicalSlice(t)
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return marshalCanonicalSlice(out)
	case []int:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return marshalCanonicalSlice(out)
	case []int64:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return marshalCanonicalSlice(out)
	case []float64:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return marshalCanonicalSlice(out)
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}
		return marshalCanonicalSlice(out)
	default:
		// Primitives (string/int/bool/float) and structs: use encoding/json
		// which is already canonical for non-map types.
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		// If json.Marshal returned a map-like object that we now need to
		// re-canonicalize (e.g. a struct that marshals to an object),
		// decode-and-re-encode it.
		var probe any
		if err := json.Unmarshal(b, &probe); err == nil {
			if _, ok := probe.(map[string]any); ok {
				return marshalCanonicalMap(probe.(map[string]any))
			}
		}
		return b, nil
	}
}

func marshalCanonicalMap(m map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := []byte{'{'}
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf = append(buf, kb...)
		buf = append(buf, ':')
		vb, err := marshalCanonical(m[k])
		if err != nil {
			return nil, err
		}
		buf = append(buf, vb...)
	}
	buf = append(buf, '}')
	return buf, nil
}

func marshalCanonicalSlice(s []any) ([]byte, error) {
	buf := []byte{'['}
	for i, item := range s {
		if i > 0 {
			buf = append(buf, ',')
		}
		b, err := marshalCanonical(item)
		if err != nil {
			return nil, err
		}
		buf = append(buf, b...)
	}
	buf = append(buf, ']')
	return buf, nil
}
