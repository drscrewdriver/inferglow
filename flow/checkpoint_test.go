// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package flow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestSnapshot constructs an ExecutionSnapshot with deterministic fields
// suitable for checkpoint round-trip tests. The ExecutionID is fixed so the
// snapshot can be saved and loaded back by the same key.
func buildTestSnapshot() *ExecutionSnapshot {
	return &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "exec-checkpoint-test",
		FlowName:      "checkpoint-flow",
		Status:        StatusPaused,
		StepLog: map[string]*StepLogEntrySnapshot{
			"stepA": {
				StepName:   "stepA",
				Input:      "start",
				Output:     "startA",
				DurationMS: 5,
				Error:      "",
			},
			"stepB": {
				StepName:   "stepB",
				Input:      "startA",
				Output:     nil,
				DurationMS: 2,
				Error:      "stepB failed",
			},
		},
		Result:    "startA",
		PausedAt:  "stepA",
		CreatedAt: time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
		RunContext: map[string]any{
			"user": "alice",
		},
		OwnerID:  "owner-1",
		LeaseTTL: 60,
	}
}

// TestFileCheckpointStore_SaveLoad verifies that a snapshot saved to a
// FileCheckpointStore can be loaded back with all fields intact.
func TestFileCheckpointStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	original := buildTestSnapshot()
	if err := store.Save(original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the file exists at the expected path.
	path := filepath.Join(dir, original.ExecutionID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint file not created at %s: %v", path, err)
	}

	loaded, err := store.Load(original.ExecutionID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil snapshot")
	}

	// Verify fields match.
	if loaded.ExecutionID != original.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", loaded.ExecutionID, original.ExecutionID)
	}
	if loaded.FlowName != original.FlowName {
		t.Errorf("FlowName = %q, want %q", loaded.FlowName, original.FlowName)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status = %q, want %q", loaded.Status, original.Status)
	}
	if loaded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", loaded.SchemaVersion, original.SchemaVersion)
	}
	if loaded.PausedAt != original.PausedAt {
		t.Errorf("PausedAt = %q, want %q", loaded.PausedAt, original.PausedAt)
	}
	if loaded.Result != original.Result {
		t.Errorf("Result = %v, want %v", loaded.Result, original.Result)
	}
	if loaded.OwnerID != original.OwnerID {
		t.Errorf("OwnerID = %q, want %q", loaded.OwnerID, original.OwnerID)
	}
	if loaded.LeaseTTL != original.LeaseTTL {
		t.Errorf("LeaseTTL = %d, want %d", loaded.LeaseTTL, original.LeaseTTL)
	}
	if !loaded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", loaded.CreatedAt, original.CreatedAt)
	}
	if len(loaded.StepLog) != len(original.StepLog) {
		t.Fatalf("len(StepLog) = %d, want %d", len(loaded.StepLog), len(original.StepLog))
	}
	entryA := loaded.StepLog["stepA"]
	if entryA == nil {
		t.Fatal("loaded StepLog missing stepA")
	}
	if entryA.Output != "startA" {
		t.Errorf("stepA Output = %v, want startA", entryA.Output)
	}
	if entryA.DurationMS != 5 {
		t.Errorf("stepA DurationMS = %d, want 5", entryA.DurationMS)
	}
	entryB := loaded.StepLog["stepB"]
	if entryB == nil {
		t.Fatal("loaded StepLog missing stepB")
	}
	if entryB.Error != "stepB failed" {
		t.Errorf("stepB Error = %q, want 'stepB failed'", entryB.Error)
	}
	if loaded.RunContext["user"] != "alice" {
		t.Errorf("RunContext.user = %v, want alice", loaded.RunContext["user"])
	}
}

