package flow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegrationSequentialFlow tests a complete 3-step sequential flow
func TestIntegrationSequentialFlow(t *testing.T) {
	// Step 1: Input "world" -> output map with key "greeting"
	step1 := NewStep("greet", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"greeting": input.(string)}, nil
	}).Build()

	// Step 2: Input map -> append word, output map with key "message"
	step2 := NewStep("enrich", func(ctx context.Context, input any) (any, error) {
		m := input.(map[string]any)
		m["message"] = m["greeting"].(string) + " world"
		return m, nil
	}).Build()

	// Step 3: Input map -> append final word, output map with key "final"
	step3 := NewStep("finalize", func(ctx context.Context, input any) (any, error) {
		m := input.(map[string]any)
		m["final"] = m["message"].(string) + "!"
		return m, nil
	}).Build()

	flow := NewFlow().AddStep(step1).To(step2).To(step3).Build()

	exec := flow.Execute(context.Background(), "hello")

	// Verify final output contains all transformations
	resultMap, ok := exec.State.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be map[string]any, got %T", exec.State.Result)
	}
	if resultMap["greeting"] != "hello" {
		t.Errorf("expected greeting='hello', got %v", resultMap["greeting"])
	}
	if resultMap["message"] != "hello world" {
		t.Errorf("expected message='hello world', got %v", resultMap["message"])
	}
	if resultMap["final"] != "hello world!" {
		t.Errorf("expected final='hello world!', got %v", resultMap["final"])
	}

	// Verify StepLog has all 3 entries with correct data
	if len(exec.State.StepLog) != 3 {
		t.Fatalf("expected 3 step log entries, got %d", len(exec.State.StepLog))
	}
	for name := range map[string]bool{"greet": true, "enrich": true, "finalize": true} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("expected step log entry for %s", name)
		}
	}

	// Verify StepLog entries have correct input/output
	greetLog := exec.State.StepLog["greet"]
	if greetLog == nil {
		t.Fatal("missing greet log entry")
	}
	if greetLog.Input != "hello" {
		t.Errorf("expected greet input 'hello', got %v", greetLog.Input)
	}
	greetResult := greetLog.Output.(map[string]any)
	if greetResult["greeting"] != "hello" {
		t.Errorf("expected greet output greeting='hello', got %v", greetResult["greeting"])
	}

	enrichLog := exec.State.StepLog["enrich"]
	if enrichLog == nil {
		t.Fatal("missing enrich log entry")
	}
	enrichInput := enrichLog.Input.(map[string]any)
	if enrichInput["greeting"] != "hello" {
		t.Errorf("expected enrich input to contain greeting='hello', got %v", enrichInput)
	}

	// Verify Status == StatusCompleted
	if exec.State.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", exec.State.Status)
	}
}

// TestIntegrationFlowWithBranch tests the true/positive path of a conditional branch
func TestIntegrationFlowWithBranch(t *testing.T) {
	// Step "input": outputs map["valid"] = true
	stepInput := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": true, "data": "application"}, nil
	}).Build()

	// Step "approve": returns "approved"
	stepApprove := NewStep("approve", func(ctx context.Context, input any) (any, error) {
		return "approved", nil
	}).Build()

	// Step "deny": returns "denied"
	stepDeny := NewStep("deny", func(ctx context.Context, input any) (any, error) {
		return "denied", nil
	}).Build()

	// Step "finalize": appends "done" to input
	stepFinalize := NewStep("finalize", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "done", nil
	}).Build()

	// Build flow with branch: input -> If(valid==true) -> approve -> finalize, else deny -> finalize
	flow := NewFlow().
		AddStep(stepInput).
		If(func(output any) bool {
			m := output.(map[string]any)
			return m["valid"].(bool)
		}, stepApprove, stepDeny).
		To(stepFinalize).
		Build()

	exec := flow.Execute(context.Background(), nil)

	// Verify result is "approveddone" (approve returns "approved", finalize appends "done")
	if exec.State.Result != "approveddone" {
		t.Errorf("expected result 'approveddone', got %v", exec.State.Result)
	}

	// Verify Status == StatusCompleted
	if exec.State.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", exec.State.Status)
	}

	// Verify StepLog shows input -> approve -> finalize (no deny entry)
	if len(exec.State.StepLog) != 3 {
		t.Fatalf("expected 3 step log entries, got %d", len(exec.State.StepLog))
	}
	for name := range map[string]bool{"input": true, "approve": true, "finalize": true} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("expected step log entry for %s", name)
		}
	}
	if _, ok := exec.State.StepLog["deny"]; ok {
		t.Error("deny step should NOT have been executed")
	}
}

