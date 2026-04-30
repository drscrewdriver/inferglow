package actionruntime

// ShouldContinue determines whether the orchestrator should continue the
// PLAN → EXECUTE loop based on the latest decision and round index.
func ShouldContinue(decision Decision, roundIndex, maxRounds int) bool {
	if maxRounds > 0 && roundIndex >= maxRounds {
		return false
	}
	if decision.NextAction != "execute" {
		return false
	}
	return len(decision.ActionCalls) > 0
}
