package memory

import (
	"testing"
)

func TestMemoryBridge_New(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	bridge := NewMemoryBridge(store)
	if bridge == nil {
		t.Fatal("NewMemoryBridge should return non-nil bridge")
	}

	// Verify default config
	if bridge.config.Enabled != true {
		t.Error("Default config should have Enabled = true")
	}
	if bridge.config.MinStepsForExtraction != 3 {
		t.Errorf("Default MinStepsForExtraction = %d, want 3", bridge.config.MinStepsForExtraction)
	}
}

func TestMemoryBridge_ExtractFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	bridge := NewMemoryBridge(store)

	snapshot := map[string]any{
		"execution_id": "exec-123",
		"flow_name":    "test-flow",
		"status":       "completed",
		"step_log": map[string]any{
			"step1": "result1",
			"step2": "result2",
			"step3": "result3",
		},
		"result": map[string]any{
			"tools_used":       []any{"tool1", "tool2"},
			"duration_seconds": float64(42.5),
			"steps_executed":   3,
		},
	}

	result, err := bridge.ExtractFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("ExtractFromSnapshot failed: %v", err)
	}

	if result == nil {
		t.Fatal("ExtractFromSnapshot should return non-nil result")
	}

	// Should have extracted project insights from the completed flow
	if result.ExtractedCount == 0 {
		t.Error("Expected at least 1 extracted memory from completed flow with tools_used, duration, and steps")
	}

	// Verify extracted memories have content
	for _, m := range result.Memories {
		if m.Body == "" {
			t.Errorf("Extracted memory %q should have non-empty Body", m.Name)
		}
	}
}

func TestMemoryBridge_ExtractFromSnapshot_Disabled(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	bridge := NewMemoryBridge(store)
	bridge.SetEnabled(false)

	snapshot := map[string]any{
		"execution_id": "exec-456",
		"flow_name":    "test-flow",
		"status":       "completed",
		"step_log": map[string]any{
			"step1": "result1",
			"step2": "result2",
			"step3": "result3",
		},
		"result": map[string]any{
			"tools_used": []any{"tool1"},
		},
	}

	result, err := bridge.ExtractFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("ExtractFromSnapshot failed: %v", err)
	}

	if result.ExtractedCount != 0 {
		t.Errorf("Expected 0 extracted memories when bridge is disabled, got %d", result.ExtractedCount)
	}
}

func TestMemoryBridge_ExtractFromSnapshot_InsufficientSteps(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	bridge := NewMemoryBridge(store)

	snapshot := map[string]any{
		"execution_id": "exec-789",
		"flow_name":    "tiny-flow",
		"status":       "completed",
		"step_log": map[string]any{
			"step1": "result1",
		},
		"result": map[string]any{
			"tools_used": []any{"tool1"},
		},
	}

	result, err := bridge.ExtractFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("ExtractFromSnapshot failed: %v", err)
	}

	if result.ExtractedCount != 0 {
		t.Errorf("Expected 0 extracted memories when steps < MinStepsForExtraction, got %d", result.ExtractedCount)
	}
}
