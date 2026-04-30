package actionruntime

import (
	"encoding/json"
	"fmt"
)

// DecisionJSON is an intermediate struct for parsing LLM structured output.
type decisionJSON struct {
	NextAction    string       `json:"next_action"`
	ActionCalls   []ActionCall `json:"action_calls,omitempty"`
	FinalResponse string       `json:"final_response,omitempty"`
}

// ParseDecision parses a JSON string into a Decision.
//
// It expects a JSON object with:
//   - next_action: "execute" or "response"
//   - action_calls: (optional) array of {name, params} for execute decisions
//   - final_response: (optional) the response text for response decisions
func ParseDecision(content string) (*Decision, error) {
	var j decisionJSON
	if err := json.Unmarshal([]byte(content), &j); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if j.NextAction == "" {
		return nil, fmt.Errorf("missing next_action field")
	}
	if j.NextAction != "execute" && j.NextAction != "response" {
		return nil, fmt.Errorf("invalid next_action: %q", j.NextAction)
	}
	decision := Decision{
		NextAction:    j.NextAction,
		ActionCalls:   j.ActionCalls,
		FinalResponse: j.FinalResponse,
	}
	return &decision, nil
}
