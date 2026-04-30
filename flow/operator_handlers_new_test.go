package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// ForEachSplitHandler 测试
// ============================================================================

func TestForEachSplitHandlerKind(t *testing.T) {
	h := &ForEachSplitHandler{}
	if h.Kind() != OpForEachSplit {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpForEachSplit)
	}
}

func TestForEachSplitHandlerEmitsSignals(t *testing.T) {
	h := &ForEachSplitHandler{}
	var mu sync.Mutex
	var emitted []Signal
	sn := NewSignalNet()
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpForEachSplit,
			Name: "split",
		},
		Input:      []any{"a", "b", "c"},
		SignalNet:  sn,
		EmitSignal: func(s Signal) { mu.Lock(); emitted = append(emitted, s); mu.Unlock() },
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got %v", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 3 {
		t.Fatalf("expected 3 signals, got %d", len(emitted))
	}
	for i, want := range []string{"BatchItem[0]", "BatchItem[1]", "BatchItem[2]"} {
		if emitted[i].TriggerEvent != want {
			t.Errorf("emitted[%d].TriggerEvent = %q, want %q", i, emitted[i].TriggerEvent, want)
		}
	}
	// SignalNet 也应记录 3 个信号
	if !sn.IsAccepted("BatchItem[0]") || !sn.IsAccepted("BatchItem[1]") || !sn.IsAccepted("BatchItem[2]") {
		t.Error("SignalNet should have accepted all BatchItem signals")
	}
}

func TestForEachSplitHandlerBadInput(t *testing.T) {
	h := &ForEachSplitHandler{}
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpForEachSplit, Name: "split"},
		Input:    "not_a_slice",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for non-slice input")
	}
}

func TestForEachSplitHandlerEmptySlice(t *testing.T) {
	h := &ForEachSplitHandler{}
	oc := &OperatorContext{
		Ctx:        context.Background(),
		Operator:   &Operator{ID: "op-1", Kind: OpForEachSplit, Name: "split"},
		Input:      []any{},
		SignalNet:  NewSignalNet(),
		EmitSignal: func(s Signal) {},
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for empty slice, got %v", out)
	}
}

// ============================================================================
// ForEachCollectHandler 测试
// ============================================================================

func TestForEachCollectHandlerKind(t *testing.T) {
	h := &ForEachCollectHandler{}
	if h.Kind() != OpForEachCollect {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpForEachCollect)
	}
}

func TestForEachCollectHandlerWaitAndMerge(t *testing.T) {
	sn := NewSignalNet()
	h := &ForEachCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpForEachCollect,
			Name: "collect",
			Options: map[string]any{
				"expected_count": 3,
			},
		},
		SignalNet: sn,
	}
	go func() {
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[0]", TriggerEvent: "BatchItem[0]", Value: "x0"})
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[1]", TriggerEvent: "BatchItem[1]", Value: "x1"})
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[2]", TriggerEvent: "BatchItem[2]", Value: "x2"})
	}()
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	want := []string{"x0", "x1", "x2"}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("results[%d] = %v, want %s", i, results[i], w)
		}
	}
}

func TestForEachCollectHandlerZeroCount(t *testing.T) {
	h := &ForEachCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpForEachCollect,
			Name: "collect",
			Options: map[string]any{
				"expected_count": 0,
			},
		},
		SignalNet: NewSignalNet(),
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok || len(results) != 0 {
		t.Errorf("expected empty []any, got %v", out)
	}
}

// ============================================================================
// MatchCaseHandler 测试
// ============================================================================

func TestMatchCaseHandlerKind(t *testing.T) {
	h := &MatchCaseHandler{}
	if h.Kind() != OpMatchCase {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpMatchCase)
	}
}

func TestMatchCaseHandlerFirstMatch(t *testing.T) {
	h := &MatchCaseHandler{}
	cases := []CaseSpec{
		{
			ID:        "negative",
			Predicate: func(in any) bool { return in.(int) < 0 },
			Handler: func(rd *TriggerFlowRuntimeData) (any, error) {
				return "neg", nil
			},
		},
		{
			ID:        "zero",
			Predicate: func(in any) bool { return in.(int) == 0 },
			Handler: func(rd *TriggerFlowRuntimeData) (any, error) {
				return "zero", nil
			},
		},
		{
			ID:        "positive",
			Predicate: func(in any) bool { return in.(int) > 0 },
			Handler: func(rd *TriggerFlowRuntimeData) (any, error) {
				return "pos", nil
			},
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCase,
			Name: "case",
			Options: map[string]any{
				"cases": cases,
			},
		},
		Input: 5,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "pos" {
		t.Errorf("output = %v, want pos", out)
	}
}

func TestMatchCaseHandlerNoMatchWithDefault(t *testing.T) {
	h := &MatchCaseHandler{}
	cases := []CaseSpec{
		{
			ID:        "only_even",
			Predicate: func(in any) bool { return in.(int)%2 == 0 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "even", nil },
		},
	}
	var defHandler func(*TriggerFlowRuntimeData) (any, error) = func(rd *TriggerFlowRuntimeData) (any, error) {
		return "odd_default", nil
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCase,
			Name: "case",
			Options: map[string]any{
				"cases":   cases,
				"default": defHandler,
			},
		},
		Input: 7,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "odd_default" {
		t.Errorf("output = %v, want odd_default", out)
	}
}

func TestMatchCaseHandlerNoMatchNoDefault(t *testing.T) {
	h := &MatchCaseHandler{}
	cases := []CaseSpec{
		{
			ID:        "only_even",
			Predicate: func(in any) bool { return in.(int)%2 == 0 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "even", nil },
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCase,
			Name: "case",
			Options: map[string]any{
				"cases": cases,
			},
		},
		Input: 7,
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for no match and no default")
	}
}

func TestMatchCaseHandlerMissingCases(t *testing.T) {
	h := &MatchCaseHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpMatchCase,
			Name:    "case",
			Options: map[string]any{},
		},
		Input: 1,
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for missing cases")
	}
}

