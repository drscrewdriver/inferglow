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
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package actionruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
)

// ActionDispatcher executes a list of ActionCalls using the ActionRegistry.
type ActionDispatcher struct {
	registry  *action.ActionRegistry
	auditHook audit.AuditHook
}

// NewActionDispatcher creates a dispatcher for the given registry. It is
// equivalent to NewActionDispatcherWithAudit with a NoOpHook so existing
// callers keep their pre-audit behavior and zero overhead.
func NewActionDispatcher(r *action.ActionRegistry) *ActionDispatcher {
	return NewActionDispatcherWithAudit(r, &audit.NoOpHook{})
}

// NewActionDispatcherWithAudit creates a dispatcher that appends an audit
// entry to hook after every registry.Execute call (success or failure). A
// nil hook disables auditing entirely; pass &audit.NoOpHook{} for the
// zero-overhead default.
func NewActionDispatcherWithAudit(r *action.ActionRegistry, hook audit.AuditHook) *ActionDispatcher {
	return &ActionDispatcher{registry: r, auditHook: hook}
}

// Execute 按并行安全声明（ActionSpec.ParallelSafe）分流执行所有 ActionCall：
// 声明 ParallelSafe=false 的调用在单独的 goroutine 中按原始调用顺序逐个串行执行；
// 其余调用（未声明 / 查不到 spec / 显式 true）照旧每个调用一个 goroutine 并发执行。
// 两组同时推进，结果仍按原始调用下标写入并返回。
// 每个 call 的审计条目无论 registry.Execute 成败都会追加；审计 Append 的
// 返回值被有意忽略，审计失败不会中断 action 执行。
func (d *ActionDispatcher) Execute(ctx context.Context, calls []ActionCall) []*action.ActionResult {
	results := make([]*action.ActionResult, len(calls))
	serial, concurrent := d.partitionCalls(calls)

	var wg sync.WaitGroup
	// 并行安全组：每个调用一个 goroutine 并发执行，与原有行为一致。
	wg.Add(len(concurrent))
	for _, idx := range concurrent {
		go func(i int) {
			defer wg.Done()
			d.executeCall(ctx, i, calls[i], results)
		}(idx)
	}
	// 串行组：单个 goroutine 内按原始调用顺序逐个执行。
	if len(serial) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, idx := range serial {
				d.executeCall(ctx, idx, calls[idx], results)
			}
		}()
	}

	wg.Wait()
	return results
}

// ExecuteInterruptible 与 Execute 采用相同的并行安全分流策略，但额外监听
// preemptCh：preemptCh 关闭时取消 ctx——正在执行的调用收到取消信号，
// 串行组中尚未开始的调用将以已取消的 ctx 立即返回错误结果——收集已完成的
// 结果并返回 (results, true)。全部调用未被打断时返回 (results, false)。
//
// The caller should pass a context that tools respect (ctx.Done()).
func (d *ActionDispatcher) ExecuteInterruptible(
	ctx context.Context,
	calls []ActionCall,
	preemptCh <-chan struct{},
) ([]*action.ActionResult, bool) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]*action.ActionResult, len(calls))
	serial, concurrent := d.partitionCalls(calls)
	var wg sync.WaitGroup
	var preempted bool
	done := make(chan struct{})

	wg.Add(len(concurrent))
	for _, idx := range concurrent {
		go func(i int) {
			defer wg.Done()
			d.executeCall(ctx, i, calls[i], results)
		}(idx)
	}
	if len(serial) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, idx := range serial {
				d.executeCall(ctx, idx, calls[idx], results)
			}
		}()
	}

	// Wait for either all calls to complete or preemption.
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All calls completed normally.
	case <-preemptCh:
		preempted = true
		cancel()  // Cancel remaining tools.
		wg.Wait() // Collect completed results.
	}

	return results, preempted
}

// partitionCalls 按并行安全声明把 calls 的下标划分为两组（均保持原始顺序）：
// serial 为声明 ParallelSafe=false 的调用下标（同轮须按调用顺序串行执行），
// concurrent 为其余调用下标（并行安全，照常并发执行）。
// 查不到 Action、Action 未携带 Spec 或 ParallelSafe 为 nil 的调用一律视为
// 并行安全，保持现有并发行为不变。
func (d *ActionDispatcher) partitionCalls(calls []ActionCall) (serial, concurrent []int) {
	for i, call := range calls {
		if d.isSerialCall(call.Name) {
			serial = append(serial, i)
		} else {
			concurrent = append(concurrent, i)
		}
	}
	return serial, concurrent
}

// isSerialCall 判断名字对应的 Action 是否声明了同轮须串行执行
// （ActionSpec.ParallelSafe 显式为 false）。
func (d *ActionDispatcher) isSerialCall(name string) bool {
	a := d.registry.GetAction(name)
	if a == nil || a.Spec == nil || a.Spec.ParallelSafe == nil {
		return false
	}
	return !*a.Spec.ParallelSafe
}

// executeCall 执行单个 ActionCall 并把结果写入 results[idx]，
// 包含 panic 恢复与审计上报，供 Execute 与 ExecuteInterruptible 共用。
func (d *ActionDispatcher) executeCall(ctx context.Context, idx int, c ActionCall, results []*action.ActionResult) {
	start := time.Now()

	// recover from executor panics: without this, results[idx]
	// stays nil and upstream consumers hit a nil-pointer panic.
	// On panic we synthesize a "panic" ActionResult and append an
	// audit entry so the failure is observable.
	defer func() {
		if r := recover(); r != nil {
			results[idx] = &action.ActionResult{
				OK:     false,
				Status: "panic",
				Error:  fmt.Sprintf("panic: %v", r),
			}
			if d.auditHook != nil {
				entry := &audit.AuditEntry{
					Timestamp: start,
					Source:    "action",
					Action:    "execute",
					Input:     c,
					Output:    results[idx],
					Duration:  time.Since(start),
					Metadata:  map[string]string{"action_name": c.Name},
					Error:     fmt.Sprintf("panic: %v", r),
				}
				_, _ = d.auditHook.Append(entry)
			}
		}
	}()

	result, err := d.registry.Execute(ctx, c.Name, c.Params)
	duration := time.Since(start)
	if err != nil {
		results[idx] = &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  err.Error(),
		}
	} else {
		results[idx] = result
	}
	if d.auditHook != nil {
		entry := &audit.AuditEntry{
			Timestamp: start,
			Source:    "action",
			Action:    "execute",
			Input:     c,
			Output:    results[idx],
			Duration:  duration,
			Metadata:  map[string]string{"action_name": c.Name},
		}
		if err != nil {
			entry.Error = err.Error()
		}
		// Intentionally ignore return values: audit failures must
		// not break action execution.
		_, _ = d.auditHook.Append(entry)
	}
}
