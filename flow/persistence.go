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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	RunContext    map[string]any     `json:"run_context,omitempty" yaml:"run_context,omitempty"`
	Interrupts    []InterruptRecord  `json:"interrupts,omitempty" yaml:"interrupts,omitempty"`
	Intervention  *InterventionState `json:"intervention,omitempty" yaml:"intervention,omitempty"`
	SubFlowFrames []*SubFlowFrame    `json:"sub_flow_frames,omitempty" yaml:"sub_flow_frames,omitempty"`
	LastSignal    *SignalEventRecord `json:"last_signal,omitempty" yaml:"last_signal,omitempty"`
	DurableState  map[string]any     `json:"durable_system_state,omitempty" yaml:"durable_system_state,omitempty"`
	ResourceReqs  map[string]any     `json:"resource_requirements,omitempty" yaml:"resource_requirements,omitempty"`
	Compaction    *CompactionRecord  `json:"compaction,omitempty" yaml:"compaction,omitempty"`
	ResumeLedger  []ResumeToken      `json:"resume_ledger,omitempty" yaml:"resume_ledger,omitempty"`
	OwnerID       string             `json:"owner_id,omitempty" yaml:"owner_id,omitempty"`
	LeaseTTL      int64              `json:"lease_ttl,omitempty" yaml:"lease_ttl,omitempty"`
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
		ExecutionID:   generateExecutionID(p.flowName),
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
			// BUG-20 / F-MEDIUM-6: 深拷贝以避免外部修改影响 snapshot
			// （State/RuntimeData/FlowData 是引用类型，浅拷贝会共享引用）
			snapshot.SubFlowFrames = make([]*SubFlowFrame, len(frames))
			for i, f := range frames {
				if f == nil {
					continue
				}
				snapshot.SubFlowFrames[i] = f.DeepCopy()
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

// generateExecutionID 生成全局唯一的 ExecutionID。
//
// BUG-21 / F-MEDIUM-10: 原实现使用 time.Now().UnixNano()，在 Windows 上
// time.Now() 分辨率约 1ms，高频调用必然冲突。
//
// 修复：使用 UnixNano + crypto/rand 生成的 8 字节随机后缀，保留 flowName
// 后缀以保证可读性。最终格式："<unixnano>-<rand16hex>-<flowName>"。
//
// 即使两次调用落在同一纳秒，crypto/rand 提供的 64-bit 随机后缀也能保证唯一性。
// 失败回退到 time.Now().UnixNano()（与旧行为一致），但记录 fallback 标记。
func generateExecutionID(flowName string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 极罕见的回退：crypto/rand 不可用时退化为旧行为
		return fmt.Sprintf("%d-%s", time.Now().UnixNano(), flowName)
	}
	return fmt.Sprintf("%d-%s-%s", time.Now().UnixNano(), hex.EncodeToString(buf[:]), flowName)
}

// ============================================================================
// 自动 Checkpoint 持久化
// ============================================================================

// CheckpointStore abstracts the storage backend for execution checkpoints.
// Implementations may persist snapshots to the local filesystem, a remote
// key-value store, a database, etc.
type CheckpointStore interface {
	Save(snapshot *ExecutionSnapshot) error
	Load(id string) (*ExecutionSnapshot, error)
	Delete(id string) error
}

// Serializer abstracts the (de)serialization format used by a CheckpointStore.
// The default implementation is JSONSerializer.
type Serializer interface {
	Marshal(s *ExecutionSnapshot) ([]byte, error)
	Unmarshal(data []byte) (*ExecutionSnapshot, error)
}

// JSONSerializer is the default Serializer implementation. It encodes a
// snapshot as pretty-printed JSON with a 2-space indent.
type JSONSerializer struct{}

// Marshal encodes the snapshot using json.MarshalIndent with a 2-space indent.
func (j *JSONSerializer) Marshal(s *ExecutionSnapshot) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}
	return json.MarshalIndent(s, "", "  ")
}

