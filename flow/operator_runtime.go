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