// TestFileCheckpointStore_Delete verifies that Delete removes the checkpoint
// file and a subsequent Load returns an error.
func TestFileCheckpointStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	snap := buildTestSnapshot()
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Sanity check: the file exists before delete.
	path := filepath.Join(dir, snap.ExecutionID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint file should exist before delete: %v", err)
	}

	if err := store.Delete(snap.ExecutionID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// The file should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("checkpoint file should be removed after delete, got err=%v", err)
	}

	// Loading the deleted checkpoint should fail.
	if _, err := store.Load(snap.ExecutionID); err == nil {
		t.Fatal("Load after Delete should return an error, got nil")
	}
}

// TestCheckpoint_JSONSerializer verifies the JSONSerializer Marshal/Unmarshal round-trip.
func TestCheckpoint_JSONSerializer(t *testing.T) {
	s := &JSONSerializer{}
	original := buildTestSnapshot()

	data, err := s.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal returned empty data")
	}

	loaded, err := s.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Unmarshal returned nil snapshot")
	}

	if loaded.ExecutionID != original.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", loaded.ExecutionID, original.ExecutionID)
	}
	if loaded.FlowName != original.FlowName {
		t.Errorf("FlowName = %q, want %q", loaded.FlowName, original.FlowName)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status = %q, want %q", loaded.Status, original.Status)
	}
	if loaded.OwnerID != original.OwnerID {
		t.Errorf("OwnerID = %q, want %q", loaded.OwnerID, original.OwnerID)
	}
	if len(loaded.StepLog) != len(original.StepLog) {
		t.Errorf("len(StepLog) = %d, want %d", len(loaded.StepLog), len(original.StepLog))
	}

	// Verify the serialized form uses 2-space indentation.
	// json.MarshalIndent with "" prefix and "  " indent produces lines like
	// `  "key":`, so a 2-space-indented top-level field must be present.
	str := string(data)
	if !strings.Contains(str, "\n  \"execution_id\"") {
		t.Errorf("expected 2-space indented JSON output, got: %s", str)
	}
}

// TestCheckpointManager_AutoSave creates a manager with AutoCheckpoint enabled,
// saves a checkpoint, and verifies the file exists on disk.
func TestCheckpointManager_AutoSave(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	mgr := NewCheckpointManager(CheckpointConfig{
		Store:          store,
		AutoCheckpoint: true,
	})

	if !mgr.ShouldCheckpoint() {
		t.Fatal("ShouldCheckpoint should be true when AutoCheckpoint is enabled and Store is set")
	}

	snap := buildTestSnapshot()
	if err := mgr.SaveCheckpoint(snap); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// The file should exist at {dir}/{ExecutionID}.json.
	path := filepath.Join(dir, snap.ExecutionID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint file not created at %s: %v", path, err)
	}

	// Loading via the store should return the same snapshot.
	loaded, err := store.Load(snap.ExecutionID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.ExecutionID != snap.ExecutionID {
		t.Errorf("loaded ExecutionID = %q, want %q", loaded.ExecutionID, snap.ExecutionID)
	}
}

// TestCheckpointManager_ForceNewRun verifies that with ForceNewRun=true,
// LoadCheckpoint returns (nil, nil) without touching the store, even when a
// checkpoint exists on disk.
func TestCheckpointManager_ForceNewRun(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	// Seed the store with a real checkpoint.
	seed := buildTestSnapshot()
	if err := store.Save(seed); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	mgr := NewCheckpointManager(CheckpointConfig{
		Store:        store,
		CheckPointID: seed.ExecutionID,
		ForceNewRun:  true,
	})

	loaded, err := mgr.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint with ForceNewRun should not error, got: %v", err)
	}
	if loaded != nil {
		t.Fatalf("LoadCheckpoint with ForceNewRun should return nil snapshot, got %v", loaded)
	}
}

