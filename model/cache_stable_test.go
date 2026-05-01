package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestMarshalStable_SameMapIdenticalBytes verifies that serializing the same
// map twice produces identical bytes (i.e., the encoder is deterministic).
func TestMarshalStable_SameMapIdenticalBytes(t *testing.T) {
	m := map[string]any{
		"name":       "calc",
		"description": "a calculator",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
			},
		},
	}
	first, err := MarshalStable(m)
	if err != nil {
		t.Fatalf("first MarshalStable error: %v", err)
	}
	for i := 0; i < 20; i++ {
		b, err := MarshalStable(m)
		if err != nil {
			t.Fatalf("iter %d MarshalStable error: %v", i, err)
		}
		if !bytes.Equal(first, b) {
			t.Fatalf("iter %d: bytes differ\nfirst: %s\n curr: %s", i, first, b)
		}
	}
}

// TestMarshalStable_MapKeyOrdering verifies that two semantically-equal maps
// produce identical bytes regardless of Go map iteration order, and that the
// output keys are alphabetically sorted.
func TestMarshalStable_MapKeyOrdering(t *testing.T) {
	m1 := map[string]any{"b": 1, "a": 2, "c": 3}
	m2 := map[string]any{"c": 3, "a": 2, "b": 1}

	b1, err := MarshalStable(m1)
	if err != nil {
		t.Fatalf("m1 error: %v", err)
	}
	b2, err := MarshalStable(m2)
	if err != nil {
		t.Fatalf("m2 error: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("expected identical bytes\nm1: %s\nm2: %s", b1, b2)
	}

	// Verify alphabetical key ordering in output
	s := string(b1)
	aIdx := strings.Index(s, `"a"`)
	bIdx := strings.Index(s, `"b"`)
	cIdx := strings.Index(s, `"c"`)
	if !(aIdx < bIdx && bIdx < cIdx) {
		t.Fatalf("keys not alphabetically sorted in output %s (a=%d b=%d c=%d)", s, aIdx, bIdx, cIdx)
	}
}

// TestMarshalStable_NestedMapSorting verifies recursive sorting of nested maps.
func TestMarshalStable_NestedMapSorting(t *testing.T) {
	m := map[string]any{
		"outer_z": map[string]any{
			"inner_y": 1,
			"inner_x": 2,
		},
		"outer_a": map[string]any{
			"inner_b": 3,
			"inner_a": 4,
		},
	}
	b, err := MarshalStable(m)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	s := string(b)
	// outer_a should come before outer_z
	if strings.Index(s, `"outer_a"`) > strings.Index(s, `"outer_z"`) {
		t.Fatalf("outer keys not sorted: %s", s)
	}
	// inner_a should come before inner_b
	aInner := strings.Index(s, `"inner_a"`)
	bInner := strings.Index(s, `"inner_b"`)
	if aInner > bInner {
		t.Fatalf("inner keys of outer_a not sorted: %s", s)
	}
	// inner_x should come before inner_y
	xInner := strings.Index(s, `"inner_x"`)
	yInner := strings.Index(s, `"inner_y"`)
	if xInner > yInner {
		t.Fatalf("inner keys of outer_z not sorted: %s", s)
	}
}

// TestMarshalStable_SliceOrderPreserved verifies that slice ordering is
// preserved (NOT sorted) — only map keys are sorted.
func TestMarshalStable_SliceOrderPreserved(t *testing.T) {
	s := []any{"c", "a", "b"}
	b, err := MarshalStable(s)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := `["c","a","b"]`
	if string(b) != want {
		t.Fatalf("got %s, want %s", b, want)
	}
}

// TestMarshalStable_ToolDefinition verifies that a ToolDefinition serializes
// stably across calls (the parameters map is the main concern).
func TestMarshalStable_ToolDefinition(t *testing.T) {
	td := ToolDefinition{
		Name:        "calc",
		Description: "a calculator",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
				"precision":  map[string]any{"type": "integer"},
			},
			"required": []any{"expression"},
		},
	}
	first, err := MarshalStable(td)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	for i := 0; i < 20; i++ {
		b, err := MarshalStable(td)
		if err != nil {
			t.Fatalf("iter %d error: %v", i, err)
		}
		if !bytes.Equal(first, b) {
			t.Fatalf("iter %d: bytes differ\nfirst: %s\n curr: %s", i, first, b)
		}
	}
	// Verify alphabetically sorted keys at top level: description, name, parameters
	s := string(first)
	descIdx := strings.Index(s, `"description"`)
	nameIdx := strings.Index(s, `"name"`)
	paramIdx := strings.Index(s, `"parameters"`)
	if !(descIdx < nameIdx && nameIdx < paramIdx) {
		t.Fatalf("top-level keys not sorted: %s", s)
	}
}

