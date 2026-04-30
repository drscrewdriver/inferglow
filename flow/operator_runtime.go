package flow

import (
	"fmt"
	"sync"
)

// OperatorRuntime 算子运行时。
// 将 OperatorRegistry + SignalNet + Handler 集合串联起来，
// 根据 Operator.Kind 路由到对应的 OperatorHandler 执行。
type OperatorRuntime struct {
	mu        sync.RWMutex
	registry  *OperatorRegistry
	signalNet *SignalNet
	handlers  map[OperatorKind]OperatorHandler
}

// NewOperatorRuntime 创建算子运行时。
// reg / sn 允许为 nil（仅在不需要相应能力的场景下使用）。
func NewOperatorRuntime(reg *OperatorRegistry, sn *SignalNet) *OperatorRuntime {
	return &OperatorRuntime{
		registry:  reg,
		signalNet: sn,
		handlers:  make(map[OperatorKind]OperatorHandler),
	}
}

// RegisterHandler 注册一个算子 Handler。
// 同一 Kind 多次注册会覆盖之前的 Handler。
func (r *OperatorRuntime) RegisterHandler(h OperatorHandler) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Kind()] = h
}

// Dispatch 根据 oc.Operator.Kind 路由到注册的 Handler 执行。
// 若未注册对应 Kind 的 Handler，返回 ErrOperatorNotImplemented。
func (r *OperatorRuntime) Dispatch(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("dispatch: nil operator context")
	}
	if oc.Operator == nil {
		return nil, fmt.Errorf("dispatch: nil operator in context")
	}

	r.mu.RLock()
	handler, ok := r.handlers[oc.Operator.Kind]
	r.mu.RUnlock()

	if !ok || handler == nil {
		return nil, fmt.Errorf("%w: %s", ErrOperatorNotImplemented, oc.Operator.Kind)
	}

	return handler.Execute(oc)
}