// TestCheckpointManager_StateModifier verifies that the StateModifier is
// applied to the snapshot before it is persisted, and that the modified field
// is observable when loading back from the store.
func TestCheckpointManager_StateModifier(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	mgr := NewCheckpointManager(CheckpointConfig{
		Store: store,
		StateModifier: func(s *ExecutionSnapshot) *ExecutionSnapshot {
			s.OwnerID = "modified-owner"
			s.RunContext = map[string]any{"trace": "abc-123"}
			return s
		},
	})

	snap := buildTestSnapshot()
	// Ensure the original OwnerID differs from the modifier's value.
	if snap.OwnerID == "modified-owner" {
		t.Fatal("test setup error: original OwnerID already equals modifier value")
	}

	if err := mgr.SaveCheckpoint(snap); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := store.Load(snap.ExecutionID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.OwnerID != "modified-owner" {
		t.Errorf("OwnerID = %q, want 'modified-owner' (StateModifier should have been applied)", loaded.OwnerID)
	}
	if loaded.RunContext["trace"] != "abc-123" {
		t.Errorf("RunContext.trace = %v, want 'abc-123'", loaded.RunContext["trace"])
	}
}

// TestCheckpoint_ResumeFromSnapshot verifies that ResumeFromSnapshot rebuilds an
// Execution whose state matches the snapshot.
func TestCheckpoint_ResumeFromSnapshot(t *testing.T) {
	snap := buildTestSnapshot()

	exec := snap.ResumeFromSnapshot()
	if exec == nil {
		t.Fatal("ResumeFromSnapshot returned nil Execution")
	}

	if exec.State.Status != snap.Status {
		t.Errorf("Status = %q, want %q", exec.State.Status, snap.Status)
	}
	if exec.State.Result != snap.Result {
		t.Errorf("Result = %v, want %v", exec.State.Result, snap.Result)
	}
	if len(exec.State.StepLog) != len(snap.StepLog) {
		t.Fatalf("len(StepLog) = %d, want %d", len(exec.State.StepLog), len(snap.StepLog))
	}

	entryA := exec.State.StepLog["stepA"]
	if entryA == nil {
		t.Fatal("StepLog missing stepA")
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

	entryB := exec.State.StepLog["stepB"]
	if entryB == nil {
		t.Fatal("StepLog missing stepB")
	}
	if entryB.Error == nil {
		t.Fatal("stepB Error should be restored as non-nil")
	}
	if entryB.Error.Error() != "stepB failed" {
		t.Errorf("stepB Error = %q, want 'stepB failed'", entryB.Error.Error())
	}

	// Errors slice should be nil after resume (consistent with toExecution).
	if exec.State.Errors != nil {
		t.Errorf("Errors = %v, want nil", exec.State.Errors)
	}
}

// TestFlow_CheckpointOptions verifies that each FlowOption sets the
// corresponding field on the built Flow.
func TestFlow_CheckpointOptions(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)
	modifier := func(s *ExecutionSnapshot) *ExecutionSnapshot {
		s.OwnerID = "opt-owner"
		return s
	}
	ser := &JSONSerializer{}

	flow := NewFlow().
		WithOptions(
			WithCheckPointID("cp-id-123"),
			WithWriteToCheckPointID("write-id-456"),
			WithForceNewRun(),
			WithStateModifier(modifier),
			WithAutoCheckpoint(store),
			WithSerializer(ser),
		).Build()

	if flow.checkPointID != "cp-id-123" {
		t.Errorf("checkPointID = %q, want 'cp-id-123'", flow.checkPointID)
	}
	if flow.writeToID != "write-id-456" {
		t.Errorf("writeToID = %q, want 'write-id-456'", flow.writeToID)
	}
	if !flow.forceNewRun {
		t.Error("forceNewRun = false, want true")
	}
	if flow.stateModifier == nil {
		t.Error("stateModifier = nil, want non-nil")
	}
	if !flow.autoCheckpoint {
		t.Error("autoCheckpoint = false, want true")
	}
	if flow.checkpointStore == nil {
		t.Error("checkpointStore = nil, want non-nil (set by WithAutoCheckpoint)")
	}
	if flow.serializer == nil {
		t.Error("serializer = nil, want non-nil (set by WithSerializer)")
	}

	// Verify the StateModifier is actually invokable and applies its change.
	snap := buildTestSnapshot()
	if flow.stateModifier != nil {
		modified := flow.stateModifier(snap)
		if modified.OwnerID != "opt-owner" {
			t.Errorf("StateModifier did not set OwnerID, got %q", modified.OwnerID)
		}
	}
}

// TestFlowPause_NoAutoCheckpoint verifies that without WithAutoCheckpoint,
// Flow.Pause behaves like Execution.Pause and does not persist anything.
func TestFlowPause_NoAutoCheckpoint(t *testing.T) {
	dir := t.TempDir()
	// A store is created but NOT wired into the Flow, so autoCheckpoint stays
	// off and nothing should be persisted.
	_ = NewFileCheckpointStore(dir)
	flow := NewFlow().WithOptions(WithCheckPointID("no-auto")).Build()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}

	pp := flow.Pause(exec, "review")
	if pp.CheckpointID != "" {
		t.Errorf("CheckpointID = %q, want empty (auto-checkpoint off)", pp.CheckpointID)
	}
	// Nothing should have been written to the store's directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no checkpoint files, got %d", len(entries))
	}
}

