package flow

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// SubFlowFrame - 子流执行帧
//
// 记录一次子流嵌套调用的上下文：父流 ID、子流引用、运行时数据、流数据、
// 创建时间和结果。SubFlowFrame 主要用于持久化和可观测性，不直接驱动执行。
// ============================================================================

// SubFlowFrame 描述一次子流调用的执行帧。
//
// 字段说明：
//   - ParentID:    父流执行标识（通常为父 Operator.ID 或父 ExecutionID）
//   - ChildFlow:   子流引用，类型为 any 以避免 JSON 序列化时的循环引用。
//                  运行时可断言为 ChildFlow 接口使用。
//   - State:       子流自定义状态（可变，per-frame）
//   - RuntimeData: 创建帧时的 TriggerFlowRuntimeData 快照
//   - FlowData:    创建帧时的 FlowData 快照
//   - CreatedAt:   帧创建时间
//   - CompletedAt: 帧完成时间（nil 表示未完成）
//   - Result:      子流执行结果
//   - Error:       子流执行错误（若有）
type SubFlowFrame struct {
	ParentID    string                 `json:"parent_id" yaml:"parent_id"`
	ChildFlow   any                    `json:"-" yaml:"-"` // *TriggerFlow 或 ChildFlow，避免序列化
	State       map[string]any         `json:"state,omitempty" yaml:"state,omitempty"`
	RuntimeData *TriggerFlowRuntimeData `json:"runtime_data,omitempty" yaml:"runtime_data,omitempty"`
	FlowData    map[string]any         `json:"flow_data,omitempty" yaml:"flow_data,omitempty"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Result      any                    `json:"result,omitempty" yaml:"result,omitempty"`
	Error       string                 `json:"error,omitempty" yaml:"error,omitempty"`
}

// IsCompleted 返回 frame 是否已完成（无论成功或失败）。
func (f *SubFlowFrame) IsCompleted() bool {
	if f == nil {
		return false
	}
	return f.CompletedAt != nil
}

// ============================================================================
// ChildFlow - 子流统一接口
//
// 任何 *TriggerFlow[InputT, StreamT, ResultT] 通过 RunChild 方法实现该接口，
// 使得 SubFlowHandler 可以在不感知具体泛型参数的情况下调用子流。
// ============================================================================

// ChildFlow 是子流的统一调用接口。
// 实现者负责将 any 输入转换为自己的 InputT，并返回 any 类型的结果。
type ChildFlow interface {
	RunChild(input any) (any, error)
}

// RunChild 让 *TriggerFlow 实现 ChildFlow 接口。
// 输入会被断言为 InputT；类型不匹配时返回 error。
// 调用等价于 f.Run(input.(InputT))。
func (f *TriggerFlow[InputT, StreamT, ResultT]) RunChild(input any) (any, error) {
	var zero ResultT
	if f == nil {
		return zero, fmt.Errorf("subflow: nil trigger flow")
	}
	in, ok := input.(InputT)
	if !ok {
		return zero, fmt.Errorf("subflow: input type mismatch: got %T, want %T", input, *new(InputT))
	}
	return f.Run(in)
}

// ============================================================================
// SubFlowRegistry - 活跃子流帧注册表
//
// 全局注册表跟踪所有正在执行或已完成（但未清理）的 SubFlowFrame。
// 主要用途：
//   - 持久化引擎枚举所有子流帧用于 ExecutionSnapshot
//   - 调试/可观测性工具查看当前嵌套层级
//   - 错误诊断时定位失败的子流
// ============================================================================

// SubFlowRegistry 跟踪所有活跃的 SubFlowFrame。
type SubFlowRegistry struct {
	mu     sync.RWMutex
	frames map[string]*SubFlowFrame
}

// NewSubFlowRegistry 创建空的 SubFlowRegistry。
func NewSubFlowRegistry() *SubFlowRegistry {
	return &SubFlowRegistry{
		frames: make(map[string]*SubFlowFrame),
	}
}

// Register 注册一个 SubFlowFrame。frame.ID 由调用方在 ParentID 基础上保证唯一。
// 若 ID 已存在，覆盖旧值。
func (r *SubFlowRegistry) Register(id string, frame *SubFlowFrame) {
	if r == nil || id == "" || frame == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frames == nil {
		r.frames = make(map[string]*SubFlowFrame)
	}
	r.frames[id] = frame
}

// Unregister 移除一个已注册的 SubFlowFrame。返回是否成功移除。
func (r *SubFlowRegistry) Unregister(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.frames[id]; !ok {
		return false
	}
	delete(r.frames, id)
	return true
}

// Get 按 ID 查找 SubFlowFrame。
func (r *SubFlowRegistry) Get(id string) (*SubFlowFrame, bool) {
	if r == nil || id == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.frames[id]
	return f, ok
}

// List 返回所有已注册的 SubFlowFrame 切片（副本）。
func (r *SubFlowRegistry) List() []*SubFlowFrame {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*SubFlowFrame, 0, len(r.frames))
	for _, f := range r.frames {
		out = append(out, f)
	}
	return out
}

// Count 返回已注册的 frame 数量。
func (r *SubFlowRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.frames)
}

// Clear 清空所有 frame。
func (r *SubFlowRegistry) Clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = make(map[string]*SubFlowFrame)
}

// ============================================================================
// 全局 SubFlowRegistry
// ============================================================================

var (
	globalSubFlowRegistryMu sync.RWMutex
	globalSubFlowRegistry   = NewSubFlowRegistry()
)

// GlobalSubFlowRegistry 返回全局 SubFlowRegistry。
func GlobalSubFlowRegistry() *SubFlowRegistry {
	globalSubFlowRegistryMu.RLock()
	defer globalSubFlowRegistryMu.RUnlock()
	return globalSubFlowRegistry
}

// SetGlobalSubFlowRegistry 替换全局 SubFlowRegistry。
// 传入 nil 时重置为空注册表。主要用于测试隔离。
func SetGlobalSubFlowRegistry(r *SubFlowRegistry) {
	globalSubFlowRegistryMu.Lock()
	defer globalSubFlowRegistryMu.Unlock()
	if r == nil {
		r = NewSubFlowRegistry()
	}
	globalSubFlowRegistry = r
}

// ResetGlobalSubFlowRegistry 重置全局 SubFlowRegistry 为空。用于测试隔离。
func ResetGlobalSubFlowRegistry() {
	SetGlobalSubFlowRegistry(NewSubFlowRegistry())
}

// ============================================================================
// SubFlowHandler 完整执行逻辑
//
// SubFlowHandler.Execute 在 operator_handlers.go 中定义，调用本文件的
// executeSubFlow 完成完整逻辑。executeSubFlow 负责：
//   1. 从 Operator.Options 解析子流引用（child_flow 或 child_flow_executor）
//   2. 创建 SubFlowFrame 并注册到 GlobalSubFlowRegistry
//   3. 调用子流执行
//   4. 记录结果/错误到 frame，设置 CompletedAt
//   5. 返回结果
// ============================================================================

// executeSubFlow 是 SubFlowHandler 的完整实现。
//
// 子流来源优先级：
//  1. Options["child_flow_executor"]: func(any) (any, error) - 灵活的函数式接口
//  2. Options["child_flow"]: ChildFlow 接口实现 - 直接传入 TriggerFlow 实例
//
// 若两者都未提供，原样返回 Input（骨架行为）。
//
// frameID 由调用方传入，用于在注册表中唯一标识本次调用。
// 若 frameID 为空，使用 Operator.ID + CreatedAt.UnixNano() 生成。
func executeSubFlow(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("sub_flow handler: nil operator context")
	}

	// 解析子流引用
	executor := readChildFlowExecutor(oc.Operator)
	childFlow := readChildFlow(oc.Operator)

	if executor == nil && childFlow == nil {
		// 骨架模式：无子流引用时原样返回
		return oc.Input, nil
	}

	// 创建 frame
	frameID := generateSubFlowFrameID(oc)
	frame := &SubFlowFrame{
		ParentID:    oc.Operator.ID,
		ChildFlow:   childFlow,
		State:       make(map[string]any),
		FlowData:    make(map[string]any),
		CreatedAt:   time.Now(),
	}

	// 注册 frame
	GlobalSubFlowRegistry().Register(frameID, frame)

	// 执行子流，确保 frame 完成状态被记录
	result, err := runChildFlow(executor, childFlow, oc.Input)

	// 记录结果到 frame
	completedAt := time.Now()
	frame.CompletedAt = &completedAt
	if err != nil {
		frame.Error = err.Error()
	} else {
		frame.Result = result
	}

	// 注意：不立即 Unregister，以便持久化引擎在 ExecutionSnapshot 时能枚举所有帧。
	// 调用方（或外部清理逻辑）负责调用 Unregister。

	return result, err
}

// runChildFlow 调用子流执行器或 ChildFlow 接口。
func runChildFlow(executor func(any) (any, error), childFlow ChildFlow, input any) (any, error) {
	if executor != nil {
		return executor(input)
	}
	if childFlow != nil {
		return childFlow.RunChild(input)
	}
	return input, nil
}

// readChildFlow 从 Options["child_flow"] 读取 ChildFlow 接口实现。
// 不存在或类型不匹配时返回 nil。
func readChildFlow(op *Operator) ChildFlow {
	if op == nil || op.Options == nil {
		return nil
	}
	raw, ok := op.Options["child_flow"]
	if !ok || raw == nil {
		return nil
	}
	cf, ok := raw.(ChildFlow)
	if !ok || cf == nil {
		return nil
	}
	return cf
}

// generateSubFlowFrameID 为每次子流调用生成唯一 ID。
// 使用 Operator.ID + 当前 UnixNano 保证唯一性。
func generateSubFlowFrameID(oc *OperatorContext) string {
	if oc == nil || oc.Operator == nil || oc.Operator.ID == "" {
		return fmt.Sprintf("subflow_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%d", oc.Operator.ID, time.Now().UnixNano())
}
