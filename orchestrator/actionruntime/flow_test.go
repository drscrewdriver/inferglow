package actionruntime

import "testing"

func TestShouldContinue_Execute(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if !ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=true for execute decision")
	}
}

func TestShouldContinue_Response(t *testing.T) {
	decision := Decision{NextAction: "response", FinalResponse: "done"}
	if ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=false for response decision")
	}
}

func TestShouldContinue_MaxRounds(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if ShouldContinue(decision, 5, 5) {
		t.Error("Expected shouldContinue=false at max rounds")
	}
}

func TestShouldContinue_MaxRoundsExceeded(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: []ActionCall{{Name: "test"}}}
	if ShouldContinue(decision, 6, 5) {
		t.Error("Expected shouldContinue=false when exceeding max rounds")
	}
}

func TestShouldContinue_EmptyActionCalls(t *testing.T) {
	decision := Decision{NextAction: "execute", ActionCalls: nil}
	if ShouldContinue(decision, 0, 5) {
		t.Error("Expected shouldContinue=false when ActionCalls is empty")
	}
}