// TestFlowPause_AutoCheckpoint verifies that with WithAutoCheckpoint, Flow.Pause
// automatically persists a snapshot and the returned PausePoint carries the
// checkpoint ID.
func TestFlowPause_AutoCheckpoint(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	flow := NewFlow().
		WithOptions(WithAutoCheckpoint(store), WithCheckPointID("auto-pause-id")).Build()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}

	pp := flow.Pause(exec, "review")
	if pp == nil {
		t.Fatal("Pause returned nil PausePoint")
	}
	if exec.State.Status != StatusPaused {
		t.Errorf("exec.Status = %q, want paused", exec.State.Status)
	}
	if pp.CheckpointID != "auto-pause-id" {
		t.Errorf("CheckpointID = %q, want 'auto-pause-id'", pp.CheckpointID)
	}

	// The checkpoint file should exist at {dir}/{CheckpointID}.json.
	path := filepath.Join(dir, pp.CheckpointID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint file not created at %s: %v", path, err)
	}

	// Loading via the store returns the snapshot with paused metadata.
	loaded, err := store.Load(pp.CheckpointID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.PausedAt != "stepA" {
		t.Errorf("loaded PausedAt = %q, want stepA", loaded.PausedAt)
	}
	if loaded.Status != StatusPaused {
		t.Errorf("loaded Status = %q, want paused", loaded.Status)
	}
	if loaded.FlowName != "" {
		t.Errorf("loaded FlowName = %q, want empty (Flow has no name)", loaded.FlowName)
	}
}

// TestCheckpoint_FlowResumeFromSnapshot verifies that Flow.ResumeFromSnapshot
// restores StepLog history and continues execution from the step after PausedAt.
func TestCheckpoint_FlowResumeFromSnapshot(t *testing.T) {
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

	// Snapshot paused after stepA executed: stepA produced "startA".
	snapshot := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "resume-snap",
		FlowName:      "resume-flow",
		Status:        StatusPaused,
		StepLog: map[string]*StepLogEntrySnapshot{
			"stepA": {
				StepName:   "stepA",
				Input:      "start",
				Output:     "startA",
				DurationMS: 1,
			},
		},
		Result:      "startA",
		PausedAt:    "stepA",
		PausedInput: "start",
		CreatedAt:   time.Now(),
	}

	exec := flow.ResumeFromSnapshot(snapshot)
	if exec == nil {
		t.Fatal("ResumeFromSnapshot returned nil")
	}
	if exec.State.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", exec.State.Status)
	}
	// stepA output "startA" flows into stepB ("startAB") then stepC ("startABC").
	if exec.State.Result != "startABC" {
		t.Errorf("Result = %v, want startABC", exec.State.Result)
	}
	// Restored history (stepA) plus newly executed steps (stepB, stepC).
	for _, name := range []string{"stepA", "stepB", "stepC"} {
		if _, ok := exec.State.StepLog[name]; !ok {
			t.Errorf("StepLog missing %s", name)
		}
	}
	// StepExecLog should reflect history-first ordering.
	if len(exec.State.StepExecLog) < 3 {
		t.Errorf("StepExecLog len = %d, want >= 3", len(exec.State.StepExecLog))
	}
	if len(exec.State.StepExecLog) > 0 && exec.State.StepExecLog[0] != "stepA" {
		t.Errorf("StepExecLog[0] = %q, want stepA (history first)", exec.State.StepExecLog[0])
	}
}