// TestIntegrationFlowWithBranchFalsePath tests the false/negative path of a conditional branch
func TestIntegrationFlowWithBranchFalsePath(t *testing.T) {
	// Step "input": outputs map["valid"] = false
	stepInput := NewStep("input", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"valid": false, "data": "application"}, nil
	}).Build()

	// Step "approve": returns "approved"
	stepApprove := NewStep("approve", func(ctx context.Context, input any) (any, error) {
		return "approved", nil
	}).Build()

	// Step "deny": returns "denied"
	stepDeny := NewStep("deny", func(ctx context.Context, input any) (any, error) {
		return "denied", nil
	}).Build()

	// Step "finalize": appends "done" to input
	stepFinalize := NewStep("finalize", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "done", nil
	}).Build()

	// Build flow with branch: input -> If(valid==true) -> approve -> finalize, else deny
	// Note: To(stepFinalize) chains from the trueStep (approve), not the falseStep (deny)
	// So when condition is false, execution goes input -> deny and stops (deny has no To edge)
	flow := NewFlow().
		AddStep(stepInput).
		If(func(output any) bool {
			m := output.(map[string]any)
			return m["valid"].(bool)
		}, stepApprove, stepDeny).
		To(stepFinalize).
		Build()

	exec := flow.Execute(context.Background(), nil)

	// For false path: input -> deny, execution stops at deny because deny has no To edge
	// The result is "denied" (the output of the last executed step)
	if exec.State.Result != "denied" {
		t.Errorf("expected result 'denied', got %v", exec.State.Result)
	}

	// Verify Status == StatusCompleted
	if exec.State.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", exec.State.Status)
	}

	// Verify StepLog shows input -> deny (2 entries, no finalize because deny has no To edge)
	if len(exec.State.StepLog) != 2 {
		t.Fatalf("expected 2 step log entries, got %d", len(exec.State.StepLog))
	}
	for name := range map[string]bool{"input": true, "deny": true} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("expected step log entry for %s", name)
		}
	}
	if _, ok := exec.State.StepLog["finalize"]; ok {
		t.Error("finalize step should NOT have been executed (no edge from deny)")
	}
	if _, ok := exec.State.StepLog["approve"]; ok {
		t.Error("approve step should NOT have been executed")
	}
}

// TestIntegrationFlowWithPauseAndResume tests pause and resume functionality
func TestIntegrationFlowWithPauseAndResume(t *testing.T) {
	// Define 3 steps: s1 -> s2 -> s3
	step1 := NewStep("s1", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_step1", nil
	}).Build()

	step2 := NewStep("s2", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_step2", nil
	}).Build()

	step3 := NewStep("s3", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_step3", nil
	}).Build()

	flow := NewFlow().AddStep(step1).To(step2).To(step3).Build()

	// Execute to completion (this will actually complete all steps since there's no
	// real pause trigger in Execute; we pause the resulting Execution manually)
	exec := flow.Execute(context.Background(), "start")

	// Pause the execution
	pp := exec.Pause("review needed")
	if pp == nil {
		t.Fatal("expected PausePoint to be non-nil")
	}
	if pp.StepName == "" {
		t.Error("expected PausePoint.StepName to be non-empty")
	}
	// Pause takes the last executed step's Input from StepLog, not the final Result.
	// s3's input is start_step1_step2 (s2's output)
	if pp.Input != "start_step1_step2" {
		t.Errorf("expected PausePoint.Input to be 'start_step1_step2', got %v", pp.Input)
	}
	if exec.State.Status != StatusPaused {
		t.Errorf("expected StatusPaused after Pause, got %s", exec.State.Status)
	}

	// Resume with new input "resumed"
	resumedExec := flow.Resume(context.Background(), pp, "resumed")
	if resumedExec == nil {
		t.Fatal("expected resumed Execution to be non-nil")
	}

	// Verify the resumed execution completed successfully
	if resumedExec.State.Status != StatusCompleted {
		t.Errorf("expected resumed execution StatusCompleted, got %s", resumedExec.State.Status)
	}
}

