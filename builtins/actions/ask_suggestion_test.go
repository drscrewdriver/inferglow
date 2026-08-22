package actions

import (
	"context"
	"testing"
)

func TestAskSuggestion_RequiresQuestion(t *testing.T) {
	res, err := NewAskSuggestionAction().Executor.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.OK {
		t.Fatalf("want ok=false ActionResult for missing question, got %#v", res)
	}
	if res.Error == "" {
		t.Fatal("want a readable error message in ActionResult.Error")
	}
}

func TestAskSuggestion_PosesQuestion(t *testing.T) {
	res, err := NewAskSuggestionAction().Executor.Execute(context.Background(), map[string]any{
		"question": "Proceed with destructive cleanup?",
		"context":  "rm -rf /tmp/scratch",
		"options":  []string{"yes", "no"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.OK || res.Status != "posed" {
		t.Fatalf("got %#v, want status=posed ok=true", res)
	}
	out, ok := res.Result.(AskSuggestionResult)
	if !ok {
		t.Fatalf("Result type: got %T, want AskSuggestionResult", res.Result)
	}
	if !out.NeedsOperator || out.Question != "Proceed with destructive cleanup?" {
		t.Errorf("unexpected result: %+v", out)
	}
}