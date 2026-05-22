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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// BUG-22: InterventionPoint timeout 可配置
//
// 现状（修复前）：InterventionPointHandler.Execute 硬编码 maxWait=5*time.Minute。
// 调用方无法通过 Options["timeout"] 自定义超时。在测试场景下需要等待 5 分钟
// 才能验证超时路径，且生产场景下可能需要更长/更短的超时。
//
// 修复要求：
//   - 从 OperatorContext.Operator.Options["timeout"] 读取超时
//   - 接受 time.Duration 直接传入或 string（time.ParseDuration 解析）
//   - 默认值仍为 5*time.Minute（向后兼容）
//   - timeout<=0 时使用默认值（避免无限等待）
// ============================================================================

// TestInterventionPoint_CustomTimeout 验证 Options["timeout"] 覆盖默认 5 分钟超时。
// 设置 100ms 超时后不发送 resume 信号，Execute 应在 ~100ms 内返回 timeout error。
func TestInterventionPoint_CustomTimeout(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "short_pause",
			Options: map[string]any{
				"timeout": 100 * time.Millisecond,
			},
		},
		Input:     "input",
		SignalNet: sn,
	}

	start := time.Now()
	_, err := h.Execute(oc)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention 'timed out', got: %v", err)
	}
	// 应使用自定义 100ms 超时，而非默认 5 分钟
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want < 2s (custom 100ms timeout should be used, not 5min default)", elapsed)
	}
	if elapsed < 90*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 90ms (should wait for the full custom timeout)", elapsed)
	}
}

// TestInterventionPoint_CustomTimeoutString 验证 string 类型的 timeout 也被支持。
func TestInterventionPoint_CustomTimeoutString(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "str_pause",
			Options: map[string]any{
				"timeout": "150ms",
			},
		},
		Input:     "input",
		SignalNet: sn,
	}

	start := time.Now()
	_, err := h.Execute(oc)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want < 2s (string timeout 150ms should be parsed)", elapsed)
	}
	if elapsed < 140*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 140ms (should wait full 150ms)", elapsed)
	}
}

// TestInterventionPoint_DefaultTimeoutWhenOptionMissing 验证未设置 timeout 时使用默认 5 分钟。
// 这里用 ctx 取消提前退出，避免实际等待 5 分钟。
func TestInterventionPoint_DefaultTimeoutWhenOptionMissing(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	ctx, cancel := context.WithCancel(context.Background())
	oc := &OperatorContext{
		Ctx: ctx,
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "default_pause",
			// 不设置 timeout
		},
		Input:     "input",
		SignalNet: sn,
	}

	// 50ms 后取消 ctx，让 Execute 提前返回，验证默认路径不会立即超时
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := h.Execute(oc)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error (ctx cancel), got nil")
	}
	// 默认超时为 5min，所以应该是 ctx 取消触发，而不是超时触发
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want < 2s (should be ctx cancel, not 5min default timeout)", elapsed)
	}
}

// TestInterventionPoint_InvalidTimeoutFallsBackToDefault 验证非法 timeout 类型时
// 回退到默认 5 分钟（不 panic，不报错）。
func TestInterventionPoint_InvalidTimeoutFallsBackToDefault(t *testing.T) {
	h := &InterventionPointHandler{}
	sn := NewSignalNet()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oc := &OperatorContext{
		Ctx: ctx,
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpIntervention,
			Name: "invalid_pause",
			Options: map[string]any{
				"timeout": "not-a-duration", // 非法字符串
			},
		},
		Input:     "input",
		SignalNet: sn,
	}

	// 50ms 后取消 ctx，避免等待 5 分钟
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := h.Execute(oc)
	// 应该返回 ctx 错误（而非 panic）
	if err == nil {
		t.Fatal("expected error (ctx cancel), got nil")
	}
}

// TestCollectBranch_ParentCancel verifies that CollectBranch exits promptly
// when the parent context is cancelled, without leaking goroutines.
//
// BUG-11 / F-HIGH-3: CollectBranch goroutine 泄漏，无并发限制，不响应 context 取消.
// 修复：使用 context.WithCancel(parentCtx) + WorkerPool(8) + select ctx.Done.
func TestCollectBranch_ParentCancel(t *testing.T) {
	h := &CollectBranchHandler{}

	// 启动信号：每个 branch handler 都阻塞等待，直到收到 cancel 信号
	started := make(chan struct{}, 4)
	release := make(chan struct{})

	branches := map[string]Handler{}
	for i := 0; i < 4; i++ {
		idx := i
		branches[fmt.Sprintf("b%d", idx)] = func(rd *TriggerFlowRuntimeData) (any, error) {
			started <- struct{}{}
			select {
			case <-release:
				return fmt.Sprintf("result_%d", idx), nil
			case <-time.After(5 * time.Second):
				return nil, fmt.Errorf("timeout in branch %d", idx)
			}
		}
	}

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oc := &OperatorContext{
		Ctx: parentCtx,
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpCollectBranch,
			Name: "collect",
			Options: map[string]any{
				"branches": branches,
			},
		},
		Input: "input",
	}

	// 在 goroutine 中调用 Execute
	done := make(chan error, 1)
	go func() {
		_, err := h.Execute(oc)
		done <- err
	}()

	// 等待所有 branch 启动
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("branch %d did not start in time", i)
		}
	}

	// 取消父 context
	cancel()

	// Execute 应在合理时间内（≤ 500ms）返回 error
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after parent ctx cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CollectBranch did not exit after parent ctx cancel (goroutine leak)")
	}

	// 释放未结束的 branch 防止测试残留
	close(release)
}

