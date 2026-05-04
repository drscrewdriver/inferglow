package actionruntime

import (
	"strings"
	"testing"
)

// TestRepairLLMJSON_CodeBlock verifies O-CRITICAL-1 subtask: stripping
// ```json ... ``` code fences. LLMs frequently wrap structured output in
// markdown code blocks even when asked not to. The repair function must
// extract the inner JSON content before json.Unmarshal sees it.
func TestRepairLLMJSON_CodeBlock(t *testing.T) {
	input := "```json\n" + `{"next_action":"response","final_response":"hi"}` + "\n```"
	got := RepairLLMJSON(input)
	want := `{"next_action":"response","final_response":"hi"}`
	if got != want {
		t.Errorf("RepairLLMJSON code-block strip:\n got = %q\nwant = %q", got, want)
	}
}

// TestRepairLLMJSON_CodeBlockPlain verifies a ``` (no language tag) fence is
// also stripped.
func TestRepairLLMJSON_CodeBlockPlain(t *testing.T) {
	input := "```\n" + `{"next_action":"response","final_response":"hi"}` + "\n```"
	got := RepairLLMJSON(input)
	want := `{"next_action":"response","final_response":"hi"}`
	if got != want {
		t.Errorf("RepairLLMJSON plain code-block strip:\n got = %q\nwant = %q", got, want)
	}
}

// TestRepairLLMJSON_Noise verifies O-CRITICAL-1 subtask: extracting the
// JSON object from surrounding prose noise. When the LLM emits "Sure!
// Here is the decision: {...}" the repair function must locate the first
// '{' and the matching '}' and return only the JSON substring.
func TestRepairLLMJSON_Noise(t *testing.T) {
	input := `Sure! Here is the decision you asked for:
{"next_action":"response","final_response":"42"}
Let me know if you need anything else.`
	got := RepairLLMJSON(input)
	want := `{"next_action":"response","final_response":"42"}`
	if got != want {
		t.Errorf("RepairLLMJSON noise extraction:\n got = %q\nwant = %q", got, want)
	}
}

// TestRepairLLMJSON_TrailingComma verifies O-CRITICAL-1 subtask: removing
// trailing commas before '}' or ']' which are a common LLM JSON mistake.
func TestRepairLLMJSON_TrailingComma(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trailing comma before closing brace",
			input: `{"next_action":"response","final_response":"hi",}`,
			want:  `{"next_action":"response","final_response":"hi"}`,
		},
		{
			name:  "trailing comma before closing bracket",
			input: `{"action_calls":[{"name":"a","params":{},}],}`,
			want:  `{"action_calls":[{"name":"a","params":{}}]}`,
		},
		{
			name:  "multiple trailing commas",
			input: `{"a":[1,2,3,],"b":{"c":1,},}`,
			want:  `{"a":[1,2,3],"b":{"c":1}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RepairLLMJSON(tc.input)
			if got != tc.want {
				t.Errorf("RepairLLMJSON trailing comma (%s):\n got = %q\nwant = %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestParseDecision_WithLLMGarbage verifies O-CRITICAL-1 subtask: ParseDecision
// must transparently handle LLM-emitted content that includes markdown fences,
// leading/trailing prose, and trailing commas. The RepairLLMJSON repair
// pipeline must run before json.Unmarshal so callers do not see those errors.
func TestParseDecision_WithLLMGarbage(t *testing.T) {
	input := "```json\n" +
		`Sure! Here is the decision:` + "\n" +
		`{"next_action":"response","final_response":"hello world",}` + "\n```"
	decision, err := ParseDecision(input)
	if err != nil {
		t.Fatalf("ParseDecision returned error for LLM garbage: %v", err)
	}
	if decision.NextAction != "response" {
		t.Errorf("NextAction: got %q, want %q", decision.NextAction, "response")
	}
	if decision.FinalResponse != "hello world" {
		t.Errorf("FinalResponse: got %q, want %q", decision.FinalResponse, "hello world")
	}
}

// TestParseDecision_ExecuteWithMarkdownAndNoise combines code-block + noise +
// trailing comma for an execute decision with action_calls.
func TestParseDecision_ExecuteWithMarkdownAndNoise(t *testing.T) {
	input := "I'll execute an action.\n```json\n" +
		`{"next_action":"execute","action_calls":[{"name":"calc","params":{"a":1,},},],}` + "\n```\nDone."
	decision, err := ParseDecision(input)
	if err != nil {
		t.Fatalf("ParseDecision returned error: %v", err)
	}
	if decision.NextAction != "execute" {
		t.Errorf("NextAction: got %q, want %q", decision.NextAction, "execute")
	}
	if len(decision.ActionCalls) != 1 {
		t.Fatalf("ActionCalls len: got %d, want 1", len(decision.ActionCalls))
	}
	if decision.ActionCalls[0].Name != "calc" {
		t.Errorf("ActionCalls[0].Name: got %q, want %q", decision.ActionCalls[0].Name, "calc")
	}
}

// TestRepairLLMJSON_PassthroughValidJSON verifies that valid JSON is returned
// unchanged so the repair pipeline is a no-op for already-clean input.
func TestRepairLLMJSON_PassthroughValidJSON(t *testing.T) {
	input := `{"next_action":"response","final_response":"hi"}`
	got := RepairLLMJSON(input)
	if got != input {
		t.Errorf("RepairLLMJSON should be a no-op for valid JSON:\n got = %q\nwant = %q", got, input)
	}
}

// TestRepairLLMJSON_EmptyAndWhitespace verifies edge cases don't panic and
// return something json.Unmarshal can reject cleanly.
func TestRepairLLMJSON_EmptyAndWhitespace(t *testing.T) {
	cases := []string{"", "   ", "\n\n", "no json here at all"}
	for _, tc := range cases {
		got := RepairLLMJSON(tc)
		// Should not panic; contents are best-effort.
		if tc == "" && got != "" {
			t.Errorf("RepairLLMJSON(empty) = %q, want empty", got)
		}
		// For "no json here", should still be valid (return input unchanged or empty)
		if tc == "no json here at all" && strings.Contains(got, "{") {
			t.Errorf("RepairLLMJSON should not invent braces: %q", got)
		}
	}
}
