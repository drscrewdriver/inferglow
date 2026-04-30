package flow

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// NewWorkerPool / 基础属性测试
// ============================================================================

// Check: NewWorkerPool 创建 pool 且字段正确
func TestNewWorkerPool(t *testing.T) {
	p := NewWorkerPool(4, 16)
	if p == nil {
		t.Fatal("NewWorkerPool returned nil")
	}
	if p.Workers() != 4 {
		t.Errorf("Workers = %d, want 4", p.Workers())
	}
	if p.QueueSize() != 16 {
		t.Errorf("QueueSize = %d, want 16", p.QueueSize())
	}
	if p.IsStopped() {
		t.Error("fresh pool should not be stopped")
	}
}

// Check: NewWorkerPool 对 maxWorkers<=0 规范化为 1
func TestNewWorkerPoolZeroWorkers(t *testing.T) {
	p := NewWorkerPool(0, 4)
	if p.Workers() != 1 {
		t.Errorf("Workers = %d, want 1 (normalized)", p.Workers())
	}
}

// Check: NewWorkerPool 对负数 queueSize 规范化为 0
func TestNewWorkerPoolNegativeQueueSize(t *testing.T) {
	p := NewWorkerPool(2, -1)
	if p.QueueSize() != 0 {
		t.Errorf("QueueSize = %d, want 0 (normalized)", p.QueueSize())
	}
}

// ============================================================================
// Start / Submit / Stop - 基本并发执行
// ============================================================================

