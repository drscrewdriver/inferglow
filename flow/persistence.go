package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ExecutionSnapshot is the serializable representation of an Execution.
// It can be persisted to JSON or YAML and later loaded to resume execution.
//
// 字段分为两组：
//   - 基础字段（v1）：ExecutionID / FlowName / Status / StepLog / Result / PausedAt / PausedInput / CreatedAt
//   - 扩展字段（v2）：RunContext / Interrupts / Intervention / SubFlowFrames / LastSignal /
//     DurableState / ResourceReqs / Compaction / ResumeLedger / OwnerID / LeaseTTL
//
// 扩展字段全部使用 omitempty，旧版加载器可安全忽略。
type ExecutionSnapshot struct {
	SchemaVersion string                           `json:"schema_version" yaml:"schema_version"`
	ExecutionID   string                           `json:"execution_id" yaml:"execution_id"`
	FlowName      string                           `json:"flow_name" yaml:"flow_name"`
	Status        ExecutionStatus                  `json:"status" yaml:"status"`
	StepLog       map[string]*StepLogEntrySnapshot `json:"step_log" yaml:"step_log"`
	Result        any                              `json:"result,omitempty" yaml:"result,omitempty"`
	PausedAt      string                           `json:"paused_at" yaml:"paused_at"`
	PausedInput   any                              `json:"paused_input,omitempty" yaml:"paused_input,omitempty"`
	CreatedAt     time.Time                        `json:"created_at" yaml:"created_at"`

	// 扩展字段（v2）：补充 Execution 状态恢复所需的完整信息。
	RunContext    map[string]any      `json:"run_context,omitempty" yaml:"run_context,omitempty"`
	Interrupts    []InterruptRecord   `json:"interrupts,omitempty" yaml:"interrupts,omitempty"`
	Intervention  *InterventionState  `json:"intervention,omitempty" yaml:"intervention,omitempty"`
	SubFlowFrames []*SubFlowFrame     `json:"sub_flow_frames,omitempty" yaml:"sub_flow_frames,omitempty"`
	LastSignal    *SignalEventRecord  `json:"last_signal,omitempty" yaml:"last_signal,omitempty"`
	DurableState  map[string]any      `json:"durable_system_state,omitempty" yaml:"durable_system_state,omitempty"`
	ResourceReqs  map[string]any      `json:"resource_requirements,omitempty" yaml:"resource_requirements,omitempty"`
	Compaction    *CompactionRecord   `json:"compaction,omitempty" yaml:"compaction,omitempty"`
	ResumeLedger  []ResumeToken       `json:"resume_ledger,omitempty" yaml:"resume_ledger,omitempty"`
	OwnerID       string              `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
	LeaseTTL      int64               `json:"lease_ttl,omitempty" yaml:"lease_ttl,omitempty"`
}

// StepLogEntrySnapshot is the serializable representation of a StepLogEntry.
// The Error field is stored as a string; on load it is restored via errors.New.
type StepLogEntrySnapshot struct {
	StepName   string `json:"step_name" yaml:"step_name"`
	Input      any    `json:"input" yaml:"input"`
	Output     any    `json:"output" yaml:"output"`
	DurationMS int64  `json:"duration_ms" yaml:"duration_ms"`
	Error      string `json:"error,omitempty" yaml:"error,omitempty"`
}

// ============================================================================
// 扩展字段辅助类型
// ============================================================================

// InterruptRecord 记录一次中断事件（信号触发引起的执行暂停）。
type InterruptRecord struct {
	SignalName string    `json:"signal_name" yaml:"signal_name"`
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	HandlerID  string    `json:"handler_id,omitempty" yaml:"handler_id,omitempty"`
}

// InterventionState 记录人工干预的当前状态。
type InterventionState struct {
	HandlerID string     `json:"handler_id" yaml:"handler_id"`
	Message   string     `json:"message,omitempty" yaml:"message,omitempty"`
	Suspended time.Time  `json:"suspended" yaml:"suspended"`
	Resumed   *time.Time `json:"resumed,omitempty" yaml:"resumed,omitempty"`
}

// SignalEventRecord 记录最后一次接收到的信号事件。
// 命名为 SignalEventRecord 以避免与 signal.go 中的 SignalEvent 常量冲突。
type SignalEventRecord struct {
	Name    string    `json:"name" yaml:"name"`
	Payload any       `json:"payload,omitempty" yaml:"payload,omitempty"`
	Time    time.Time `json:"time" yaml:"time"`
}

// CompactionRecord 记录状态压缩操作的元数据。
type CompactionRecord struct {
	Version   int       `json:"version" yaml:"version"`
	Original  int       `json:"original_size" yaml:"original_size"`
	Compact   int       `json:"compact_size" yaml:"compact_size"`
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
}

// ResumeToken 记录一次恢复点的信息。
type ResumeToken struct {
	Checkpoint string    `json:"checkpoint" yaml:"checkpoint"`
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	Data       any       `json:"data,omitempty" yaml:"data,omitempty"`
}

// ExecutionPersistence provides JSON/YAML persistence for an Execution.
// Use NewExecutionPersistence to construct, then call SaveJSON/SaveYAML or
// LoadJSON/LoadYAML to serialize or restore an Execution.
type ExecutionPersistence struct {
	execution *Execution
	flowName  string
}

// NewExecutionPersistence creates an ExecutionPersistence bound to the given
// Execution and flow name. The flowName is stored in snapshots for traceability.
func NewExecutionPersistence(exec *Execution, flowName string) *ExecutionPersistence {
	return &ExecutionPersistence{
		execution: exec,
		flowName:  flowName,
	}
}

// buildSnapshot converts the bound Execution into an ExecutionSnapshot.
// PausedAt and PausedInput are left empty/nil; callers that pause the flow
// are expected to populate these fields on the returned snapshot.
//
// 扩展字段 SubFlowFrames 自动从 GlobalSubFlowRegistry 拷贝；其他扩展字段
// 由调用方在需要时直接设置（例如 snapshot.OwnerID = "..."）。
func (p *ExecutionPersistence) buildSnapshot() *ExecutionSnapshot {
	snapshot := &ExecutionSnapshot{
		SchemaVersion: "v1",
		ExecutionID:   fmt.Sprintf("%d-%s", time.Now().UnixNano(), p.flowName),
		FlowName:      p.flowName,
		Status:        p.execution.State.Status,
		StepLog:       make(map[string]*StepLogEntrySnapshot),
		Result:        p.execution.State.Result,
		PausedAt:      "",
		PausedInput:   nil,
		CreatedAt:     time.Now(),
	}

	for name, entry := range p.execution.State.StepLog {
		if entry == nil {
			continue
		}
		errStr := ""
		if entry.Error != nil {
			errStr = entry.Error.Error()
		}
		snapshot.StepLog[name] = &StepLogEntrySnapshot{
			StepName:   entry.StepName,
			Input:      entry.Input,
			Output:     entry.Output,
			DurationMS: entry.Duration.Milliseconds(),
			Error:      errStr,
		}
	}

	// 自动从全局 SubFlowRegistry 拷贝活跃子流帧
	if reg := GlobalSubFlowRegistry(); reg != nil {
		if frames := reg.List(); len(frames) > 0 {
			// 深拷贝以避免外部修改影响快照
			snapshot.SubFlowFrames = make([]*SubFlowFrame, len(frames))
			for i, f := range frames {
				if f == nil {
					continue
				}
				copyFrame := *f
				snapshot.SubFlowFrames[i] = &copyFrame
			}
		}
	}

	return snapshot
}

// SaveJSON serializes the bound Execution to a JSON file at the given path.
// The file is written with 0644 permissions and 2-space indentation.
// Returns the snapshot that was persisted.
func (p *ExecutionPersistence) SaveJSON(path string) (*ExecutionSnapshot, error) {
	if p.execution == nil {
		return nil, fmt.Errorf("execution is nil")
	}
	snapshot := p.buildSnapshot()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot to json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write snapshot file %s: %w", path, err)
	}
	return snapshot, nil
}

// SaveYAML serializes the bound Execution to a YAML file at the given path.
// The file is written with 0644 permissions.
// Returns the snapshot that was persisted.
func (p *ExecutionPersistence) SaveYAML(path string) (*ExecutionSnapshot, error) {
	if p.execution == nil {
		return nil, fmt.Errorf("execution is nil")
	}
	snapshot := p.buildSnapshot()
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot to yaml: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write snapshot file %s: %w", path, err)
	}
	return snapshot, nil
}

// LoadJSON reads a JSON snapshot from path and reconstructs an Execution.
// The reconstructed Execution.State.StepLog preserves the original history;
// State.Errors is set to nil since errors are restored per-entry in StepLog.
func (p *ExecutionPersistence) LoadJSON(path string) (*Execution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot file %s: %w", path, err)
	}
	var snapshot ExecutionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot from json: %w", err)
	}
	return snapshot.toExecution(), nil
}

// LoadYAML reads a YAML snapshot from path and reconstructs an Execution.
// Field semantics are identical to LoadJSON; only the serialization format differs.
func (p *ExecutionPersistence) LoadYAML(path string) (*Execution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot file %s: %w", path, err)
	}
	var snapshot ExecutionSnapshot
	if err := yaml.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot from yaml: %w", err)
	}
	return snapshot.toExecution(), nil
}

// toExecution rebuilds an Execution from the snapshot, restoring each
// StepLogEntry including its error (via errors.New when the Error string is non-empty).
func (s *ExecutionSnapshot) toExecution() *Execution {
	stepLog := make(map[string]*StepLogEntry, len(s.StepLog))
	for name, entry := range s.StepLog {
		if entry == nil {
			continue
		}
		var err error
		if entry.Error != "" {
			err = errors.New(entry.Error)
		}
		stepLog[name] = &StepLogEntry{
			StepName: entry.StepName,
			Input:    entry.Input,
			Output:   entry.Output,
			Duration: time.Duration(entry.DurationMS) * time.Millisecond,
			Error:    err,
		}
	}

	return &Execution{
		State: ExecutionState{
			Status:  s.Status,
			Result:  s.Result,
			Errors:  nil,
			StepLog: stepLog,
		},
	}
}

// AsPausePoint converts the snapshot into a PausePoint suitable for
// passing to Flow.Resume. StepName is taken from PausedAt, Input from
// PausedInput, and Timestamp from CreatedAt.
func (s *ExecutionSnapshot) AsPausePoint() *PausePoint {
	return &PausePoint{
		StepName:  s.PausedAt,
		Input:     s.PausedInput,
		Timestamp: s.CreatedAt,
	}
}
