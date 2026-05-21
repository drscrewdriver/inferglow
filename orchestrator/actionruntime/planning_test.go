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
