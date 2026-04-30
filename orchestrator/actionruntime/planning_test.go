package actionruntime

import (
	"testing"
)

func TestParseDecision_Execute(t *testing.T) {
	jsonStr := `{
		"next_action": "execute",
		"action_calls": [
			{"name": "calc", "params": {"a": 1, "b": 2}},
			{"name": "format", "params": {"input": "result"}}
		]
	}`

	decision, err := ParseDecision(jsonStr)
	if err != nil {
		t.Fatalf("ParseDecision returned error: %v", err)
	}

	if decision.NextAction != "execute" {
		t.Errorf("NextAction: got %q, want %q", decision.NextAction, "execute")
	}
	if len(decision.ActionCalls) != 2 {
		t.Fatalf("ActionCalls length: got %d, want 2", len(decision.ActionCalls))
	}
	if decision.ActionCalls[0].Name != "calc" {
		t.Errorf("First call name: got %q, want %q", decision.ActionCalls[0].Name, "calc")
	}
	if decision.FinalResponse != "" {
		t.Errorf("FinalResponse should be empty, got %q", decision.FinalResponse)
	}
}

func TestParseDecision_Response(t *testing.T) {
	jsonStr := `{
		"next_action": "response",
		"final_response": "结果是 42"
	}`

	decision, err := ParseDecision(jsonStr)
	if err != nil {
		t.Fatalf("ParseDecision returned error: %v", err)
	}

	if decision.NextAction != "response" {
		t.Errorf("NextAction: got %q, want %q", decision.NextAction, "response")
	}
	if decision.FinalResponse != "结果是 42" {
		t.Errorf("FinalResponse: got %q, want %q", decision.FinalResponse, "结果是 42")
	}
	if len(decision.ActionCalls) != 0 {
		t.Errorf("ActionCalls should be empty, got %v", decision.ActionCalls)
	}
}

func TestParseDecision_InvalidJSON(t *testing.T) {
	_, err := ParseDecision("not valid json")
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParseDecision_MissingNextAction(t *testing.T) {
	jsonStr := `{"action_calls": []}`
	_, err := ParseDecision(jsonStr)
	if err == nil {
		t.Fatal("Expected error for missing next_action, got nil")
	}
}