// ============================================================================
// MatchCollectHandler 测试
// ============================================================================

func TestMatchCollectHandlerKind(t *testing.T) {
	h := &MatchCollectHandler{}
	if h.Kind() != OpMatchCollect {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpMatchCollect)
	}
}

func TestMatchCollectHandlerMultipleMatches(t *testing.T) {
	h := &MatchCollectHandler{}
	cases := []CaseSpec{
		{
			ID:        "divisible_by_2",
			Predicate: func(in any) bool { return in.(int)%2 == 0 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "even", nil },
		},
		{
			ID:        "divisible_by_3",
			Predicate: func(in any) bool { return in.(int)%3 == 0 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "div3", nil },
		},
		{
			ID:        "greater_than_100",
			Predicate: func(in any) bool { return in.(int) > 100 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "big", nil },
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCollect,
			Name: "collect",
			Options: map[string]any{
				"cases": cases,
			},
		},
		Input: 6, // divisible by 2 AND 3, but not > 100
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}
	if m["divisible_by_2"] != "even" {
		t.Errorf("divisible_by_2 = %v, want even", m["divisible_by_2"])
	}
	if m["divisible_by_3"] != "div3" {
		t.Errorf("divisible_by_3 = %v, want div3", m["divisible_by_3"])
	}
	if _, exists := m["greater_than_100"]; exists {
		t.Error("greater_than_100 should NOT match input 6")
	}
}

func TestMatchCollectHandlerNoMatchesReturnsEmptyMap(t *testing.T) {
	h := &MatchCollectHandler{}
	cases := []CaseSpec{
		{
			ID:        "only_negative",
			Predicate: func(in any) bool { return in.(int) < 0 },
			Handler:   func(rd *TriggerFlowRuntimeData) (any, error) { return "neg", nil },
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCollect,
			Name: "collect",
			Options: map[string]any{
				"cases": cases,
			},
		},
		Input: 5,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestMatchCollectHandlerCaseError(t *testing.T) {
	h := &MatchCollectHandler{}
	cases := []CaseSpec{
		{
			ID:        "fail_case",
			Predicate: func(in any) bool { return true },
			Handler: func(rd *TriggerFlowRuntimeData) (any, error) {
				return nil, errors.New("case failed")
			},
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchCollect,
			Name: "collect",
			Options: map[string]any{
				"cases": cases,
			},
		},
		Input: 1,
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error from case handler")
	}
	if !strings.Contains(err.Error(), "fail_case") {
		t.Errorf("error should mention case id 'fail_case', got: %v", err)
	}
}

// ============================================================================
// CollectBranchHandler 测试
// ============================================================================

func TestCollectBranchHandlerKind(t *testing.T) {
	h := &CollectBranchHandler{}
	if h.Kind() != OpCollectBranch {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpCollectBranch)
	}
}

func TestCollectBranchHandlerParallel(t *testing.T) {
	h := &CollectBranchHandler{}
	branches := map[string]Handler{
		"b1": func(rd *TriggerFlowRuntimeData) (any, error) {
			time.Sleep(5 * time.Millisecond)
			return "r1", nil
		},
		"b2": func(rd *TriggerFlowRuntimeData) (any, error) {
			return "r2", nil
		},
		"b3": func(rd *TriggerFlowRuntimeData) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return "r3", nil
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpCollectBranch,
			Name: "branch",
			Options: map[string]any{
				"branches": branches,
			},
		},
		Input: "input",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}
	if len(m) != 3 {
		t.Fatalf("expected 3 results, got %d", len(m))
	}
	if m["b1"] != "r1" || m["b2"] != "r2" || m["b3"] != "r3" {
		t.Errorf("results = %v", m)
	}
}

func TestCollectBranchHandlerError(t *testing.T) {
	h := &CollectBranchHandler{}
	branches := map[string]Handler{
		"ok":   func(rd *TriggerFlowRuntimeData) (any, error) { return "ok", nil },
		"fail": func(rd *TriggerFlowRuntimeData) (any, error) { return nil, errors.New("branch failed") },
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpCollectBranch,
			Name: "branch",
			Options: map[string]any{
				"branches": branches,
			},
		},
		Input: "input",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error from failed branch")
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("error should mention 'fail' branch, got: %v", err)
	}
}

