package model

import (
	"bytes"
	"encoding/json"
	"sort"
)

// MarshalStable serializes v to JSON with deterministic byte ordering:
//   - Object keys sorted alphabetically (recursively)
//   - Map keys sorted alphabetically
//   - No whitespace differences between calls with same input
//
// This is critical for prefix cache hits: if the same tool definition
// serializes to different bytes across calls (due to Go map iteration
// randomness), the prefix cache cannot hit.
func MarshalStable(v any) ([]byte, error) {
	// First marshal normally
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Re-parse and re-encode with sorted keys
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := encodeStable(&buf, decoded); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeStable writes v to buf with stable ordering.
func encodeStable(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(string(val))
	case string:
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(b)
	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeStable(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := encodeStable(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		// Fallback to standard json encoding
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

// MustMarshalStable is like MarshalStable but panics on error.
// Use only for values known to be marshalable (e.g., static tool definitions).
func MustMarshalStable(v any) []byte {
	b, err := MarshalStable(v)
	if err != nil {
		panic(err)
	}
	return b
}

// StableString returns the stable string representation of v.
// Two calls with semantically-equal inputs return identical strings.
func StableString(v any) string {
	b, err := MarshalStable(v)
	if err != nil {
		return ""
	}
	return string(b)
}