// TestMarshalStable_ToolDefinitionSlice verifies that a slice of ToolDefinitions
// serializes stably across calls — this is the exact use case for buildToolDefinitions.
func TestMarshalStable_ToolDefinitionSlice(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "search",
			Description: "search the web",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "calc",
			Description: "calculator",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expr": map[string]any{"type": "string"},
				},
			},
		},
	}
	first, err := MarshalStable(tools)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	for i := 0; i < 20; i++ {
		b, err := MarshalStable(tools)
		if err != nil {
			t.Fatalf("iter %d error: %v", i, err)
		}
		if !bytes.Equal(first, b) {
			t.Fatalf("iter %d: bytes differ\nfirst: %s\n curr: %s", i, first, b)
		}
	}
}

// TestStableString_Consistency verifies StableString returns the same string
// across 100 calls.
func TestStableString_Consistency(t *testing.T) {
	v := map[string]any{
		"z": []any{1, 2, 3},
		"a": map[string]any{"nested": true},
		"m": "hello",
	}
	first := StableString(v)
	if first == "" {
		t.Fatal("StableString returned empty for valid input")
	}
	for i := 0; i < 100; i++ {
		s := StableString(v)
		if s != first {
			t.Fatalf("iter %d: got %q, want %q", i, s, first)
		}
	}
}

// TestMarshalStable_Primitives verifies primitive types serialize correctly.
func TestMarshalStable_Primitives(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, "null"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"string", "hello", `"hello"`},
		{"empty_string", "", `""`},
		{"int", 42, "42"},
		{"negative_int", -7, "-7"},
		{"float", 3.14, "3.14"},
		{"empty_array", []any{}, "[]"},
		{"empty_map", map[string]any{}, "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := MarshalStable(tc.v)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// TestMustMarshalStable_PanicsOnError verifies that MustMarshalStable panics
// when given an unmarshalable value.
func TestMustMarshalStable_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	// A channel cannot be JSON-marshaled.
	MustMarshalStable(make(chan int))
}

// TestMarshalStable_JsonNumberPreserved verifies that large integer values
// are preserved as integers (not converted to float64) thanks to UseNumber.
func TestMarshalStable_JsonNumberPreserved(t *testing.T) {
	v := map[string]any{
		"big":   json.Number("9223372036854775807"), // max int64
		"small": 1,
	}
	b, err := MarshalStable(v)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"big":9223372036854775807`) {
		t.Fatalf("expected big int preserved, got %s", s)
	}
	if !strings.Contains(s, `"small":1`) {
		t.Fatalf("expected small int preserved, got %s", s)
	}
}

// TestMarshalStable_SpecialStringChars verifies strings with special
// characters are properly escaped.
func TestMarshalStable_SpecialStringChars(t *testing.T) {
	v := map[string]any{"key": "value with \"quotes\" and \n newline"}
	b, err := MarshalStable(v)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Round-trip via standard json to verify correctness
	var decoded map[string]string
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if decoded["key"] != `value with "quotes" and `+"\n newline" {
		t.Fatalf("round-trip mismatch: %q", decoded["key"])
	}
}
