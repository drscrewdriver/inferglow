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
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// F-MEDIUM-9: WorkerPool task panic recover + 日志
//
// 现状（修复前）：runTask 没有 recover，task panic 会向上传播，
// 导致 worker goroutine 死亡，整个进程崩溃（Go 中未 recover 的 panic
// 会终止程序）。多个 task 共享 worker，一个 task panic 会杀死 worker，
// 后续 task 无法执行，pool 实际不可用。
//
// 修复要求：
//   - runTask 添加 `defer func() { if r := recover(); r != nil { log.Printf(...) } }()`
//   - panic 不杀死 worker
//   - 后续 task 仍能正常执行
//   - taskWg.Done() 仍被调用（避免 Wait() 死锁）
// ============================================================================

// TestWorkerPool_TaskPanicRecovered 验证 task panic 被恢复后，
// worker 不死，后续 task 仍能正常执行。
//
// 修复前（RED）：panic 会终止整个测试进程，go test 报告失败（panic stack trace）。
// 修复后（GREEN）：panic 被捕获，worker 存活，normalTask 执行完成。
func TestWorkerPool_TaskPanicRecovered(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	var completed int64

	// 提交一个会 panic 的 task
	if err := p.Submit(func() {
		panic("intentional test panic")
	}); err != nil {
		t.Fatalf("Submit(panic task) failed: %v", err)
	}

	// 提交一个正常 task —— panic 后 worker 应仍能处理它
	if err := p.Submit(func() {
		atomic.AddInt64(&completed, 1)
	}); err != nil {
		t.Fatalf("Submit(normal task) failed: %v", err)
	}

	p.Wait()
	p.Stop()

	if got := atomic.LoadInt64(&completed); got != 1 {
		t.Errorf("completed = %d, want 1 (worker should survive panic and process next task)", got)
	}
}

// TestWorkerPool_TaskPanicDoesNotStopPool 验证 task panic 后 pool 仍可用。
func TestWorkerPool_TaskPanicDoesNotStopPool(t *testing.T) {
	p := NewWorkerPool(2, 4)
	p.Start()

	// 提交 panic task
	if err := p.Submit(func() {
		panic("another test panic")
	}); err != nil {
		t.Fatalf("Submit(panic task) failed: %v", err)
	}
	p.Wait()

	// pool 不应被标记为 stopped
	if p.IsStopped() {
		t.Error("pool should NOT be stopped after a task panic (panic should be recovered)")
	}

	// 提交另一个 task 验证 pool 仍可处理新任务
	var done int64
	if err := p.Submit(func() {
		atomic.AddInt64(&done, 1)
	}); err != nil {
		t.Fatalf("Submit after panic failed: %v", err)
	}
	p.Wait()
	p.Stop()

	if got := atomic.LoadInt64(&done); got != 1 {
		t.Errorf("done = %d, want 1 (pool should still process tasks after panic)", got)
	}
}

// TestWorkerPool_MultiplePanicsRecovered 验证多次 panic 不会杀死所有 worker，
// 正常 task 仍能完成。
func TestWorkerPool_MultiplePanicsRecovered(t *testing.T) {
	p := NewWorkerPool(2, 8)
	p.Start()

	var completed int64

	// 交替提交 panic task 和正常 task
	for i := 0; i < 3; i++ {
		if err := p.Submit(func() {
			panic("panic iteration")
		}); err != nil {
			t.Fatalf("Submit[%d](panic) failed: %v", i, err)
		}
		if err := p.Submit(func() {
			atomic.AddInt64(&completed, 1)
		}); err != nil {
			t.Fatalf("Submit[%d](normal) failed: %v", i, err)
		}
	}

	p.Wait()
	p.Stop()

	// 所有 3 个正常 task 都应完成
	if got := atomic.LoadInt64(&completed); got != 3 {
		t.Errorf("completed = %d, want 3 (all normal tasks should complete despite panics)", got)
	}
}

// TestWorkerPool_TaskPanicTaskWgDecremented 验证 panic 后 taskWg 仍被正确递减，
// Wait() 不会死锁。
func TestWorkerPool_TaskPanicTaskWgDecremented(t *testing.T) {
	p := NewWorkerPool(1, 4)
	p.Start()

	// 提交一个 panic task
	p.Submit(func() {
		panic("panic for taskWg test")
	})

	// Wait() 必须返回 —— 如果 taskWg.Done() 没在 panic 时被调用，会死锁
	done := make(chan struct{})
	go func() {
		p.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Good: Wait returned, taskWg was decremented
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() did not return within 2s - taskWg.Done() not called on panic (deadlock)")
	}

	p.Stop()
}

// TestWorkerPool_StartIdempotent verifies that calling Start() multiple times
// does NOT start additional workers beyond the configured number.
//
// F-HIGH-7: WorkerPool.Start 多次调用会启动多个 worker 集.
// 修复：添加 `started atomic.Bool` guard.
func TestWorkerPool_StartIdempotent(t *testing.T) {
	p := NewWorkerPool(4, 8)

	// 调用 Start 3 次
	p.Start()
	p.Start()
	p.Start()

	// 提交 100 个任务，每个任务记录自己的 goroutine ID（通过并发计数器）
	var concurrent int64
	var maxConcurrent int64
	for i := 0; i < 100; i++ {
		if err := p.Submit(func() {
			cur := atomic.AddInt64(&concurrent, 1)
			if cur > maxConcurrent {
				atomic.StoreInt64(&maxConcurrent, cur)
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt64(&concurrent, -1)
		}); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	p.Wait()
	p.Stop()

	// 最大并发数应不超过 4（workers 配置值）
	// 若 Start 不幂等，会启动 12 个 worker，maxConcurrent 可能 > 4
	if maxConcurrent > 4 {
		t.Errorf("max concurrent = %d, want <= 4 (Start should be idempotent)", maxConcurrent)
	}
}

// TestWorkerPool_StartIdempotentWorkerCount 直接验证：多次 Start 后
// 实际启动的 worker 数等于配置值。
func TestWorkerPool_StartIdempotentWorkerCount(t *testing.T) {
	p := NewWorkerPool(3, 8)

	// 用一个 channel 记录每个 worker goroutine 启动
	workerStarted := make(chan struct{}, 100)

	// 由于 worker 是私有的，我们通过提交大量短任务并测量并发度来间接验证
	// Start 3 次
	p.Start()
	p.Start()
	p.Start()

	var concurrent int64
	var maxConcurrent int64
	for i := 0; i < 50; i++ {
		p.Submit(func() {
			cur := atomic.AddInt64(&concurrent, 1)
			if cur > maxConcurrent {
				atomic.StoreInt64(&maxConcurrent, cur)
			}
			// 持续一段时间以便其他 worker 并发执行
			workerStarted <- struct{}{}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&concurrent, -1)
		})
	}

	p.Wait()
	p.Stop()

	// 期望最大并发 = 3
	if maxConcurrent > 3 {
		t.Errorf("max concurrent = %d, want <= 3 (Start idempotent)", maxConcurrent)
	}
	if maxConcurrent < 2 {
		t.Errorf("max concurrent = %d, expected at least 2 (concurrent)", maxConcurrent)
	}
}
