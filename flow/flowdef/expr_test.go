package flowdef

import "testing"

func TestEvalWhen_SimpleFieldTrue(t *testing.T) {
	data := map[string]any{"should_run": true}
	got, err := evalWhen("{{.should_run}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("expected true, got false")
	}
}

func TestEvalWhen_SimpleFieldFalse(t *testing.T) {
	data := map[string]any{"should_run": false}
	got, err := evalWhen("{{.should_run}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if got {
		t.Errorf("expected false, got true")
	}
}

func TestEvalWhen_NestedField(t *testing.T) {
	// "{{.triage.priority != 'low'}}" style expression: uses ne on nested map.
	data := map[string]any{
		"triage": map[string]any{"priority": "high"},
	}
	got, err := evalWhen("{{ne .triage.priority `low`}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("expected true for priority=high != low")
	}
}

func TestEvalWhen_NestedFieldFalse(t *testing.T) {
	data := map[string]any{
		"triage": map[string]any{"priority": "low"},
	}
	got, err := evalWhen("{{ne .triage.priority `low`}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if got {
		t.Errorf("expected false for priority=low == low")
	}
}

func TestEvalWhen_AllTruthy(t *testing.T) {
	data := map[string]any{
		"reviewer": map[string]any{"passed": []any{true, true, true}},
	}
	got, err := evalWhen("{{all .reviewer.passed}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("expected all-true to be true")
	}
}

func TestEvalWhen_AllHasFalse(t *testing.T) {
	data := map[string]any{
		"reviewer": map[string]any{"passed": []any{true, false, true}},
	}
	got, err := evalWhen("{{all .reviewer.passed}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if got {
		t.Errorf("expected all-with-false to be false")
	}
}

func TestEvalWhen_AnyTruthy(t *testing.T) {
	data := map[string]any{
		"reviewer": map[string]any{"blocker": []any{false, true, false}},
	}
	got, err := evalWhen("{{any .reviewer.blocker}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("expected any-with-true to be true")
	}
}

func TestEvalWhen_AnyAllFalse(t *testing.T) {
	data := map[string]any{
		"reviewer": map[string]any{"blocker": []any{false, false, false}},
	}
	got, err := evalWhen("{{any .reviewer.blocker}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if got {
		t.Errorf("expected any-all-false to be false")
	}
}

func TestEvalWhen_Len(t *testing.T) {
	data := map[string]any{
		"reviewer": map[string]any{"files": []any{"a.go", "b.go", "c.go"}},
	}
	got, err := evalWhen("{{gt (len .reviewer.files) 2}}", data)
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("expected len=3 > 2 to be true")
	}
}

func TestEvalWhen_EmptyExpr(t *testing.T) {
	// Empty when expression means "always run".
	got, err := evalWhen("", map[string]any{})
	if err != nil {
		t.Fatalf("evalWhen error: %v", err)
	}
	if !got {
		t.Errorf("empty expression should evaluate to true (always run)")
	}
}

func TestEvalWhen_BadTemplate(t *testing.T) {
	_, err := evalWhen("{{ .should_run", map[string]any{"should_run": true})
	if err == nil {
		t.Fatal("expected error for malformed template, got nil")
	}
}
