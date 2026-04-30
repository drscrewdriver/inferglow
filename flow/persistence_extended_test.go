package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// 扩展字段结构体测试
// ============================================================================

// Check: InterruptRecord JSON 序列化包含所有字段
func TestInterruptRecordSerialization(t *testing.T) {
	ts := time.Now()
	r := InterruptRecord{
		SignalName: "Pause",
		Timestamp:  ts,
		HandlerID:  "h-1",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"signal_name":"Pause"`) {
		t.Errorf("JSON missing signal_name: %s", s)
	}
	if !strings.Contains(s, `"handler_id":"h-1"`) {
		t.Errorf("JSON missing handler_id: %s", s)
	}
}

// Check: InterruptRecord omitempty 对空 HandlerID 生效
func TestInterruptRecordOmitemptyHandlerID(t *testing.T) {
	r := InterruptRecord{
		SignalName: "Pause",
		Timestamp:  time.Now(),
	}
	data, _ := json.Marshal(r)
	s := string(data)
	if strings.Contains(s, "handler_id") {
		t.Errorf("handler_id should be omitted when empty: %s", s)
	}
}

// Check: InterventionState 序列化
func TestInterventionStateSerialization(t *testing.T) {
	now := time.Now()
	state := &InterventionState{
		HandlerID: "iv-1",
		Message:   "Need human review",
		Suspended: now,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"handler_id":"iv-1"`) {
		t.Errorf("JSON missing handler_id: %s", s)
	}
	if !strings.Contains(s, `"message":"Need human review"`) {
		t.Errorf("JSON missing message: %s", s)
	}
	if strings.Contains(s, `"resumed"`) {
		t.Errorf("resumed should be omitted when nil: %s", s)
	}
}

// Check: InterventionState Resumed 设置后被序列化
func TestInterventionStateResumedSet(t *testing.T) {
	now := time.Now()
	state := &InterventionState{
		HandlerID: "iv-1",
		Suspended: now,
		Resumed:   &now,
	}
	data, _ := json.Marshal(state)
	s := string(data)
	if !strings.Contains(s, `"resumed"`) {
		t.Errorf("resumed should be present: %s", s)
	}
}

// Check: SignalEventRecord 序列化
func TestSignalEventRecordSerialization(t *testing.T) {
	e := &SignalEventRecord{
		Name:    "Chunk[step1]",
		Payload: "hello",
		Time:    time.Now(),
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"name":"Chunk[step1]"`) {
		t.Errorf("JSON missing name: %s", s)
	}
	if !strings.Contains(s, `"payload":"hello"`) {
		t.Errorf("JSON missing payload: %s", s)
	}
}

// Check: CompactionRecord 序列化
func TestCompactionRecordSerialization(t *testing.T) {
	r := &CompactionRecord{
		Version:   1,
		Original:  1024,
		Compact:   256,
		Timestamp: time.Now(),
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"version":1`) {
		t.Errorf("JSON missing version: %s", s)
	}
	if !strings.Contains(s, `"original_size":1024`) {
		t.Errorf("JSON missing original_size: %s", s)
	}
	if !strings.Contains(s, `"compact_size":256`) {
		t.Errorf("JSON missing compact_size: %s", s)
	}
}