func TestCollectBranchHandlerMissingBranches(t *testing.T) {
	h := &CollectBranchHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpCollectBranch,
			Name:    "branch",
			Options: map[string]any{},
		},
		Input: "input",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for missing branches")
	}
}

// ============================================================================
// InterventionPointHandler 测试
// ============================================================================

func TestInterventionPointHandlerKind(t *testing.T) {
	h := &InterventionPointHandler{}
	if h.Kind() != OpIntervention {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpIntervention)
	}
}

func TestInterventionPointHandlerWaitForResume(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	var mu sync.Mutex
	var emitted []Signal
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "pause1",
		},
		Input:      "original_input",
		SignalNet:  sn,
		EmitSignal: func(s Signal) { mu.Lock(); emitted = append(emitted, s); mu.Unlock() },
	}
	// 异步发送 resume 信号
	go func() {
		time.Sleep(20 * time.Millisecond)
		sn.AcceptSignal(&Signal{
			ID:           "Resume[pause1]",
			TriggerEvent: "Resume[pause1]",
			Value:        "resumed_value",
		})
	}()
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "resumed_value" {
		t.Errorf("output = %v, want resumed_value", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 intervention signal, got %d", len(emitted))
	}
	if emitted[0].TriggerEvent != "Intervention[pause1]" {
		t.Errorf("emitted TriggerEvent = %q, want Intervention[pause1]", emitted[0].TriggerEvent)
	}
}

func TestInterventionPointHandlerCustomResumeSignal(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "pause2",
			Options: map[string]any{
				"resume_signal": "CustomResume",
			},
		},
		Input:     "input_val",
		SignalNet: sn,
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "CustomResume", TriggerEvent: "CustomResume"})
	}()
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 没有自定义 resume value，应返回原 input
	if out != "input_val" {
		t.Errorf("output = %v, want input_val", out)
	}
}

func TestInterventionPointHandlerContextCancel(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	ctx, cancel := context.WithCancel(context.Background())
	oc := &OperatorContext{
		Ctx: ctx,
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "pause3",
		},
		Input:     "input",
		SignalNet: sn,
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error on context cancel")
	}
}

// ============================================================================
// SubFlowHandler 测试（骨架）
// ============================================================================

func TestSubFlowHandlerKind(t *testing.T) {
	h := &SubFlowHandler{}
	if h.Kind() != OpSubFlow {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpSubFlow)
	}
}

func TestSubFlowHandlerSkeletonNoExecutor(t *testing.T) {
	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpSubFlow,
			Name:    "sub",
			Options: map[string]any{},
		},
		Input: "passthrough",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "passthrough" {
		t.Errorf("output = %v, want passthrough (skeleton)", out)
	}
}

func TestSubFlowHandlerWithExecutor(t *testing.T) {
	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return in.(string) + "_processed", nil
				},
			},
		},
		Input: "data",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "data_processed" {
		t.Errorf("output = %v, want data_processed", out)
	}
}

func TestSubFlowHandlerExecutorError(t *testing.T) {
	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return nil, errors.New("child flow failed")
				},
			},
		},
		Input: "data",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error from child executor")
	}
	if !strings.Contains(err.Error(), "child flow failed") {
		t.Errorf("error should contain 'child flow failed', got: %v", err)
	}
}

// ============================================================================
// ResultSinkHandler 测试
// ============================================================================

func TestResultSinkHandlerKind(t *testing.T) {
	h := &ResultSinkHandler{}
	if h.Kind() != OpResultSink {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpResultSink)
	}
}

func TestResultSinkHandlerEmitsAndAccepts(t *testing.T) {
	h := &ResultSinkHandler{}
	var mu sync.Mutex
	var emitted []Signal
	sn := NewSignalNet()
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpResultSink,
			Name: "sink1",
		},
		Input:      "final_result",
		SignalNet:  sn,
		EmitSignal: func(s Signal) { mu.Lock(); emitted = append(emitted, s); mu.Unlock() },
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "final_result" {
		t.Errorf("output = %v, want final_result", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(emitted))
	}
	if emitted[0].TriggerEvent != "ResultSink[sink1]" {
		t.Errorf("TriggerEvent = %q, want ResultSink[sink1]", emitted[0].TriggerEvent)
	}
	if emitted[0].Value != "final_result" {
		t.Errorf("Value = %v, want final_result", emitted[0].Value)
	}
	if !sn.IsAccepted("ResultSink[sink1]") {
		t.Error("SignalNet should have accepted ResultSink signal")
	}
}

func TestResultSinkHandlerNilEmit(t *testing.T) {
	h := &ResultSinkHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpResultSink,
			Name: "sink2",
		},
		Input:     42,
		SignalNet: NewSignalNet(),
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != 42 {
		t.Errorf("output = %v, want 42", out)
	}
}