// TestCheckpoint_FlowResumeFromSnapshot_Nil verifies graceful handling of a nil snapshot.
func TestCheckpoint_FlowResumeFromSnapshot_Nil(t *testing.T) {
	flow := NewFlow().Build()
	exec := flow.ResumeFromSnapshot(nil)
	if exec == nil {
		t.Fatal("ResumeFromSnapshot(nil) returned nil")
	}
	if exec.State.Status != StatusFailed {
		t.Errorf("Status = %q, want failed", exec.State.Status)
	}
	if len(exec.State.Errors) == 0 {
		t.Error("expected at least one error recorded for nil snapshot")
	}
}

// TestCheckpoint_FlowCrashRecovery simulates a crash-recovery cycle at the Flow level:
//  1. Run a Flow with auto-checkpoint; pause mid-execution to persist a
//     checkpoint under a fixed ID.
//  2. "Crash" — discard all in-memory references.
//  3. Build a fresh Flow and load the checkpoint by ID.
//  4. Resume execution from the snapshot and verify the result + history.
func TestCheckpoint_FlowCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()
	stepC := NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "C", nil
	}).Build()

	origFlow := NewFlow().AddStep(stepA).To(stepB).To(stepC).
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("crash-exec"),
		).Build()

	// Simulate partial execution up to stepA, then crash-pause: auto-saves
	// under "crash-exec".
	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}
	pp := origFlow.Pause(exec, "crash")
	if pp.CheckpointID != "crash-exec" {
		t.Fatalf("CheckpointID = %q, want crash-exec", pp.CheckpointID)
	}

	// Verify the checkpoint was persisted under the configured ID.
	path := filepath.Join(dir, "crash-exec.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint not persisted at %s: %v", path, err)
	}

	// "Crash": discard all in-memory state.
	exec = nil
	origFlow = nil

	// New flow (rebuilt) loads the checkpoint and resumes from it. The store
	// must be bound (via WithAutoCheckpoint) so LoadCheckpoint can reach it;
	// autoCheckpoint being true is harmless here since we never Pause.
	newFlow := NewFlow().AddStep(stepA).To(stepB).To(stepC).
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("crash-exec"),
		).Build()

	recovered, err := newFlow.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if recovered == nil {
		t.Fatal("LoadCheckpoint returned nil snapshot")
	}
	if recovered.PausedAt != "stepA" {
		t.Errorf("recovered PausedAt = %q, want stepA", recovered.PausedAt)
	}
	if recovered.ExecutionID != "crash-exec" {
		t.Errorf("recovered ExecutionID = %q, want crash-exec", recovered.ExecutionID)
	}

	resumed := newFlow.ResumeFromSnapshot(recovered)
	if resumed.State.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", resumed.State.Status)
	}
	if resumed.State.Result != "startABC" {
		t.Errorf("Result = %v, want startABC", resumed.State.Result)
	}
	if _, ok := resumed.State.StepLog["stepA"]; !ok {
		t.Error("resumed StepLog should contain restored stepA")
	}
	if _, ok := resumed.State.StepLog["stepC"]; !ok {
		t.Error("resumed StepLog should contain newly executed stepC")
	}
}

