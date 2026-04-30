package actionruntime

import (
	"encoding/json"
	"testing"
)

func TestActionCall_JSONSerialization(t *testing.T) {
	// Test that ActionCall can be serialized and deserialized correctly
	original := ActionCall{
		Name:   "calc",
		Params: map[string]any{"a": 1, "b": 2, "op": "add"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ActionCall: %v", err)
	}

	var decoded ActionCall
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal ActionCall: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Params["op"] != "add" {
		t.Errorf("Params mismatch: got %v", decoded.Params)
	}
	if a, ok := decoded.Params["a"].(float64); !ok || int(a) != 1 {
		t.Errorf("Params[a] mismatch: got %v", decoded.Params["a"])
	}
	if b, ok := decoded.Params["b"].(float64); !ok || int(b) != 2 {
		t.Errorf("Params[b] mismatch: got %v", decoded.Params["b"])
	}
}

func TestActionCall_JSONRoundTrip(t *testing.T) {
	// Test full round-trip JSON serialization
	jsonStr := `{"name":"weather_query","params":{"city":"Beijing"}}`

	var decoded ActionCall
	err := json.Unmarshal([]byte(jsonStr), &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != "weather_query" {
		t.Errorf("Name mismatch: got %q, want %q", decoded.Name, "weather_query")
	}
	if decoded.Params["city"] != "Beijing" {
		t.Errorf("Params mismatch: got %v", decoded.Params)
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var reDecoded ActionCall
	err = json.Unmarshal(encoded, &reDecoded)
	if err != nil {
		t.Fatalf("Failed to re-unmarshal: %v", err)
	}

	if reDecoded.Name != decoded.Name {
		t.Errorf("Round-trip Name mismatch: got %q, want %q", reDecoded.Name, decoded.Name)
	}
}

func TestDecision_ExecuteDecision(t *testing.T) {
	// Test parsing an execute decision
	decision := Decision{
		NextAction: "execute",
		ActionCalls: []ActionCall{
			{Name: "calc", Params: map[string]any{"a": 1, "b": 2}},
			{Name: "format", Params: map[string]any{"input": 3}},
		},
	}

	if decision.NextAction != "execute" {
		t.Errorf("NextAction mismatch: got %q, want %q", decision.NextAction, "execute")
	}
	if len(decision.ActionCalls) != 2 {
		t.Fatalf("Expected 2 ActionCalls, got %d", len(decision.ActionCalls))
	}
	if decision.ActionCalls[0].Name != "calc" {
		t.Errorf("First action name: got %q, want %q", decision.ActionCalls[0].Name, "calc")
	}
	if decision.FinalResponse != "" {
		t.Errorf("FinalResponse should be empty for execute decision, got %q", decision.FinalResponse)
	}
}

func TestDecision_ResponseDecision(t *testing.T) {
	// Test parsing a response decision
	decision := Decision{
		NextAction:    "response",
		FinalResponse: "结果是 3",
	}

	if decision.NextAction != "response" {
		t.Errorf("NextAction mismatch: got %q, want %q", decision.NextAction, "response")
	}
	if decision.FinalResponse != "结果是 3" {
		t.Errorf("FinalResponse mismatch: got %q, want %q", decision.FinalResponse, "结果是 3")
	}
	if len(decision.ActionCalls) != 0 {
		t.Errorf("ActionCalls should be empty for response decision, got %v", decision.ActionCalls)
	}
}

func TestDecision_JSONRoundTrip(t *testing.T) {
	// Test JSON serialization/deserialization of Decision
	original := Decision{
		NextAction: "execute",
		ActionCalls: []ActionCall{
			{Name: "calc", Params: map[string]any{"x": 42}},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Decision
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.NextAction != original.NextAction {
		t.Errorf("NextAction mismatch: got %q, want %q", decoded.NextAction, original.NextAction)
	}
	if len(decoded.ActionCalls) != len(original.ActionCalls) {
		t.Errorf("ActionCalls length mismatch: got %d, want %d", len(decoded.ActionCalls), len(original.ActionCalls))
	}
}