// Unmarshal decodes JSON data into an ExecutionSnapshot.
func (j *JSONSerializer) Unmarshal(data []byte) (*ExecutionSnapshot, error) {
	var snap ExecutionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// FileCheckpointStore persists checkpoints as individual files in a directory.
// Each snapshot is written to `{dir}/{ExecutionID}.json` and retrieved by the
// same ExecutionID. The serialization format is controlled by the configured
// Serializer (JSONSerializer by default).
type FileCheckpointStore struct {
	dir        string
	serializer Serializer
}

// NewFileCheckpointStore creates a FileCheckpointStore rooted at dir.
// The store uses a JSONSerializer by default. The directory is created on
// the first Save call if it does not already exist.
func NewFileCheckpointStore(dir string) *FileCheckpointStore {
	return &FileCheckpointStore{
		dir:        dir,
		serializer: &JSONSerializer{},
	}
}

// Save writes the snapshot to `{dir}/{snapshot.ExecutionID}.json`.
// snapshot.ExecutionID serves as the key used to later Load or Delete the
// checkpoint. The directory is created (with 0755 permissions) if missing.
func (f *FileCheckpointStore) Save(snapshot *ExecutionSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if snapshot.ExecutionID == "" {
		return fmt.Errorf("snapshot.ExecutionID is empty")
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint dir %s: %w", f.dir, err)
	}
	data, err := f.serializer.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	path := filepath.Join(f.dir, snapshot.ExecutionID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write checkpoint file %s: %w", path, err)
	}
	return nil
}

// Load reads the checkpoint identified by id from `{dir}/{id}.json`.
func (f *FileCheckpointStore) Load(id string) (*ExecutionSnapshot, error) {
	if id == "" {
		return nil, fmt.Errorf("checkpoint id is empty")
	}
	path := filepath.Join(f.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint file %s: %w", path, err)
	}
	return f.serializer.Unmarshal(data)
}

// Delete removes the checkpoint file at `{dir}/{id}.json`.
func (f *FileCheckpointStore) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("checkpoint id is empty")
	}
	path := filepath.Join(f.dir, id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete checkpoint file %s: %w", path, err)
	}
	return nil
}

// CheckpointConfig configures checkpoint behavior for a CheckpointManager.
type CheckpointConfig struct {
	Store          CheckpointStore
	AutoCheckpoint bool
	Serializer     Serializer
	CheckPointID   string
	WriteToID      string
	ForceNewRun    bool
	StateModifier  func(*ExecutionSnapshot) *ExecutionSnapshot
}

// CheckpointManager coordinates checkpoint save/load operations against a
// CheckpointStore, applying any configured StateModifier and honoring the
// ForceNewRun / AutoCheckpoint flags.
type CheckpointManager struct {
	config     CheckpointConfig
	serializer Serializer
}

// NewCheckpointManager creates a CheckpointManager bound to the given config.
// When config.Serializer is nil a JSONSerializer is used as the default.
func NewCheckpointManager(config CheckpointConfig) *CheckpointManager {
	ser := config.Serializer
	if ser == nil {
		ser = &JSONSerializer{}
	}
	return &CheckpointManager{
		config:     config,
		serializer: ser,
	}
}

// SaveCheckpoint applies the StateModifier (if set) to the snapshot and then
// persists it via the configured Store.
func (m *CheckpointManager) SaveCheckpoint(snapshot *ExecutionSnapshot) error {
	if m.config.Store == nil {
		return fmt.Errorf("checkpoint store is nil")
	}
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if m.config.StateModifier != nil {
		snapshot = m.config.StateModifier(snapshot)
	}
	return m.config.Store.Save(snapshot)
}

// LoadCheckpoint retrieves the checkpoint identified by CheckPointID from the
// configured Store. When ForceNewRun is true, no load is performed and the
// method returns (nil, nil) so callers can start a fresh run.
func (m *CheckpointManager) LoadCheckpoint() (*ExecutionSnapshot, error) {
	if m.config.ForceNewRun {
		return nil, nil
	}
	if m.config.Store == nil {
		return nil, fmt.Errorf("checkpoint store is nil")
	}
	return m.config.Store.Load(m.config.CheckPointID)
}

// ShouldCheckpoint reports whether automatic checkpointing is active, i.e.
// AutoCheckpoint is enabled and a Store has been configured.
func (m *CheckpointManager) ShouldCheckpoint() bool {
	return m.config.AutoCheckpoint && m.config.Store != nil
}