// TestCheckpoint_FlowWithCheckPointID verifies that a snapshot is saved and
// loaded under the ID configured via WithCheckPointID.
func TestCheckpoint_FlowWithCheckPointID(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	flow := NewFlow().
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("fixed-cp-id"),
		).Build()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"s1": {StepName: "s1", Input: "in", Output: "out1", Duration: time.Millisecond},
			},
			StepExecLog: []string{"s1"},
			Result:      "out1",
		},
	}
	pp := flow.Pause(exec, "review")
	if pp.CheckpointID != "fixed-cp-id" {
		t.Errorf("CheckpointID = %q, want fixed-cp-id", pp.CheckpointID)
	}
	// File written under the fixed ID.
	path := filepath.Join(dir, "fixed-cp-id.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("checkpoint not at fixed ID path: %v", err)
	}
	// No file under a random ExecutionID should exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 checkpoint file, got %d", len(entries))
	}
	// LoadCheckpoint reads back the same ID.
	loaded, err := flow.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}
	if loaded.ExecutionID != "fixed-cp-id" {
		t.Errorf("loaded ExecutionID = %q, want fixed-cp-id", loaded.ExecutionID)
	}
}

// TestCheckpoint_FlowWithWriteToCheckPointID verifies versioned writes: a
// snapshot is loaded from the old CheckPointID and a new checkpoint is written
// to the WriteToID, leaving the old checkpoint untouched.
func TestCheckpoint_FlowWithWriteToCheckPointID(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	// Seed a checkpoint under the OLD id.
	seed := buildTestSnapshot()
	seed.ExecutionID = "old-id"
	seed.PausedAt = "stepA"
	if err := store.Save(seed); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	// Flow reads from old, writes to new.
	flow := NewFlow().
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("old-id"),
			WithWriteToCheckPointID("new-id"),
		).Build()

	// Load from old id.
	loaded, err := flow.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}
	if loaded.ExecutionID != "old-id" {
		t.Errorf("loaded ExecutionID = %q, want old-id", loaded.ExecutionID)
	}

	// Auto-save writes to new id.
	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}
	pp := flow.Pause(exec, "version")
	if pp.CheckpointID != "new-id" {
		t.Errorf("CheckpointID = %q, want new-id", pp.CheckpointID)
	}
	// New file exists, old file still exists (untouched).
	if _, err := os.Stat(filepath.Join(dir, "new-id.json")); err != nil {
		t.Fatalf("new checkpoint file not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-id.json")); err != nil {
		t.Fatalf("old checkpoint file should still exist: %v", err)
	}
	// Loading the new id returns the versioned snapshot.
	v2, err := store.Load("new-id")
	if err != nil {
		t.Fatalf("Load new-id failed: %v", err)
	}
	if v2.ExecutionID != "new-id" {
		t.Errorf("v2 ExecutionID = %q, want new-id", v2.ExecutionID)
	}
	if v2.PausedAt != "stepA" {
		t.Errorf("v2 PausedAt = %q, want stepA", v2.PausedAt)
	}
}

// TestCheckpoint_FlowWithForceNewRun verifies that WithForceNewRun causes
// LoadCheckpoint to ignore an existing checkpoint and return (nil, nil), so the
// Flow executes from scratch.
func TestCheckpoint_FlowWithForceNewRun(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	// Seed a checkpoint that should be IGNORED.
	seed := buildTestSnapshot()
	seed.ExecutionID = "existing"
	if err := store.Save(seed); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "A", nil
	}).Build()
	stepB := NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "B", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).To(stepB).
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("existing"),
			WithForceNewRun(),
		).Build()

	// ForceNewRun => LoadCheckpoint returns (nil, nil) even though a
	// checkpoint exists.
	loaded, err := flow.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint with ForceNewRun should not error: %v", err)
	}
	if loaded != nil {
		t.Fatalf("LoadCheckpoint with ForceNewRun should return nil, got %v", loaded)
	}

	// Execute fresh from scratch.
	exec := flow.Execute(context.Background(), "start")
	if exec.State.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", exec.State.Status)
	}
	if exec.State.Result != "startAB" {
		t.Errorf("Result = %v, want startAB", exec.State.Result)
	}
}

