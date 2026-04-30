package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestExecution constructs an Execution with StatusPaused and two StepLog
// entries: one successful (stepA) and one failed (stepB).
func buildTestExecution() *Execution {
	return &Execution{
		State: ExecutionState{
			Status: StatusPaused,
			StepLog: map[string]*StepLogEntry{
				"stepA": {
					StepName: "stepA",
					Input:    "start",
					Output:   "startA",
					Duration: 5 * time.Millisecond,
					Error:    nil,
				},
				"stepB": {
					StepName: "stepB",
					Input:    "startA",
					Output:   nil,
					Duration: 2 * time.Millisecond,
					Error:    fmt.Errorf("stepB failed"),
				},
			},
			Result: "startA",
		},
	}
}

// TestSaveJSON verifies that SaveJSON writes a file containing the required fields.
func TestSaveJSON(t *testing.T) {
	exec := buildTestExecution()
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.json")

	p := NewExecutionPersistence(exec, "test-flow")
	snapshot, err := p.SaveJSON(path)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("snapshot file is empty")
	}

	// Verify returned snapshot has expected fields
	if snapshot.SchemaVersion != "v1" {
		t.Errorf("expected SchemaVersion 'v1', got %q", snapshot.SchemaVersion)
	}
	if snapshot.FlowName != "test-flow" {
		t.Errorf("expected FlowName 'test-flow', got %q", snapshot.FlowName)
	}
	if snapshot.ExecutionID == "" {
		t.Error("expected non-empty ExecutionID")
	}
	if snapshot.Status != StatusPaused {
		t.Errorf("expected Status %q, got %q", StatusPaused, snapshot.Status)
	}
	if len(snapshot.StepLog) != 2 {
		t.Errorf("expected 2 StepLog entries, got %d", len(snapshot.StepLog))
	}
	if snapshot.PausedAt != "" {
		t.Errorf("expected empty PausedAt, got %q", snapshot.PausedAt)
	}
	if snapshot.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	// Verify file content contains the required JSON keys
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(raw)
	requiredKeys := []string{
		"schema_version",
		"execution_id",
		"flow_name",
		"status",
		"step_log",
	}
	for _, key := range requiredKeys {
		if !strings.Contains(content, "\""+key+"\"") {
			t.Errorf("JSON output missing required key %q", key)
		}
	}

	// Decode the JSON to verify StepLogEntrySnapshot fields
	var decoded ExecutionSnapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.SchemaVersion != "v1" {
		t.Errorf("decoded SchemaVersion = %q, want v1", decoded.SchemaVersion)
	}
	if decoded.Status != StatusPaused {
		t.Errorf("decoded Status = %q, want %q", decoded.Status, StatusPaused)
	}
	if entry := decoded.StepLog["stepA"]; entry == nil {
		t.Error("decoded StepLog missing stepA")
	} else {
		if entry.StepName != "stepA" {
			t.Errorf("stepA StepName = %q, want stepA", entry.StepName)
		}
		if entry.DurationMS != 5 {
			t.Errorf("stepA DurationMS = %d, want 5", entry.DurationMS)
		}
		if entry.Error != "" {
			t.Errorf("stepA Error = %q, want empty", entry.Error)
		}
	}
	if entry := decoded.StepLog["stepB"]; entry == nil {
		t.Error("decoded StepLog missing stepB")
	} else if entry.Error != "stepB failed" {
		t.Errorf("stepB Error = %q, want 'stepB failed'", entry.Error)
	}
}

