package evidence

import "testing"

func TestPresetTDDGate_AllowsWhenEvidenceOK(t *testing.T) {
	g := PresetTDDGate(false)
	allowed, unmet, err := g.Evaluate(map[Key]bool{
		KeyTestsPassed: true,
		KeyCoverageOK:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatalf("want allowed, unmet=%v", unmet)
	}
}

func TestPresetTDDGate_DeniesWhenCoverageMissing(t *testing.T) {
	g := PresetTDDGate(false)
	allowed, unmet, err := g.Evaluate(map[Key]bool{KeyTestsPassed: true})
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("want denied when coverage evidence missing")
	}
	if len(unmet) != 1 || unmet[0] == "" {
		t.Errorf("want a single readable unmet reason, got %v", unmet)
	}
}

func TestPresetTDDGate_RequiresLintWhenFlagged(t *testing.T) {
	g := PresetTDDGate(true)
	if !g.RequiresLint() {
		t.Fatal("lint should be required")
	}
	allowed, _, _ := g.Evaluate(map[Key]bool{
		KeyTestsPassed: true,
		KeyCoverageOK:  true,
	})
	if allowed {
		t.Fatal("want denied when lint evidence missing and lint required")
	}
}

func TestConsequenceGate_DeniesWithoutVerification(t *testing.T) {
	g := ConsequenceGate()
	// Tests pass but no concrete verification artifact → blocked.
	allowed, unmet, err := g.Evaluate(map[Key]bool{KeyTestsPassed: true})
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("consequence gate must block without a verification artifact")
	}
	if len(unmet) != 1 {
		t.Fatalf("want 1 unmet (verification), got %v", unmet)
	}
	// With verification present, allowed.
	allowed, _, _ = g.Evaluate(map[Key]bool{KeyTestsPassed: true, KeyVerification: true})
	if !allowed {
		t.Fatal("want allowed when tests pass and verification present")
	}
}

func TestGate_NilPanics(t *testing.T) {
	var g *Gate
	if _, _, err := g.Evaluate(nil); err == nil {
		t.Fatal("want error for nil gate")
	}
}