package schema

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// SC-MEDIUM-5: extractor 截断 JSON 修复增强
// ============================================================================

// TestExtractor_RepairTruncatedJSON 验证未闭合括号的截断 JSON 能被修复。
func TestExtractor_RepairTruncatedJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unclosed_array_in_object",
			input: `{"a":1,"b":[1,2,3`,
			want:  `{"a":1,"b":[1,2,3]}`,
		},
		{
			name:  "unclosed_object",
			input: `{"a":1,"b":2`,
			want:  `{"a":1,"b":2}`,
		},
		{
			name:  "unclosed_nested_object",
			input: `{"a":{"b":1`,
			want:  `{"a":{"b":1}}`,
		},
		{
			name:  "trailing_comma_in_object",
			input: `{"a":1,`,
			want:  `{"a":1}`,
		},
		{
			name:  "trailing_comma_in_array",
			input: `{"a":[1,2,3,`,
			want:  `{"a":[1,2,3]}`,
		},
		{
			name:  "unclosed_string_value",
			input: `{"a":"hello`,
			want:  `{"a":"hello"}`,
		},
		{
			name:  "unclosed_string_in_array",
			input: `{"a":["foo","bar`,
			want:  `{"a":["foo","bar"]}`,
		},
		{
			name:  "complete_json_unchanged",
			input: `{"a":1,"b":[1,2,3]}`,
			want:  `{"a":1,"b":[1,2,3]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RepairTruncatedJSON(tc.input)

			// 验证修复后能被解析为合法 JSON
			var parsed map[string]any
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Errorf("RepairTruncatedJSON(%q) = %q, parse error: %v", tc.input, got, err)
				return
			}

			// 验证修复结果与期望一致
			if got != tc.want {
				t.Errorf("RepairTruncatedJSON(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestExtractor_RepairTruncatedJSONComplex 验证复杂截断 JSON 也能被修复。
func TestExtractor_RepairTruncatedJSONComplex(t *testing.T) {
	input := `{"users":[{"name":"Alice","age":30},{"name":"Bob"`
	got := RepairTruncatedJSON(input)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("RepairTruncatedJSON returned invalid JSON: %q, error: %v", got, err)
	}

	users, ok := parsed["users"].([]any)
	if !ok {
		t.Fatalf("expected users to be []any, got %T", parsed["users"])
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

// TestExtractJSONWithTruncated 验证 ExtractJSON 能从含截断 JSON 的文本中提取并修复。
func TestExtractJSONWithTruncated(t *testing.T) {
	// 模拟 LLM 输出被截断的场景
	text := `Here is the result:
{"name": "test", "values": [1, 2, 3`

	result, err := ExtractJSON(text)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("name = %v, want %q", result["name"], "test")
	}
	values, ok := result["values"].([]any)
	if !ok {
		t.Fatalf("expected values to be []any, got %T", result["values"])
	}
	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}