// TestLoadJSON verifies that LoadJSON restores Execution state, including
// reconstructing errors in StepLog via errors.New.
func TestLoadJSON(t *testing.T) {
	exec := buildTestExecution()
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.json")

	p := NewExecutionPersistence(exec, "test-flow")
	if _, err := p.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	loaded, err := p.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	if loaded.State.Status != StatusPaused {
		t.Errorf("expected Status %q, got %q", StatusPaused, loaded.State.Status)
	}
	if len(loaded.State.StepLog) != 2 {
		t.Errorf("expected 2 StepLog entries, got %d", len(loaded.State.StepLog))
	}

	entryA := loaded.State.StepLog["stepA"]
	if entryA == nil {
		t.Fatal("missing stepA entry")
	}
	if entryA.StepName != "stepA" {
		t.Errorf("stepA StepName = %q, want stepA", entryA.StepName)
	}
	if entryA.Input != "start" {
		t.Errorf("stepA Input = %v, want start", entryA.Input)
	}
	if entryA.Output != "startA" {
		t.Errorf("stepA Output = %v, want startA", entryA.Output)
	}
	if entryA.Duration != 5*time.Millisecond {
		t.Errorf("stepA Duration = %v, want %v", entryA.Duration, 5*time.Millisecond)
	}
	if entryA.Error != nil {
		t.Errorf("stepA Error = %v, want nil", entryA.Error)
	}

	entryB := loaded.State.StepLog["stepB"]
	if entryB == nil {
		t.Fatal("missing stepB entry")
	}
	if entryB.Error == nil {
		t.Fatal("expected stepB Error to be restored as non-nil")
	}
	if entryB.Error.Error() != "stepB failed" {
		t.Errorf("stepB Error = %q, want 'stepB failed'", entryB.Error.Error())
	}
	if loaded.State.Errors != nil {
		t.Errorf("expected State.Errors to be nil, got %v", loaded.State.Errors)
	}
}

// TestSaveLoadYAML verifies that SaveYAML and LoadYAML produce equivalent state.
func TestSaveLoadYAML(t *testing.T) {
	exec := buildTestExecution()
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.yaml")

	p := NewExecutionPersistence(exec, "test-flow-yaml")
	snapshot, err := p.SaveYAML(path)
	if err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}
	if snapshot.SchemaVersion != "v1" {
		t.Errorf("expected SchemaVersion 'v1', got %q", snapshot.SchemaVersion)
	}
	if snapshot.FlowName != "test-flow-yaml" {
		t.Errorf("expected FlowName 'test-flow-yaml', got %q", snapshot.FlowName)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("yaml file not created: %v", err)
	}

	loaded, err := p.LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	if loaded.State.Status != StatusPaused {
		t.Errorf("expected Status %q, got %q", StatusPaused, loaded.State.Status)
	}
	if len(loaded.State.StepLog) != 2 {
		t.Errorf("expected 2 StepLog entries, got %d", len(loaded.State.StepLog))
	}

	entryA := loaded.State.StepLog["stepA"]
	if entryA == nil {
		t.Fatal("missing stepA entry")
	}
	if entryA.Output != "startA" {
		t.Errorf("stepA Output = %v, want startA", entryA.Output)
	}
	if entryA.Duration != 5*time.Millisecond {
		t.Errorf("stepA Duration = %v, want %v", entryA.Duration, 5*time.Millisecond)
	}
	if entryA.Error != nil {
		t.Errorf("stepA Error = %v, want nil", entryA.Error)
	}

	entryB := loaded.State.StepLog["stepB"]
	if entryB == nil {
		t.Fatal("missing stepB entry")
	}
	if entryB.Error == nil || entryB.Error.Error() != "stepB failed" {
		t.Errorf("stepB Error = %v, want 'stepB failed'", entryB.Error)
	}
}

// TestSaveLoadResume simulates a cross-process pause/save/load/resume cycle:
// build a flow, pause mid-execution, save the snapshot to JSON, load it back
// in a "new process", convert the snapshot to a PausePoint, and resume.
func TestSaveLoadResume(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()
	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).To(stepC).Build()

	// Simulate execution up to stepA, then pause.
	exec := &Execution{
		State: ExecutionState{
			Status: StatusPaused,
			StepLog: map[string]*StepLogEntry{
				"stepA": {
					StepName: "stepA",
					Input:    "start",
					Output:   "startA",
					Duration: 1 * time.Millisecond,
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "paused.json")
	p := NewExecutionPersistence(exec, "resume-flow")

	// Save the snapshot.
	snapshot, err := p.SaveJSON(path)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	// Caller populates paused_at info on the snapshot, then re-saves so the
	// "new process" can read both the Execution state and pause metadata.
	snapshot.PausedAt = "stepA"
	snapshot.PausedInput = "start"
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal modified snapshot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("re-write snapshot: %v", err)
	}

	// "New process": load the Execution from disk.
	loadedExec, err := p.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}
	if loadedExec.State.Status != StatusPaused {
		t.Errorf("expected StatusPaused, got %s", loadedExec.State.Status)
	}
	if _, ok := loadedExec.State.StepLog["stepA"]; !ok {
		t.Error("loaded StepLog should contain stepA (history preserved)")
	}

	// Decode the file as a snapshot to obtain a PausePoint for Resume.
	fileData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var loadedSnapshot ExecutionSnapshot
	if err := json.Unmarshal(fileData, &loadedSnapshot); err != nil {
		t.Fatalf("Unmarshal snapshot failed: %v", err)
	}
	pp := loadedSnapshot.AsPausePoint()
	if pp == nil {
		t.Fatal("AsPausePoint returned nil")
	}
	if pp.StepName != "stepA" {
		t.Errorf("expected PausePoint.StepName 'stepA', got %q", pp.StepName)
	}
	if pp.Input != "start" {
		t.Errorf("expected PausePoint.Input 'start', got %v", pp.Input)
	}
	if pp.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}

	// Resume the flow with new input.
	newExec := flow.Resume(context.Background(), pp, "resumed")
	if newExec.State.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", newExec.State.Status)
	}
	expected := "resumedBC"
	if newExec.State.Result != expected {
		t.Errorf("expected result %q, got %v", expected, newExec.State.Result)
	}
	if _, ok := newExec.State.StepLog["stepB"]; !ok {
		t.Error("resumed StepLog should contain stepB")
	}
	if _, ok := newExec.State.StepLog["stepC"]; !ok {
		t.Error("resumed StepLog should contain stepC")
	}
}