// TestIntegrationFlowWithErrorHandling tests that errors stop execution and log correctly
func TestIntegrationFlowWithErrorHandling(t *testing.T) {
	testErr := errors.New("step2 always fails")

	// s1 succeeds
	step1 := NewStep("s1", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_s1", nil
	}).Build()

	// s2 always fails
	step2 := NewStep("s2", func(ctx context.Context, input any) (any, error) {
		return nil, testErr
	}).Build()

	// s3 should never execute
	step3 := NewStep("s3", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "_s3", nil
	}).Build()

	flow := NewFlow().AddStep(step1).To(step2).To(step3).Build()

	exec := flow.Execute(context.Background(), "start")

	// Verify Status == StatusFailed
	if exec.State.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", exec.State.Status)
	}

	// Verify Errors contains the error from s2
	if len(exec.State.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(exec.State.Errors))
	}
	if exec.State.Errors[0] != testErr {
		t.Errorf("expected error to be testErr, got %v", exec.State.Errors[0])
	}

	// Verify StepLog has s1 and s2 entries, NO s3 entry
	if len(exec.State.StepLog) != 2 {
		t.Fatalf("expected 2 step log entries, got %d", len(exec.State.StepLog))
	}
	for name := range map[string]bool{"s1": true, "s2": true} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("expected step log entry for %s", name)
		}
	}
	if _, ok := exec.State.StepLog["s3"]; ok {
		t.Error("s3 should NOT have been executed")
	}

	// Verify s2's StepLogEntry.Error is set
	s2Log := exec.State.StepLog["s2"]
	if s2Log == nil {
		t.Fatal("missing s2 log entry")
	}
	if s2Log.Error != testErr {
		t.Errorf("expected s2 log error to be testErr, got %v", s2Log.Error)
	}
}