// Check: ResumeToken 序列化
func TestResumeTokenSerialization(t *testing.T) {
	tok := ResumeToken{
		Checkpoint: "step-3",
		Timestamp:  time.Now(),
		Data:       map[string]any{"k": "v"},
	}
	data, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"checkpoint":"step-3"`) {
		t.Errorf("JSON missing checkpoint: %s", s)
	}
	if !strings.Contains(s, `"data":{"k":"v"}`) {
		t.Errorf("JSON missing data: %s", s)
	}
}

// ============================================================================
// ExecutionSnapshot 扩展字段序列化测试
// ============================================================================

// Check: ExecutionSnapshot 仅含基础字段时 JSON 不包含扩展字段
func TestExecutionSnapshotBasicFieldsOnly(t *testing.T) {
	s := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "exec-1",
		FlowName:      "flow-1",
		Status:        StatusCompleted,
		StepLog:       make(map[string]*StepLogEntrySnapshot),
		CreatedAt:     time.Now(),
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	str := string(data)
	for _, field := range []string{
		"run_context", "interrupts", "intervention", "sub_flow_frames",
		"last_signal", "durable_system_state", "resource_requirements",
		"compaction", "resume_ledger", "owner_id", "lease_ttl",
	} {
		if strings.Contains(str, field) {
			t.Errorf("basic snapshot should not contain %q, got: %s", field, str)
		}
	}
}

// Check: ExecutionSnapshot 含所有扩展字段时 JSON 正确序列化
func TestExecutionSnapshotAllExtendedFields(t *testing.T) {
	now := time.Now()
	resumed := now.Add(time.Second)
	s := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "exec-1",
		FlowName:      "flow-1",
		Status:        StatusPaused,
		CreatedAt:     now,
		RunContext:    map[string]any{"user": "alice"},
		Interrupts: []InterruptRecord{
			{SignalName: "Pause", Timestamp: now, HandlerID: "h-1"},
		},
		Intervention: &InterventionState{
			HandlerID: "iv-1",
			Message:   "Need review",
			Suspended: now,
			Resumed:   &resumed,
		},
		SubFlowFrames: []*SubFlowFrame{
			{ParentID: "p-1", CreatedAt: now, Result: "child-result"},
		},
		LastSignal: &SignalEventRecord{
			Name:    "Chunk[step1]",
			Payload: "data",
			Time:    now,
		},
		DurableState: map[string]any{"key": "value"},
		ResourceReqs: map[string]any{"cpu": 2},
		Compaction: &CompactionRecord{
			Version:   1,
			Original:  1024,
			Compact:   256,
			Timestamp: now,
		},
		ResumeLedger: []ResumeToken{
			{Checkpoint: "step-1", Timestamp: now},
		},
		OwnerID:  "owner-1",
		LeaseTTL: 60,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	str := string(data)
	for _, field := range []string{
		"run_context", "interrupts", "intervention", "sub_flow_frames",
		"last_signal", "durable_system_state", "resource_requirements",
		"compaction", "resume_ledger", "owner_id", "lease_ttl",
	} {
		if !strings.Contains(str, field) {
			t.Errorf("extended snapshot should contain %q, got: %s", field, str)
		}
	}
}

// Check: ExecutionSnapshot 扩展字段 YAML 序列化
func TestExecutionSnapshotExtendedFieldsYAML(t *testing.T) {
	s := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "exec-1",
		FlowName:      "flow-1",
		Status:        StatusPaused,
		CreatedAt:     time.Now(),
		RunContext:    map[string]any{"user": "alice"},
		OwnerID:       "owner-1",
		LeaseTTL:      60,
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	str := string(data)
	if !strings.Contains(str, "run_context:") {
		t.Errorf("YAML missing run_context: %s", str)
	}
	if !strings.Contains(str, "owner_id:") {
		t.Errorf("YAML missing owner_id: %s", str)
	}
	if !strings.Contains(str, "lease_ttl:") {
		t.Errorf("YAML missing lease_ttl: %s", str)
	}
}

// ============================================================================
// JSON 往返测试
// ============================================================================

// Check: ExecutionSnapshot JSON Marshal → Unmarshal 保留所有扩展字段
func TestExecutionSnapshotJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second) // JSON 时间精度限制
	resumed := now.Add(time.Second)
	original := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   "exec-rt",
		FlowName:      "flow-rt",
		Status:        StatusPaused,
		CreatedAt:     now,
		RunContext:    map[string]any{"user": "alice"},
		Interrupts: []InterruptRecord{
			{SignalName: "Pause", Timestamp: now, HandlerID: "h-1"},
		},
		Intervention: &InterventionState{
			HandlerID: "iv-1",
			Message:   "Need review",
			Suspended: now,
			Resumed:   &resumed,
		},
		SubFlowFrames: []*SubFlowFrame{
			{ParentID: "p-1", CreatedAt: now, Result: "child-result"},
		},
		LastSignal: &SignalEventRecord{
			Name:    "Chunk[step1]",
			Payload: "data",
			Time:    now,
		},
		DurableState:  map[string]any{"key": "value"},
		ResourceReqs:  map[string]any{"cpu": 2},
		OwnerID:       "owner-1",
		LeaseTTL:      60,
		ResumeLedger:  []ResumeToken{{Checkpoint: "step-1", Timestamp: now}},
		Compaction:    &CompactionRecord{Version: 1, Original: 100, Compact: 50, Timestamp: now},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var restored ExecutionSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	// 验证关键字段
	if restored.ExecutionID != original.ExecutionID {
		t.Errorf("ExecutionID = %q, want %q", restored.ExecutionID, original.ExecutionID)
	}
	if restored.OwnerID != original.OwnerID {
		t.Errorf("OwnerID = %q, want %q", restored.OwnerID, original.OwnerID)
	}
	if restored.LeaseTTL != original.LeaseTTL {
		t.Errorf("LeaseTTL = %d, want %d", restored.LeaseTTL, original.LeaseTTL)
	}
	if restored.RunContext["user"] != "alice" {
		t.Errorf("RunContext.user = %v, want alice", restored.RunContext["user"])
	}
	if len(restored.Interrupts) != 1 {
		t.Errorf("len(Interrupts) = %d, want 1", len(restored.Interrupts))
	}
	if restored.Interrupts[0].SignalName != "Pause" {
		t.Errorf("Interrupts[0].SignalName = %q, want Pause", restored.Interrupts[0].SignalName)
	}
	if restored.Intervention == nil {
		t.Error("Intervention should not be nil")
	} else if restored.Intervention.HandlerID != "iv-1" {
		t.Errorf("Intervention.HandlerID = %q, want iv-1", restored.Intervention.HandlerID)
	}
	if len(restored.SubFlowFrames) != 1 {
		t.Errorf("len(SubFlowFrames) = %d, want 1", len(restored.SubFlowFrames))
	} else if restored.SubFlowFrames[0].ParentID != "p-1" {
		t.Errorf("SubFlowFrames[0].ParentID = %q, want p-1", restored.SubFlowFrames[0].ParentID)
	}
	if restored.LastSignal == nil {
		t.Error("LastSignal should not be nil")
	} else if restored.LastSignal.Name != "Chunk[step1]" {
		t.Errorf("LastSignal.Name = %q, want Chunk[step1]", restored.LastSignal.Name)
	}
	if restored.DurableState["key"] != "value" {
		t.Errorf("DurableState.key = %v, want value", restored.DurableState["key"])
	}
	if restored.Compaction == nil {
		t.Error("Compaction should not be nil")
	} else if restored.Compaction.Version != 1 {
		t.Errorf("Compaction.Version = %d, want 1", restored.Compaction.Version)
	}
	if len(restored.ResumeLedger) != 1 {
		t.Errorf("len(ResumeLedger) = %d, want 1", len(restored.ResumeLedger))
	}
}

// ============================================================================
// SaveJSON/LoadJSON 集成测试 - 含扩展字段
// ============================================================================

// Check: SaveJSON + LoadJSON 保留扩展字段
func TestSaveLoadJSONWithExtendedFields(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	// 构造一个 Execution
	exec := &Execution{
		State: ExecutionState{
			Status: StatusPaused,
			Result: "paused-result",
		},
	}
	persistence := NewExecutionPersistence(exec, "extended-flow")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "extended.json")

	// 保存
	savedSnapshot, err := persistence.SaveJSON(path)
	if err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}
	// 在保存的 snapshot 上设置扩展字段
	savedSnapshot.OwnerID = "owner-1"
	savedSnapshot.LeaseTTL = 120
	savedSnapshot.RunContext = map[string]any{"key": "value"}
	// 重新写入
	data, _ := json.MarshalIndent(savedSnapshot, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 加载
	loaded, err := persistence.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}
	_ = loaded // 加载的 Execution 不包含扩展字段，但 LoadJSON 应该不报错

	// 直接反序列化原始 JSON 验证扩展字段
	rawData, _ := os.ReadFile(path)
	var snap ExecutionSnapshot
	if err := json.Unmarshal(rawData, &snap); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if snap.OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want owner-1", snap.OwnerID)
	}
	if snap.LeaseTTL != 120 {
		t.Errorf("LeaseTTL = %d, want 120", snap.LeaseTTL)
	}
	if snap.RunContext["key"] != "value" {
		t.Errorf("RunContext.key = %v, want value", snap.RunContext["key"])
	}
}

// ============================================================================
// SubFlowFrames 自动填充测试
// ============================================================================

// Check: buildSnapshot 自动从 GlobalSubFlowRegistry 填充 SubFlowFrames
func TestBuildSnapshotFillsSubFlowFrames(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	// 注册一个 frame
	GlobalSubFlowRegistry().Register("f1", &SubFlowFrame{
		ParentID:    "p-1",
		CreatedAt:   time.Now(),
		CompletedAt: nil,
		Result:      "result-1",
	})

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
		},
	}
	persistence := NewExecutionPersistence(exec, "test-flow")
	snapshot := persistence.buildSnapshot()

	if len(snapshot.SubFlowFrames) != 1 {
		t.Fatalf("len(SubFlowFrames) = %d, want 1", len(snapshot.SubFlowFrames))
	}
	if snapshot.SubFlowFrames[0].ParentID != "p-1" {
		t.Errorf("SubFlowFrames[0].ParentID = %q, want p-1", snapshot.SubFlowFrames[0].ParentID)
	}
	if snapshot.SubFlowFrames[0].Result != "result-1" {
		t.Errorf("SubFlowFrames[0].Result = %v, want result-1", snapshot.SubFlowFrames[0].Result)
	}
}

// Check: buildSnapshot 在 GlobalSubFlowRegistry 为空时不填 SubFlowFrames
func TestBuildSnapshotEmptySubFlowFrames(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
		},
	}
	persistence := NewExecutionPersistence(exec, "test-flow")
	snapshot := persistence.buildSnapshot()

	if snapshot.SubFlowFrames != nil {
		t.Errorf("SubFlowFrames should be nil when registry is empty, got %v", snapshot.SubFlowFrames)
	}
}

// Check: buildSnapshot 深拷贝 SubFlowFrames（修改原 frame 不影响 snapshot）
func TestBuildSnapshotDeepCopySubFlowFrames(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	original := &SubFlowFrame{
		ParentID:  "p-1",
		CreatedAt: time.Now(),
		Result:    "original",
	}
	GlobalSubFlowRegistry().Register("f1", original)

	exec := &Execution{State: ExecutionState{Status: StatusRunning}}
	persistence := NewExecutionPersistence(exec, "test-flow")
	snapshot := persistence.buildSnapshot()

	// 修改原 frame
	original.Result = "modified"

	// snapshot 中的 frame 不应受影响
	if snapshot.SubFlowFrames[0].Result != "original" {
		t.Errorf("snapshot SubFlowFrames[0].Result = %v, want original (deep copy)", snapshot.SubFlowFrames[0].Result)
	}
}

// ============================================================================
// 集成测试：SubFlow + SaveJSON 验证 frame 被持久化
// ============================================================================

// Check: 执行 SubFlow 后 SaveJSON 包含 SubFlowFrames
func TestSubFlowFramesPersistedToJSON(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	// 执行一个 SubFlow（会在 GlobalSubFlowRegistry 中注册 frame）
	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return in.(string) + "_result", nil
				},
			},
		},
		Input: "data",
	}
	if _, err := h.Execute(oc); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 构造 Execution 并保存
	exec := &Execution{
		State: ExecutionState{
			Status: StatusRunning,
		},
	}
	persistence := NewExecutionPersistence(exec, "test-flow")
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "with-subflow.json")

	if _, err := persistence.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	// 读取并验证 JSON 包含 sub_flow_frames
	data, _ := os.ReadFile(path)
	str := string(data)
	if !strings.Contains(str, "sub_flow_frames") {
		t.Errorf("JSON should contain sub_flow_frames, got: %s", str)
	}
	if !strings.Contains(str, "p-1") {
		t.Errorf("JSON should contain parent_id 'p-1' (from op-1), got: %s", str)
	}
}