// WithAutoCheckpoint returns a FlowOption that enables automatic checkpointing
// and binds the given store. When active, Flow.Pause persists a snapshot to the
// store on every pause.
func WithAutoCheckpoint(store CheckpointStore) FlowOption {
	return func(f *Flow) {
		f.autoCheckpoint = true
		f.checkpointStore = store
	}
}

// WithCheckPointID returns a FlowOption that sets the checkpoint ID used to
// load an existing checkpoint (and to save, when no WriteToID is configured).
func WithCheckPointID(id string) FlowOption {
	return func(f *Flow) {
		f.checkPointID = id
	}
}

// WithWriteToCheckPointID returns a FlowOption that sets the ID under which new
// checkpoints are written. This enables versioned writes: load from the old
// CheckPointID, write to the new WriteToID.
func WithWriteToCheckPointID(id string) FlowOption {
	return func(f *Flow) {
		f.writeToID = id
	}
}

// WithForceNewRun returns a FlowOption that marks the run as fresh.
// Flow.LoadCheckpoint returns (nil, nil) so callers execute from scratch,
// ignoring any existing checkpoint.
func WithForceNewRun() FlowOption {
	return func(f *Flow) {
		f.forceNewRun = true
	}
}

// WithStateModifier returns a FlowOption that installs a StateModifier invoked
// on every snapshot before it is persisted by the Flow (via Flow.Pause or
// Flow.SaveCheckpoint).
func WithStateModifier(fn func(*ExecutionSnapshot) *ExecutionSnapshot) FlowOption {
	return func(f *Flow) {
		f.stateModifier = fn
	}
}

// WithSerializer returns a FlowOption that sets the Serializer used when the
// Flow's checkpoint store is a *FileCheckpointStore. When set, save/load
// operations go through a shallow copy of the store with this serializer
// injected, leaving the original store unchanged. When the store is not a
// *FileCheckpointStore the option has no behavioral effect but is still
// recorded on the Flow.
func WithSerializer(s Serializer) FlowOption {
	return func(f *Flow) {
		f.serializer = s
	}
}

// ResumeFromSnapshot rebuilds an Execution from the snapshot by delegating to
// toExecution. The returned Execution has its StepLog, Status and Result
// restored; callers may resume the owning Flow from the snapshot's PausedAt.
func (s *ExecutionSnapshot) ResumeFromSnapshot() *Execution {
	return s.toExecution()
}

// ============================================================================
// Flow 级别的 Checkpoint 操作
// ============================================================================

// effectiveStore returns the checkpoint store to use for save/load, injecting
// the Flow's serializer when one is configured and the store is a
// *FileCheckpointStore. The original store is left unchanged; a shallow copy
// carries the injected serializer.
func (f *Flow) effectiveStore() CheckpointStore {
	if f.serializer != nil {
		if s, ok := f.checkpointStore.(*FileCheckpointStore); ok {
			cp := *s
			cp.serializer = f.serializer
			return &cp
		}
	}
	return f.checkpointStore
}

// SaveCheckpoint persists snapshot via the Flow's configured store. The
// StateModifier (if any) is applied first. The snapshot is written under
// WriteToID when set, otherwise under CheckPointID when set, otherwise under
// snapshot.ExecutionID. snapshot.ExecutionID is mutated to reflect the actual
// save key so callers can record it for later load.
func (f *Flow) SaveCheckpoint(snapshot *ExecutionSnapshot) error {
	if f.checkpointStore == nil {
		return fmt.Errorf("checkpoint store is nil")
	}
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if f.stateModifier != nil {
		snapshot = f.stateModifier(snapshot)
	}
	saveID := f.writeToID
	if saveID == "" {
		saveID = f.checkPointID
	}
	if saveID != "" {
		snapshot.ExecutionID = saveID
	}
	return f.effectiveStore().Save(snapshot)
}

// LoadCheckpoint retrieves the snapshot identified by CheckPointID from the
// configured store. Returns (nil, nil) when ForceNewRun is set or when no
// CheckPointID is configured, so callers can fall back to a fresh run.
func (f *Flow) LoadCheckpoint() (*ExecutionSnapshot, error) {
	if f.forceNewRun {
		return nil, nil
	}
	if f.checkpointStore == nil {
		return nil, fmt.Errorf("checkpoint store is nil")
	}
	if f.checkPointID == "" {
		return nil, nil
	}
	return f.effectiveStore().Load(f.checkPointID)
}