// TestP1Integration_OperatorRuntimeWithPersistence exercises the P1 operator
// runtime (OperatorRegistry + SignalNet + OperatorRuntime) together with the
// P1 ExecutionPersistence JSON round-trip.
//
// It dispatches a Chunk operator whose EmitSignal records and accepts a
// "Chunk[<name>]" signal, then persists an Execution containing a StepLog
// entry derived from the dispatch result and reloads it to verify the
// StepLog history is preserved end-to-end.
func TestP1Integration_OperatorRuntimeWithPersistence(t *testing.T) {
	// 1. Construct OperatorRegistry + SignalNet + OperatorRuntime.
	reg := NewOperatorRegistry()
	sn := NewSignalNet()
	rt := NewOperatorRuntime(reg, sn)

	// 2. Register ChunkHandler + SignalGateHandler on the runtime.
	rt.RegisterHandler(&ChunkHandler{})
	rt.RegisterHandler(&SignalGateHandler{})

	// 3. Register a Chunk operator in the registry.
	chunkOp := &Operator{
		ID:            "op-chunk",
		Kind:          OpChunk,
		Name:          "my_chunk",
		ListenSignals: []string{"START"},
		EmitSignals:   []string{"Chunk[my_chunk]"},
	}
	if err := reg.Register(chunkOp); err != nil {
		t.Fatalf("Register chunkOp failed: %v", err)
	}

	// 4. Dispatch the Chunk operator with input "hello".
	//    EmitSignal both records the signal and accepts it on the SignalNet
	//    so downstream consumers (e.g. BatchCollect) can read it back.
	var emittedSignals []Signal
	oc := &OperatorContext{
		Ctx:       context.Background(),
		Operator:  chunkOp,
		Input:     "hello",
		SignalNet: sn,
		EmitSignal: func(s Signal) {
			emittedSignals = append(emittedSignals, s)
			sn.AcceptSignal(&s)
		},
	}
	out, err := rt.Dispatch(oc)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// 5. Verify the dispatch returned "hello" (chunk passthrough).
	if out != "hello" {
		t.Errorf("output = %v, want \"hello\"", out)
	}

	// 6. Verify the signal was emitted with the expected TriggerEvent and Value.
	if len(emittedSignals) != 1 {
		t.Fatalf("expected 1 emitted signal, got %d", len(emittedSignals))
	}
	if emittedSignals[0].TriggerEvent != "Chunk[my_chunk]" {
		t.Errorf("emitted TriggerEvent = %q, want \"Chunk[my_chunk]\"", emittedSignals[0].TriggerEvent)
	}
	if emittedSignals[0].Value != "hello" {
		t.Errorf("emitted Value = %v, want \"hello\"", emittedSignals[0].Value)
	}
	if !sn.IsAccepted("Chunk[my_chunk]") {
		t.Error("expected signal \"Chunk[my_chunk]\" to be accepted by SignalNet")
	}

	// 7. Build an Execution whose StepLog records this dispatch.
	exec := &Execution{
		State: ExecutionState{
			Status: StatusCompleted,
			Result: out,
			StepLog: map[string]*StepLogEntry{
				"my_chunk": {
					StepName: "my_chunk",
					Input:    "hello",
					Output:   out,
					Duration: 1 * time.Millisecond,
				},
			},
		},
	}

	// 8. Persist via ExecutionPersistence.SaveJSON.
	dir := t.TempDir()
	path := filepath.Join(dir, "p1_operator_runtime.json")
	persist := NewExecutionPersistence(exec, "p1-integration-flow")
	snapshot, err := persist.SaveJSON(path)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}
	if snapshot.SchemaVersion != "v1" {
		t.Errorf("snapshot.SchemaVersion = %q, want \"v1\"", snapshot.SchemaVersion)
	}
	if snapshot.FlowName != "p1-integration-flow" {
		t.Errorf("snapshot.FlowName = %q, want \"p1-integration-flow\"", snapshot.FlowName)
	}
	if len(snapshot.StepLog) != 1 {
		t.Errorf("snapshot.StepLog len = %d, want 1", len(snapshot.StepLog))
	}

	// 9. LoadJSON to restore the Execution in a "new process".
	loaded, err := persist.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	// 10. Verify the StepLog history is complete.
	if loaded.State.Status != StatusCompleted {
		t.Errorf("loaded.Status = %q, want %q", loaded.State.Status, StatusCompleted)
	}
	if len(loaded.State.StepLog) != 1 {
		t.Fatalf("loaded.StepLog len = %d, want 1", len(loaded.State.StepLog))
	}
	entry := loaded.State.StepLog["my_chunk"]
	if entry == nil {
		t.Fatal("loaded StepLog missing \"my_chunk\" entry")
	}
	if entry.StepName != "my_chunk" {
		t.Errorf("entry.StepName = %q, want \"my_chunk\"", entry.StepName)
	}
	if entry.Input != "hello" {
		t.Errorf("entry.Input = %v, want \"hello\"", entry.Input)
	}
	if entry.Output != "hello" {
		t.Errorf("entry.Output = %v, want \"hello\"", entry.Output)
	}
	if entry.Duration != 1*time.Millisecond {
		t.Errorf("entry.Duration = %v, want 1ms", entry.Duration)
	}
	if entry.Error != nil {
		t.Errorf("entry.Error = %v, want nil", entry.Error)
	}
}