// Check: Start + Submit + Stop 基本流程
func TestWorkerPoolBasicSubmit(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	var count int64
	for i := 0; i < 10; i++ {
		if err := p.Submit(func() {
			atomic.AddInt64(&count, 1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}
	p.Stop()

	if got := atomic.LoadInt64(&count); got != 10 {
		t.Errorf("count = %d, want 10", got)
	}
	if !p.IsStopped() {
		t.Error("pool should be stopped after Stop")
	}
}

// Check: 并发执行：所有任务都完成（结果正确）
func TestWorkerPoolConcurrentExecution(t *testing.T) {
	p := NewWorkerPool(4, 8)
	p.Start()

	n := 100
	results := make([]int, n)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		idx := i
		if err := p.Submit(func() {
			defer wg.Done()
			// 模拟工作
			time.Sleep(time.Millisecond)
			mu.Lock()
			results[idx] = idx * 2
			mu.Unlock()
		}); err != nil {
			t.Fatalf("Submit[%d] failed: %v", idx, err)
		}
	}
	wg.Wait()
	p.Stop()

	for i := 0; i < n; i++ {
		if results[i] != i*2 {
			t.Errorf("results[%d] = %d, want %d", i, results[i], i*2)
		}
	}
}

// Check: 并发度限制：maxWorkers=1 时任务串行执行
func TestWorkerPoolSerialExecution(t *testing.T) {
	p := NewWorkerPool(1, 4)
	p.Start()

	var concurrent int64
	var maxConcurrent int64
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		if err := p.Submit(func() {
			cur := atomic.AddInt64(&concurrent, 1)
			mu.Lock()
			if cur > maxConcurrent {
				maxConcurrent = cur
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&concurrent, -1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}
	p.Stop()

	if maxConcurrent != 1 {
		t.Errorf("max concurrent = %d, want 1 (serial)", maxConcurrent)
	}
}

// Check: 并发度限制：maxWorkers=4 时最多 4 个任务并发
func TestWorkerPoolMaxConcurrency4(t *testing.T) {
	p := NewWorkerPool(4, 8)
	p.Start()

	var concurrent int64
	var maxConcurrent int64
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		if err := p.Submit(func() {
			cur := atomic.AddInt64(&concurrent, 1)
			mu.Lock()
			if cur > maxConcurrent {
				maxConcurrent = cur
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&concurrent, -1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}
	p.Stop()

	if maxConcurrent > 4 {
		t.Errorf("max concurrent = %d, want <= 4", maxConcurrent)
	}
	if maxConcurrent < 2 {
		t.Errorf("max concurrent = %d, expected at least 2 (concurrent)", maxConcurrent)
	}
}

// ============================================================================
// 任务队列满 - Submit 阻塞
// ============================================================================

// Check: 队列满时 Submit 阻塞，直到 worker 取走任务
func TestWorkerPoolQueueFullBlocks(t *testing.T) {
	// maxWorkers=1, queueSize=1：1 个 worker + 1 个缓冲槽
	// 提交 1 个长任务 + 1 个排队 + 第 3 个必然阻塞
	p := NewWorkerPool(1, 1)
	p.Start()

	// 先占用 worker
	blocker := make(chan struct{})
	p.Submit(func() {
		<-blocker // 阻塞 worker
	})

	// 填满队列
	p.Submit(func() {})

	// 现在 worker 忙，队列满。第 3 个 Submit 应阻塞。
	submitDone := make(chan error, 1)
	go func() {
		submitDone <- p.Submit(func() {})
	}()

	select {
	case <-submitDone:
		t.Fatal("Submit should block when queue is full")
	case <-time.After(50 * time.Millisecond):
		// 预期：Submit 仍然阻塞
	}

	// 释放 worker
	close(blocker)

	select {
	case err := <-submitDone:
		if err != nil {
			t.Errorf("Submit returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Submit did not unblock after worker freed")
	}

	p.Stop()
}

// ============================================================================
// Stop - 优雅关闭
// ============================================================================

// Check: Stop 等待所有正在执行的任务完成
func TestWorkerPoolStopWaitsForRunningTasks(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	var completed int64
	for i := 0; i < 4; i++ {
		p.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt64(&completed, 1)
		})
	}

	stopDone := make(chan struct{})
	go func() {
		p.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// 预期：Stop 等所有任务完成才返回
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop did not return in time")
	}

	if got := atomic.LoadInt64(&completed); got != 4 {
		t.Errorf("completed = %d, want 4 (Stop should wait for all tasks)", got)
	}
}

// Check: Stop 后 Submit 返回 error
func TestWorkerPoolSubmitAfterStop(t *testing.T) {
	p := NewWorkerPool(1, 1)
	p.Start()
	p.Stop()

	err := p.Submit(func() {})
	if err == nil {
		t.Error("Submit after Stop should return error")
	}
}

// Check: 重复 Stop 是幂等的
func TestWorkerPoolStopIdempotent(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()
	p.Stop()
	p.Stop() // 不应 panic
	p.Stop() // 不应 panic
	if !p.IsStopped() {
		t.Error("pool should be stopped")
	}
}

// Check: Stop 解除阻塞的 Submit
func TestWorkerPoolStopUnblocksSubmit(t *testing.T) {
	// maxWorkers=0 实际规范化为 1，但我们可以构造一个场景：
	// 让 worker 忙碌 + 队列满，然后 Stop 解除阻塞的 Submit
	p := NewWorkerPool(1, 0) // 无缓冲队列
	p.Start()

	blocker := make(chan struct{})
	p.Submit(func() {
		<-blocker
	})

	// 第 2 个 Submit 必然阻塞（无缓冲队列 + worker 忙）
	submitDone := make(chan error, 1)
	go func() {
		submitDone <- p.Submit(func() {})
	}()

	select {
	case <-submitDone:
		t.Fatal("Submit should block on unbuffered channel")
	case <-time.After(30 * time.Millisecond):
	}

	// Stop 应解除阻塞的 Submit
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(blocker)
	}()

	stopDone := make(chan struct{})
	go func() {
		p.Stop()
		close(stopDone)
	}()

	select {
	case err := <-submitDone:
		// Submit 可能在 Stop 之前或之后返回，但不应永远阻塞
		_ = err
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Submit was not unblocked by Stop")
	}

	<-stopDone
}

// ============================================================================
// Wait - 等待所有任务完成（不关闭 pool）
// ============================================================================

// Check: Wait 阻塞直到所有已提交任务完成
func TestWorkerPoolWait(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	var completed int64
	for i := 0; i < 10; i++ {
		p.Submit(func() {
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&completed, 1)
		})
	}

	p.Wait()

	if got := atomic.LoadInt64(&completed); got != 10 {
		t.Errorf("completed = %d, want 10", got)
	}
	if p.IsStopped() {
		t.Error("pool should still be running after Wait")
	}

	// 验证 Wait 后 pool 仍可用
	p.Submit(func() {
		atomic.AddInt64(&completed, 1)
	})
	p.Wait()
	if got := atomic.LoadInt64(&completed); got != 11 {
		t.Errorf("completed = %d, want 11 (pool should still be usable)", got)
	}

	p.Stop()
}

// Check: 空池调用 Wait 立即返回
func TestWorkerPoolWaitNoTasks(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()
	p.Wait() // 不应阻塞
	p.Stop()
}

// ============================================================================
// nil 安全
// ============================================================================

// Check: nil pool 的方法调用不 panic
func TestWorkerPoolNilSafe(t *testing.T) {
	var p *WorkerPool
	if err := p.Submit(func() {}); err == nil {
		t.Error("nil.Submit should return error")
	}
	p.Start()    // 不应 panic
	p.Stop()     // 不应 panic
	p.Wait()     // 不应 panic
	if !p.IsStopped() {
		t.Error("nil.IsStopped should return true")
	}
	if p.Workers() != 0 {
		t.Error("nil.Workers should return 0")
	}
	if p.QueueSize() != 0 {
		t.Error("nil.QueueSize should return 0")
	}
}

// ============================================================================
// nil task 安全
// ============================================================================

// Check: Submit(nil) 不导致 panic
func TestWorkerPoolSubmitNilTask(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	if err := p.Submit(nil); err != nil {
		t.Errorf("Submit(nil) returned error: %v", err)
	}
	p.Wait() // 不应死锁
	p.Stop()
}

// ============================================================================
// 并发 Submit + Stop
// ============================================================================

// Check: 多 goroutine 并发 Submit 与 Stop 不产生 race
func TestWorkerPoolConcurrentSubmitAndStop(t *testing.T) {
	p := NewWorkerPool(4, 8)
	p.Start()

	var submitted int64
	var completed int64

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				err := p.Submit(func() {
					atomic.AddInt64(&completed, 1)
				})
				if err == nil {
					atomic.AddInt64(&submitted, 1)
				}
			}
		}()
	}
	wg.Wait()
	p.Stop()

	// completed 应等于 submitted（每个成功提交的任务都被执行）
	if completed != submitted {
		t.Errorf("completed = %d, submitted = %d (should be equal)", completed, submitted)
	}
}