// TestCheckpoint_FlowWithStateModifier verifies that the StateModifier is
// applied to a snapshot before it is persisted by Flow.Pause, and the modified
// field is observable when loading back.
func TestCheckpoint_FlowWithStateModifier(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	flow := NewFlow().
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("mod-cp"),
			WithStateModifier(func(s *ExecutionSnapshot) *ExecutionSnapshot {
				s.OwnerID = "modified-by-flow"
				s.RunContext = map[string]any{"trace": "flow-123"}
				return s
			}),
		).Build()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}
	pp := flow.Pause(exec, "review")
	if pp.CheckpointID == "" {
		t.Fatal("CheckpointID should be set")
	}
	loaded, err := store.Load(pp.CheckpointID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.OwnerID != "modified-by-flow" {
		t.Errorf("OwnerID = %q, want modified-by-flow", loaded.OwnerID)
	}
	if loaded.RunContext["trace"] != "flow-123" {
		t.Errorf("RunContext.trace = %v, want flow-123", loaded.RunContext["trace"])
	}
}

// recordingSerializer wraps a Serializer and records whether Marshal was
// invoked, so tests can verify the Flow's configured Serializer is used.
type recordingSerializer struct {
	inner  Serializer
	called bool
}

func (r *recordingSerializer) Marshal(s *ExecutionSnapshot) ([]byte, error) {
	r.called = true
	return r.inner.Marshal(s)
}

func (r *recordingSerializer) Unmarshal(data []byte) (*ExecutionSnapshot, error) {
	return r.inner.Unmarshal(data)
}

// TestCheckpoint_FlowWithSerializer verifies that WithSerializer injects a
// custom Serializer into the save path of a *FileCheckpointStore.
func TestCheckpoint_FlowWithSerializer(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)
	rec := &recordingSerializer{inner: &JSONSerializer{}}

	flow := NewFlow().
		WithOptions(
			WithAutoCheckpoint(store),
			WithCheckPointID("ser-cp"),
			WithSerializer(rec),
		).Build()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
			StepLog: map[string]*StepLogEntry{
				"stepA": {StepName: "stepA", Input: "start", Output: "startA", Duration: time.Millisecond},
			},
			StepExecLog: []string{"stepA"},
			Result:      "startA",
		},
	}
	pp := flow.Pause(exec, "review")
	if !rec.called {
		t.Fatal("custom Serializer.Marshal was not invoked during auto-checkpoint")
	}
	if pp.CheckpointID != "ser-cp" {
		t.Errorf("CheckpointID = %q, want ser-cp", pp.CheckpointID)
	}
	// The file should still be valid JSON (recorder delegates to JSONSerializer),
	// so the original store (JSONSerializer) can load it back.
	loaded, err := store.Load(pp.CheckpointID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Status != StatusPaused {
		t.Errorf("loaded Status = %q, want paused", loaded.Status)
	}
	if loaded.PausedAt != "stepA" {
		t.Errorf("loaded PausedAt = %q, want stepA", loaded.PausedAt)
	}
	// The original store's serializer must NOT have been mutated.
	if store.serializer == rec {
		t.Error("WithSerializer mutated the original store's serializer; it should only affect a copy")
	}
}