// TestP1Integration_BatchFanoutCollect exercises the BatchFanout + BatchCollect
// operator pair end-to-end via the OperatorRuntime.
//
// BatchFanout emits one "BatchItem[i]" signal per input element; the
// EmitSignal hook accepts each signal on the SignalNet so BatchCollect can
// later poll and merge them by index.
func TestP1Integration_BatchFanoutCollect(t *testing.T) {
	reg := NewOperatorRegistry()
	sn := NewSignalNet()
	rt := NewOperatorRuntime(reg, sn)

	rt.RegisterHandler(&BatchFanoutHandler{})
	rt.RegisterHandler(&BatchCollectHandler{})

	// Register a BatchFanout operator (no item handler -> passthrough).
	fanoutOp := &Operator{
		ID:          "op-fanout",
		Kind:        OpBatchFanout,
		Name:        "fanout",
		EmitSignals: []string{"BatchItem[*]"},
	}
	if err := reg.Register(fanoutOp); err != nil {
		t.Fatalf("Register fanoutOp failed: %v", err)
	}

	// Dispatch BatchFanout with input []any{"a", "b", "c"}.
	fanoutOC := &OperatorContext{
		Ctx:      context.Background(),
		Operator: fanoutOp,
		Input:    []any{"a", "b", "c"},
		SignalNet: sn,
		EmitSignal: func(s Signal) {
			// BatchCollect reads via GetAcceptedSignal, so we must accept.
			sn.AcceptSignal(&s)
		},
	}
	fanoutOut, err := rt.Dispatch(fanoutOC)
	if err != nil {
		t.Fatalf("Dispatch fanout failed: %v", err)
	}

	// Verify fanout returned the passthrough []any of length 3.
	fanoutResults, ok := fanoutOut.([]any)
	if !ok {
		t.Fatalf("expected fanout output []any, got %T", fanoutOut)
	}
	if len(fanoutResults) != 3 {
		t.Fatalf("expected 3 fanout results, got %d", len(fanoutResults))
	}
	for i, want := range []string{"a", "b", "c"} {
		if fanoutResults[i] != want {
			t.Errorf("fanoutResults[%d] = %v, want %q", i, fanoutResults[i], want)
		}
	}

	// Verify 3 BatchItem signals were emitted and accepted by the SignalNet.
	expectedSignals := []string{"BatchItem[0]", "BatchItem[1]", "BatchItem[2]"}
	for _, sigID := range expectedSignals {
		if !sn.IsAccepted(sigID) {
			t.Errorf("expected signal %q to be accepted", sigID)
		}
	}

	// Register a BatchCollect operator expecting 3 items.
	collectOp := &Operator{
		ID:   "op-collect",
		Kind: OpBatchCollect,
		Name: "collect",
		Options: map[string]any{
			"expected_count": 3,
		},
	}
	if err := reg.Register(collectOp); err != nil {
		t.Fatalf("Register collectOp failed: %v", err)
	}

	// Dispatch BatchCollect — it will poll until all 3 BatchItem signals
	// are accepted (they already are), then merge them by index.
	collectOC := &OperatorContext{
		Ctx:       context.Background(),
		Operator:  collectOp,
		SignalNet: sn,
	}
	collectOut, err := rt.Dispatch(collectOC)
	if err != nil {
		t.Fatalf("Dispatch collect failed: %v", err)
	}

	// Verify collected result is []any{"a", "b", "c"} in correct order.
	collected, ok := collectOut.([]any)
	if !ok {
		t.Fatalf("expected collect output []any, got %T", collectOut)
	}
	if len(collected) != 3 {
		t.Fatalf("expected 3 collected items, got %d", len(collected))
	}
	for i, want := range []string{"a", "b", "c"} {
		if collected[i] != want {
			t.Errorf("collected[%d] = %v, want %q", i, collected[i], want)
		}
	}
}
