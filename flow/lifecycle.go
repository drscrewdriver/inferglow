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

// LifecycleState 表示 Execution 的生命周期状态。
// 与 ExecutionStatus（扁平字符串）不同，LifecycleState 通过
// LifecycleMachine 提供严格的状态转换控制。
type LifecycleState string

const (
	// LifecycleOpen 初始状态，接受所有操作。
	LifecycleOpen LifecycleState = "open"
	// LifecycleRunning 正在执行。
	LifecycleRunning LifecycleState = "running"
	// LifecycleWaiting 等待干预恢复。
	LifecycleWaiting LifecycleState = "waiting"
	// LifecycleFailed 执行失败。
	LifecycleFailed LifecycleState = "failed"
	// LifecycleSealed 封闭，不再接受新信号。
	LifecycleSealed LifecycleState = "sealed"
	// LifecycleClosed 最终状态。
	LifecycleClosed LifecycleState = "closed"
)

// validTransitions 定义合法的状态转换矩阵。
// key 为 from 状态，value 为可转换到的目标状态列表。
var validTransitions = map[LifecycleState][]LifecycleState{
	LifecycleOpen:    {LifecycleRunning, LifecycleClosed},
	LifecycleRunning: {LifecycleWaiting, LifecycleFailed, LifecycleSealed, LifecycleClosed},
	LifecycleWaiting: {LifecycleRunning, LifecycleFailed, LifecycleClosed},
	LifecycleSealed:  {LifecycleClosed},
	LifecycleFailed:  {LifecycleClosed},
	LifecycleClosed:  {}, // 最终状态，无出边
}

// isValidTransition 验证状态转换是否合法。
func isValidTransition(from, to LifecycleState) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// LifecycleMachine 状态机。
// 维护当前状态、历史状态列表和错误信息。
// 所有方法均线程安全。
type LifecycleMachine struct {
	mu        sync.RWMutex
	current   LifecycleState
	history   []LifecycleState
	errorInfo string
}

// NewLifecycleMachine 创建状态机，初始状态为 LifecycleOpen。
func NewLifecycleMachine() *LifecycleMachine {
	return &LifecycleMachine{
		current: LifecycleOpen,
		history: []LifecycleState{LifecycleOpen},
	}
}

// Transition 执行状态转换。
// 验证 (from -> to) 是合法转换，且当前状态确实为 from。
// 转换成功后追加 to 到 history。
// 返回 error 描述转换不合法的原因。
func (m *LifecycleMachine) Transition(from, to LifecycleState) error {
	if m == nil {
		return fmt.Errorf("lifecycle machine is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !isValidTransition(from, to) {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}
	if m.current != from {
		return fmt.Errorf("current state is %s, expected %s", m.current, from)
	}
	m.current = to
	m.history = append(m.history, to)
	return nil
}

// Seal 将 execution 从 running 封闭为 sealed。
// sealed 状态不再接受新信号，但允许已注册的信号处理完成。
func (m *LifecycleMachine) Seal() error {
	return m.Transition(LifecycleRunning, LifecycleSealed)
}

// Close 关闭 execution。
// 接受四种 from 状态：
//   - open -> closed（启动前取消）
//   - sealed -> closed（自然完成）
//   - running -> closed（强制关闭）
//   - failed -> closed（终止）
//   - waiting -> closed（取消等待）
//
// reason 写入 errorInfo（仅在 from != sealed 时）。
//
// BUG-13/F-MEDIUM-2 修复：check-and-set 必须原子。原实现先在锁内读取
// m.current 解锁后再调用 m.Transition 重新加锁，存在 TOCTOU 窗口——其他
// goroutine 可在窗口内把状态改为另一个可 closeable 状态，导致 Close 用
// 过期的 from 调用 Transition 而失败。现直接在持锁状态下完成检查与转换。
func (m *LifecycleMachine) Close(reason string) error {
	if m == nil {
		return fmt.Errorf("lifecycle machine is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.current
	validFroms := []LifecycleState{LifecycleOpen, LifecycleSealed, LifecycleRunning, LifecycleFailed, LifecycleWaiting}
	for _, from := range validFroms {
		if current == from {
			if !isValidTransition(from, LifecycleClosed) {
				return fmt.Errorf("invalid transition: %s -> %s", from, LifecycleClosed)
			}
			m.current = LifecycleClosed
			m.history = append(m.history, LifecycleClosed)
			if from != LifecycleSealed && reason != "" {
				m.errorInfo = reason
			}
			return nil
		}
	}
	return fmt.Errorf("cannot close from state: %s", current)
}

// Fail 将 execution 从 running/waiting 转为 failed，并记录错误信息。
//
// BUG-13/F-MEDIUM-2 修复：同 Close，check-and-set 原子化。
func (m *LifecycleMachine) Fail(reason string) error {
	if m == nil {
		return fmt.Errorf("lifecycle machine is nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.current
	validFroms := []LifecycleState{LifecycleRunning, LifecycleWaiting}
	for _, from := range validFroms {
		if current == from {
			if !isValidTransition(from, LifecycleFailed) {
				return fmt.Errorf("invalid transition: %s -> %s", from, LifecycleFailed)
			}
			m.current = LifecycleFailed
			m.history = append(m.history, LifecycleFailed)
			m.errorInfo = reason
			return nil
		}
	}
	return fmt.Errorf("cannot fail from state: %s", current)
}

// Wait 将 execution 从 running 转为 waiting（干预暂停）。
func (m *LifecycleMachine) Wait() error {
	return m.Transition(LifecycleRunning, LifecycleWaiting)
}

// Resume 将 execution 从 waiting 恢复为 running。
func (m *LifecycleMachine) Resume() error {
	return m.Transition(LifecycleWaiting, LifecycleRunning)
}

// Start 将 execution 从 open 转为 running。
func (m *LifecycleMachine) Start() error {
	return m.Transition(LifecycleOpen, LifecycleRunning)
}

// Current 返回当前状态。
func (m *LifecycleMachine) Current() LifecycleState {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// History 返回状态转换历史的副本。
// 第一个元素始终是 LifecycleOpen。
func (m *LifecycleMachine) History() []LifecycleState {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]LifecycleState, len(m.history))
	copy(out, m.history)
	return out
}

// SetError 记录错误信息（不影响状态）。
func (m *LifecycleMachine) SetError(err string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorInfo = err
}

// GetError 获取错误信息。
func (m *LifecycleMachine) GetError() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.errorInfo
}

// IsTerminal 检查当前状态是否为终态（closed）。
func (m *LifecycleMachine) IsTerminal() bool {
	return m.Current() == LifecycleClosed
}

// CanTransition 检查从当前状态能否转换到 to。
func (m *LifecycleMachine) CanTransition(to LifecycleState) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return isValidTransition(m.current, to)
}