// Pause transitions exec to StatusPaused via Execution.Pause and, when
// auto-checkpointing is active, persists the resulting snapshot to the
// configured store. The returned PausePoint carries the checkpoint ID under
// CheckpointID so callers can later resume from the persisted state. If the
// auto-save fails the error is recorded on exec.State.Errors and CheckpointID
// is left empty.
//
// This is the Flow-level counterpart of Execution.Pause: it composes the
// low-level pause with checkpoint persistence driven by FlowOption
// configuration (WithAutoCheckpoint / WithCheckPointID / WithWriteToCheckPointID /
// WithStateModifier / WithSerializer).
func (f *Flow) Pause(exec *Execution, reason string) *PausePoint {
	pp := exec.Pause(reason)
	if f.autoCheckpoint && f.checkpointStore != nil {
		snapshot := NewExecutionPersistence(exec, "").buildSnapshot()
		snapshot.PausedAt = pp.StepName
		snapshot.PausedInput = pp.Input
		if err := f.SaveCheckpoint(snapshot); err != nil {
			exec.State.Errors = append(exec.State.Errors, fmt.Errorf("auto-checkpoint save: %w", err))
		} else {
			pp.CheckpointID = snapshot.ExecutionID
		}
	}
	return pp
}

// ResumeFromSnapshot rebuilds an Execution from snapshot (restoring StepLog /
// Result / Status history) and then continues execution from the step that
// follows the snapshot's PausedAt. The returned Execution contains both the
// restored history and the newly executed steps.
//
// The resume input fed to the next step is, in priority order: the paused
// step's recorded Output (what would flow forward), the snapshot's PausedInput,
// or the snapshot's Result. Execution uses context.Background; supply a
// cancellable context via Flow.Resume when finer control is needed.
func (f *Flow) ResumeFromSnapshot(snapshot *ExecutionSnapshot) *Execution {
	if snapshot == nil {
		return &Execution{
			State: ExecutionState{
				Status:  StatusFailed,
				Errors:  []error{fmt.Errorf("resume from nil snapshot")},
				StepLog: make(map[string]*StepLogEntry),
			},
		}
	}
	history := snapshot.toExecution()
	pp := snapshot.AsPausePoint()

	resumeInput := snapshot.PausedInput
	if entry := snapshot.StepLog[snapshot.PausedAt]; entry != nil && entry.Output != nil {
		resumeInput = entry.Output
	}
	if resumeInput == nil {
		resumeInput = snapshot.Result
	}

	resumed := f.Resume(context.Background(), pp, resumeInput)

	// Merge restored history without overwriting freshly executed entries.
	for name, entry := range history.State.StepLog {
		if _, exists := resumed.State.StepLog[name]; !exists {
			resumed.State.StepLog[name] = entry
		}
	}
	// Prepend historical execution order before the newly executed steps so
	// callers observing StepExecLog see a coherent timeline. toExecution cannot
	// derive order from the StepLog map alone, so walk the Flow graph.
	resumed.State.StepExecLog = append(f.historyStepOrder(snapshot), resumed.State.StepExecLog...)
	return resumed
}

// historyStepOrder walks the Flow's linear step chain from the start step and
// returns the ordered names of steps recorded in snapshot.StepLog, up to and
// including the PausedAt step. This reconstructs the historical StepExecLog
// order that ExecutionSnapshot.toExecution cannot derive from the StepLog map
// alone. Branch steps not on the linear chain are omitted from the order but
// remain present in the merged StepLog map. The read lock is released before
// any subsequent Flow.Resume call to avoid recursive locking.
func (f *Flow) historyStepOrder(snapshot *ExecutionSnapshot) []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var order []string
	cur := f.findStartStep()
	for cur != nil {
		if _, exists := snapshot.StepLog[cur.Name]; exists {
			order = append(order, cur.Name)
		}
		if cur.Name == snapshot.PausedAt {
			break
		}
		next := f.findNextStep(cur.Name)
		if next == "" {
			break
		}
		cur = f.steps[next]
	}
	return order
}
