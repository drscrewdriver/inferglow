package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// OperatorKind 算子类型
type OperatorKind string

const (
	OpChunk          OperatorKind = "chunk"
	OpSignalGate     OperatorKind = "signal_gate"
	OpBatchFanout    OperatorKind = "batch_fanout"
	OpBatchCollect   OperatorKind = "batch_collect"
	OpForEachSplit   OperatorKind = "for_each_split"
	OpForEachCollect OperatorKind = "for_each_collect"
	OpMatchRoute     OperatorKind = "match_route"
	OpMatchCase      OperatorKind = "match_case"
	OpMatchCollect   OperatorKind = "match_collect"
	OpCollectBranch  OperatorKind = "collect_branch"
	OpIntervention   OperatorKind = "intervention_point"
	OpSubFlow        OperatorKind = "sub_flow"
	OpResultSink     OperatorKind = "result_sink"
)

// CallableRef Kind 常量
const (
	CallableRegistered = "registered"
	CallableInspected  = "inspected"
	CallableAnonymous  = "anonymous"
)

// CallableRef 可序列化 handler 引用，用于 Blueprint 序列化/反序列化。
type CallableRef struct {
	Kind     string // "registered" | "inspected" | "anonymous"
	Name     string
	Module   string
	Qualname string
	Line     int
}

// Operator 算子定义，是 TriggerFlow 编排的基本单元。
// 每个 Operator 监听若干信号、发射若干信号，并携带一个可序列化的 HandlerRef。
type Operator struct {
	ID            string
	Kind          OperatorKind
	Name          string
	ListenSignals []string
	EmitSignals   []string
	Options       map[string]any
	HandlerRef    *CallableRef
}

// OperatorRegistry 算子注册中心，管理一个 Flow 的所有算子。
type OperatorRegistry struct {
	mu        sync.RWMutex
	operators map[string]*Operator
}

// NewOperatorRegistry 创建算子注册中心。
func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{
		operators: make(map[string]*Operator),
	}
}

// Register 注册一个算子。ID 重复时返回 error。
func (r *OperatorRegistry) Register(op *Operator) error {
	if op == nil {
		return fmt.Errorf("cannot register nil operator")
	}
	if op.ID == "" {
		return fmt.Errorf("operator ID cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.operators[op.ID]; exists {
		return fmt.Errorf("operator %q already registered", op.ID)
	}
	r.operators[op.ID] = op
	return nil
}

// Get 按 ID 查找算子。
func (r *OperatorRegistry) Get(id string) (*Operator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.operators[id]
	if !ok {
		return nil, fmt.Errorf("operator %q not found", id)
	}
	return op, nil
}

// List 返回所有已注册算子的列表。
func (r *OperatorRegistry) List() []*Operator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Operator, 0, len(r.operators))
	for _, op := range r.operators {
		result = append(result, op)
	}
	return result
}

// FindByListenSignal 返回所有监听指定信号的算子。
func (r *OperatorRegistry) FindByListenSignal(signal string) []*Operator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Operator
	for _, op := range r.operators {
		for _, s := range op.ListenSignals {
			if s == signal {
				result = append(result, op)
				break
			}
		}
	}
	return result
}

// OperatorContext 算子执行上下文
type OperatorContext struct {
	Ctx        context.Context
	Operator   *Operator
	Input      any
	SignalNet  *SignalNet
	EmitSignal func(signal Signal)
}

// OperatorHandler 算子 Handler 接口
type OperatorHandler interface {
	Kind() OperatorKind
	Execute(oc *OperatorContext) (any, error)
}

// ErrOperatorNotImplemented 未实现的算子 Kind 错误
var ErrOperatorNotImplemented = errors.New("operator kind not implemented")
