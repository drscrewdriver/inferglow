package policies

import (
	"sort"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
)

func allActionIDs() []string {
	return []string{
		actions.CalculatorActionID,
		actions.WebSearchActionID,
		actions.URLFetchActionID,
		actions.FileReadActionID,
		actions.FileWriteActionID,
		actions.JSONProcessorActionID,
		actions.CodeExecutorActionID,
		actions.BashExecutorActionID,
	}
}

func sideEffectLevel(name string) action.SideEffectLevel {
	switch name {
	case actions.CalculatorActionID:
		return actions.CalculatorSpec.SideEffectLevel
	case actions.WebSearchActionID:
		return actions.WebSearchSpec.SideEffectLevel
	case actions.URLFetchActionID:
		return actions.URLFetchSpec.SideEffectLevel
	case actions.FileReadActionID:
		return actions.FileReadSpec.SideEffectLevel
	case actions.FileWriteActionID:
		return actions.FileWriteSpec.SideEffectLevel
	case actions.JSONProcessorActionID:
		return actions.JSONProcessorSpec.SideEffectLevel
	case actions.CodeExecutorActionID:
		return actions.CodeExecutorSpec.SideEffectLevel
	case actions.BashExecutorActionID:
		return actions.BashExecutorSpec.SideEffectLevel
	}
	return ""
}

func levelSet(levels ...action.SideEffectLevel) map[action.SideEffectLevel]bool {
	m := make(map[action.SideEffectLevel]bool, len(levels))
	for _, l := range levels {
		m[l] = true
	}
	return m
}

func assertRegistered(t *testing.T, r *action.ActionRegistry, want []string) {
	t.Helper()
	got := r.List()
	sort.Strings(got)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	if len(got) != len(sortedWant) {
		t.Fatalf("expected %d registered actions %v, got %d %v", len(sortedWant), sortedWant, len(got), got)
	}
	for i, name := range sortedWant {
		if got[i] != name {
			t.Fatalf("expected action %q at index %d, got %q", name, i, got[i])
		}
	}
	for _, name := range want {
		if !r.Has(name) {
			t.Errorf("expected action %q to be registered (Has=false)", name)
		}
	}
}

func assertNotRegistered(t *testing.T, r *action.ActionRegistry, names []string) {
	t.Helper()
	for _, name := range names {
		if r.Has(name) {
			t.Errorf("action %q should not be registered (Has=true)", name)
		}
	}
}

func assertLevelsAllowed(t *testing.T, r *action.ActionRegistry, allowed map[action.SideEffectLevel]bool) {
	t.Helper()
	for _, name := range r.List() {
		level := sideEffectLevel(name)
		if !allowed[level] {
			t.Errorf("action %q has side effect %q which is not allowed by this policy", name, level)
		}
	}
}

func TestRestrictivePolicy(t *testing.T) {
	r := RestrictivePolicy()
	want := []string{
		actions.CalculatorActionID,
		actions.WebSearchActionID,
		actions.URLFetchActionID,
		actions.FileReadActionID,
		actions.JSONProcessorActionID,
	}
	assertRegistered(t, r, want)
	assertNotRegistered(t, r, []string{
		actions.FileWriteActionID,
		actions.CodeExecutorActionID,
		actions.BashExecutorActionID,
	})
	assertLevelsAllowed(t, r, levelSet(action.SideEffectNone, action.SideEffectRead))
}

func TestBalancedPolicy(t *testing.T) {
	r := BalancedPolicy()
	want := []string{
		actions.CalculatorActionID,
		actions.WebSearchActionID,
		actions.URLFetchActionID,
		actions.FileReadActionID,
		actions.JSONProcessorActionID,
		actions.FileWriteActionID,
	}
	assertRegistered(t, r, want)
	assertNotRegistered(t, r, []string{
		actions.CodeExecutorActionID,
		actions.BashExecutorActionID,
	})
	assertLevelsAllowed(t, r, levelSet(action.SideEffectNone, action.SideEffectRead, action.SideEffectWrite))
}

func TestPermissivePolicy(t *testing.T) {
	r := PermissivePolicy()
	want := allActionIDs()
	assertRegistered(t, r, want)
	assertLevelsAllowed(t, r, levelSet(
		action.SideEffectNone,
		action.SideEffectRead,
		action.SideEffectWrite,
		action.SideEffectExec,
	))
}

func TestRestrictivePolicyExcludesHighRisk(t *testing.T) {
	r := RestrictivePolicy()
	for _, name := range allActionIDs() {
		level := sideEffectLevel(name)
		if level == action.SideEffectWrite || level == action.SideEffectExec {
			if r.Has(name) {
				t.Errorf("restrictive policy should not register %q (side effect %q)", name, level)
			}
		}
	}
}

func TestBalancedPolicyExcludesExec(t *testing.T) {
	r := BalancedPolicy()
	for _, name := range allActionIDs() {
		level := sideEffectLevel(name)
		if level == action.SideEffectExec {
			if r.Has(name) {
				t.Errorf("balanced policy should not register %q (side effect %q)", name, level)
			}
		}
	}
}

func TestPermissivePolicyGatesHighRisk(t *testing.T) {
	r := PermissivePolicy()
	highRisk := []string{actions.CodeExecutorActionID, actions.BashExecutorActionID}
	for _, name := range highRisk {
		if !r.Has(name) {
			t.Errorf("permissive policy should register %q", name)
		}
	}
	if !actions.CodeExecutorSpec.ApprovalRequired || !actions.CodeExecutorSpec.SandboxRequired {
		t.Error("code_executor spec must require approval and sandbox")
	}
	if !actions.BashExecutorSpec.ApprovalRequired || !actions.BashExecutorSpec.SandboxRequired {
		t.Error("bash_executor spec must require approval and sandbox")
	}
}

func TestBalancedPolicyGatesFileWrite(t *testing.T) {
	r := BalancedPolicy()
	if !r.Has(actions.FileWriteActionID) {
		t.Fatal("balanced policy should register file_write")
	}
	if !actions.FileWriteSpec.ApprovalRequired {
		t.Error("file_write spec must require approval")
	}
}