// TestErrorSerialization verifies that a custom error in StepLogEntry is
// serialized to its string form and restored via errors.New on load.
func TestErrorSerialization(t *testing.T) {
	customErr := fmt.Errorf("custom error")
	exec := &Execution{
		State: ExecutionState{
			Status: StatusFailed,
			StepLog: map[string]*StepLogEntry{
				"errStep": {
					StepName: "errStep",
					Input:    "in",
					Output:   nil,
					Duration: 3 * time.Millisecond,
					Error:    customErr,
				},
			},
		},
	}

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "err.json")
	yamlPath := filepath.Join(dir, "err.yaml")
	p := NewExecutionPersistence(exec, "err-flow")

	// JSON round-trip
	if _, err := p.SaveJSON(jsonPath); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}
	loadedJSON, err := p.LoadJSON(jsonPath)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}
	entryJSON := loadedJSON.State.StepLog["errStep"]
	if entryJSON == nil {
		t.Fatal("missing errStep entry after JSON round-trip")
	}
	if entryJSON.Error == nil {
		t.Fatal("expected error to be restored after JSON round-trip")
	}
	if entryJSON.Error.Error() != "custom error" {
		t.Errorf("JSON round-trip error = %q, want 'custom error'", entryJSON.Error.Error())
	}

	// YAML round-trip
	if _, err := p.SaveYAML(yamlPath); err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}
	loadedYAML, err := p.LoadYAML(yamlPath)
	if err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}
	entryYAML := loadedYAML.State.StepLog["errStep"]
	if entryYAML == nil {
		t.Fatal("missing errStep entry after YAML round-trip")
	}
	if entryYAML.Error == nil {
		t.Fatal("expected error to be restored after YAML round-trip")
	}
	if entryYAML.Error.Error() != "custom error" {
		t.Errorf("YAML round-trip error = %q, want 'custom error'", entryYAML.Error.Error())
	}

	// Verify the restored error is an errors.new-style error (not the original).
	if !errors.Is(entryJSON.Error, customErr) && entryJSON.Error.Error() == customErr.Error() {
		// Restored error has the same message but is a different instance; that is expected.
		t.Log("JSON: restored error is a new instance with matching message (expected behavior)")
	}
}

// TestAsPausePoint verifies the snapshot-to-PausePoint conversion.
func TestAsPausePoint(t *testing.T) {
	ts := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	s := &ExecutionSnapshot{
		PausedAt:    "stepX",
		PausedInput: "frozen-input",
		CreatedAt:   ts,
	}
	pp := s.AsPausePoint()
	if pp == nil {
		t.Fatal("AsPausePoint returned nil")
	}
	if pp.StepName != "stepX" {
		t.Errorf("expected StepName 'stepX', got %q", pp.StepName)
	}
	if pp.Input != "frozen-input" {
		t.Errorf("expected Input 'frozen-input', got %v", pp.Input)
	}
	if !pp.Timestamp.Equal(ts) {
		t.Errorf("expected Timestamp %v, got %v", ts, pp.Timestamp)
	}
}