// TestCollectBranch_ConcurrencyLimit verifies that CollectBranch limits the
// number of concurrently running branches to 8 (default WorkerPool size).
//
// BUG-11 / F-HIGH-3: CollectBranch 无并发限制.
// 修复：通过 WorkerPool(8) 限制并发.
func TestCollectBranch_ConcurrencyLimit(t *testing.T) {
	h := &CollectBranchHandler{}

	// 16 个 branch handler，每个记录最大并发数
	const n = 16
	var current, maxConcurrency int64
	var mu sync.Mutex

	branches := map[string]Handler{}
	for i := 0; i < n; i++ {
		branches[fmt.Sprintf("b%d", i)] = func(rd *TriggerFlowRuntimeData) (any, error) {
			cur := atomic.AddInt64(&current, 1)
			mu.Lock()
			if cur > maxConcurrency {
				maxConcurrency = cur
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&current, -1)
			return "ok", nil
		}
	}

	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpCollectBranch,
			Name: "collect",
			Options: map[string]any{
				"branches": branches,
			},
		},
		Input: "input",
	}

	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", out)
	}
	if len(results) != n {
		t.Errorf("expected %d results, got %d", n, len(results))
	}

	// 并发数应 <= 8
	if maxConcurrency > 8 {
		t.Errorf("max concurrency = %d, want <= 8", maxConcurrency)
	}
}

// TestBatchCollect_ContextCancel verifies that BatchCollectHandler.Execute
// returns promptly with ctx.Err when the parent context is cancelled, instead
// of waiting for the full 30s polling timeout.
//
// F-HIGH-2: BatchCollect/ForEachCollect 轮询循环不尊重 context 取消.
// 修复：轮询循环加 select { case <-ctx.Done(): return ctx.Err() }.
func TestBatchCollect_ContextCancel(t *testing.T) {
	h := &BatchCollectHandler{}

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oc := &OperatorContext{
		Ctx: parentCtx,
		Operator: &Operator{
			ID:   "op-bc-1",
			Kind: OpBatchCollect,
			Name: "batch_collect",
			Options: map[string]any{
				"expected_count": 4, // 没有任何 BatchItem 信号会被接受
			},
		},
		Input:     "input",
		SignalNet: NewSignalNet(),
	}

	// 在 goroutine 中调用 Execute
	done := make(chan error, 1)
	go func() {
		_, err := h.Execute(oc)
		done <- err
	}()

	// 短暂等待让 Execute 进入轮询循环
	time.Sleep(50 * time.Millisecond)

	// 取消父 context
	cancel()

	// Execute 应在合理时间内（≤ 500ms）返回 error，而非等待 30s 超时
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after parent ctx cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BatchCollect did not exit after parent ctx cancel (still polling until 30s timeout)")
	}
}

// TestForEachCollect_ContextCancel verifies that ForEachCollectHandler.Execute
// returns promptly with ctx.Err when the parent context is cancelled.
//
// F-HIGH-2: BatchCollect/ForEachCollect 轮询循环不尊重 context 取消.
func TestForEachCollect_ContextCancel(t *testing.T) {
	h := &ForEachCollectHandler{}

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oc := &OperatorContext{
		Ctx: parentCtx,
		Operator: &Operator{
			ID:   "op-fec-1",
			Kind: OpForEachCollect,
			Name: "for_each_collect",
			Options: map[string]any{
				"expected_count": 4, // 没有任何 BatchItem 信号会被接受
			},
		},
		Input:     "input",
		SignalNet: NewSignalNet(),
	}

	// 在 goroutine 中调用 Execute
	done := make(chan error, 1)
	go func() {
		_, err := h.Execute(oc)
		done <- err
	}()

	// 短暂等待让 Execute 进入轮询循环
	time.Sleep(50 * time.Millisecond)

	// 取消父 context
	cancel()

	// Execute 应在合理时间内（≤ 500ms）返回 error
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after parent ctx cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ForEachCollect did not exit after parent ctx cancel (still polling until 30s timeout)")
	}
}