// ============================================================================
// BatchFanoutHandler 使用 WorkerPool 的回归测试
// ============================================================================

// Check: BatchFanoutHandler 仍能正确处理 N=3 fanout
func TestBatchFanoutHandlerWithWorkerPool(t *testing.T) {
	h := &BatchFanoutHandler{}
	input := []any{1, 2, 3}
	oc := &OperatorContext{
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"handler": func(v any) (any, error) {
					return v.(int) * 10, nil
				},
			},
		},
		Input: input,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok {
		t.Fatalf("output type = %T, want []any", out)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, want := range []int{10, 20, 30} {
		if got := results[i].(int); got != want {
			t.Errorf("results[%d] = %d, want %d", i, got, want)
		}
	}
}

// Check: max_concurrency 选项被正确读取
func TestBatchFanoutHandlerMaxConcurrencyOption(t *testing.T) {
	// 验证 readMaxConcurrency 的行为
	cases := []struct {
		name     string
		options  map[string]any
		n        int
		expected int
	}{
		{"default", nil, 10, 8},
		{"explicit 4", map[string]any{"max_concurrency": 4}, 10, 4},
		{"exceeds n", map[string]any{"max_concurrency": 100}, 5, 5},
		{"zero falls back", map[string]any{"max_concurrency": 0}, 10, 8},
		{"negative falls back", map[string]any{"max_concurrency": -1}, 10, 8},
		{"int64 type", map[string]any{"max_concurrency": int64(3)}, 10, 3},
		{"float64 type", map[string]any{"max_concurrency": float64(2)}, 10, 2},
		{"invalid type falls back", map[string]any{"max_concurrency": "4"}, 10, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			op := &Operator{Options: c.options}
			got := readMaxConcurrency(op, c.n)
			if got != c.expected {
				t.Errorf("readMaxConcurrency(%v, %d) = %d, want %d", c.options, c.n, got, c.expected)
			}
		})
	}
}

// Check: max_concurrency=1 时 BatchFanoutHandler 串行执行
func TestBatchFanoutHandlerSerialOption(t *testing.T) {
	h := &BatchFanoutHandler{}
	input := []any{1, 2, 3, 4, 5}

	var concurrent int64
	var maxConcurrent int64
	var mu sync.Mutex

	oc := &OperatorContext{
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"max_concurrency": 1,
				"handler": func(v any) (any, error) {
					cur := atomic.AddInt64(&concurrent, 1)
					mu.Lock()
					if cur > maxConcurrent {
						maxConcurrent = cur
					}
					mu.Unlock()
					time.Sleep(5 * time.Millisecond)
					atomic.AddInt64(&concurrent, -1)
					return v, nil
				},
			},
		},
		Input: input,
	}
	_, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if maxConcurrent != 1 {
		t.Errorf("max concurrent = %d, want 1 (serial mode)", maxConcurrent)
	}
}

// Check: BatchFanoutHandler 错误传播仍然正常
func TestBatchFanoutHandlerErrorWithWorkerPool(t *testing.T) {
	h := &BatchFanoutHandler{}
	input := []any{1, 2, 3}
	oc := &OperatorContext{
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"handler": func(v any) (any, error) {
					if v.(int) == 2 {
						return nil, fmt.Errorf("intentional error at %d", v)
					}
					return v, nil
				},
			},
		},
		Input: input,
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