// TestCheckpoint_CrashRecovery simulates a crash-recovery cycle:
//  1. Build an Execution and persist a checkpoint snapshot to the store.
//  2. "Crash" — discard all in-memory references to the Execution/snapshot.
//  3. Load the checkpoint back from the store by its ExecutionID.
//  4. Call ResumeFromSnapshot to rebuild an Execution.
//  5. Verify the rebuilt Execution matches the original state.
func TestCheckpoint_CrashRecovery(t *testing.T) {
	dir := t.TempDir()
	store := NewFileCheckpointStore(dir)

	// Step 1: build an Execution with realistic state.
	originalExec := &Execution{
		State: ExecutionState{
			Status: StatusPaused,
			Result: "paused-result",
			StepLog: map[string]*StepLogEntry{
				"stepA": {
					StepName: "stepA",
					Input:    "start",
					Output:   "startA",
					Duration: 7 * time.Millisecond,
				},
				"stepB": {
					StepName: "stepB",
					Input:    "startA",
					Output:   "startAB",
					Duration: 3 * time.Millisecond,
					Error:    nil,
				},
			},
		},
	}
	persistence := NewExecutionPersistence(originalExec, "crash-flow")
	snapshot := persistence.buildSnapshot()
	// Populate pause metadata so it survives the round-trip.
	snapshot.PausedAt = "stepB"
	snapshot.PausedInput = "startA"

	// Persist the checkpoint via the store.
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("Save checkpoint failed: %v", err)
	}
	checkpointID := snapshot.ExecutionID

	// Step 2: "crash" — drop all in-memory references.
	originalExec = nil
	persistence = nil
	snapshot = nil

	// Step 3: load the checkpoint back from the store.
	recovered, err := store.Load(checkpointID)
	if err != nil {
		t.Fatalf("Load checkpoint after crash failed: %v", err)
	}
	if recovered == nil {
		t.Fatal("Load returned nil snapshot after crash")
	}
	if recovered.ExecutionID != checkpointID {
		t.Errorf("recovered ExecutionID = %q, want %q", recovered.ExecutionID, checkpointID)
	}
	if recovered.FlowName != "crash-flow" {
		t.Errorf("recovered FlowName = %q, want 'crash-flow'", recovered.FlowName)
	}
	if recovered.Status != StatusPaused {
		t.Errorf("recovered Status = %q, want %q", recovered.Status, StatusPaused)
	}
	if recovered.PausedAt != "stepB" {
		t.Errorf("recovered PausedAt = %q, want 'stepB'", recovered.PausedAt)
	}

	// Step 4: resume from the snapshot.
	resumedExec := recovered.ResumeFromSnapshot()
	if resumedExec == nil {
		t.Fatal("ResumeFromSnapshot returned nil Execution")
	}

	// Step 5: verify the rebuilt Execution matches the original state.
	if resumedExec.State.Status != StatusPaused {
		t.Errorf("resumed Status = %q, want %q", resumedExec.State.Status, StatusPaused)
	}
	if resumedExec.State.Result != "paused-result" {
		t.Errorf("resumed Result = %v, want 'paused-result'", resumedExec.State.Result)
	}
	if len(resumedExec.State.StepLog) != 2 {
		t.Fatalf("resumed len(StepLog) = %d, want 2", len(resumedExec.State.StepLog))
	}
	entryA := resumedExec.State.StepLog["stepA"]
	if entryA == nil {
		t.Fatal("resumed StepLog missing stepA")
	}
	if entryA.Output != "startA" {
		t.Errorf("resumed stepA Output = %v, want startA", entryA.Output)
	}
	if entryA.Duration != 7*time.Millisecond {
		t.Errorf("resumed stepA Duration = %v, want %v", entryA.Duration, 7*time.Millisecond)
	}
	entryB := resumedExec.State.StepLog["stepB"]
	if entryB == nil {
		t.Fatal("resumed StepLog missing stepB")
	}
	if entryB.Output != "startAB" {
		t.Errorf("resumed stepB Output = %v, want startAB", entryB.Output)
	}
}
