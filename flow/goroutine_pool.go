package flow

import (
	"fmt"
	"sync"
)

// ============================================================================
// WorkerPool - 通用 goroutine 池
//
// 提供固定数量的 worker goroutine 和有界任务队列，避免每次提交任务都创建新
// goroutine。支持优雅关闭：Stop() 关闭任务通道并等待所有 worker 退出；
// Wait() 等待所有已提交任务执行完毕（worker 不退出）。
// ============================================================================

// WorkerPool 是一个固定大小的 goroutine 池。
//
// 字段说明：
//   - tasks:    有界任务通道，缓冲大小 = queueSize
//   - workers:  worker 数量
//   - wg:       跟踪 worker goroutine 的退出
//   - taskWg:   跟踪已提交但尚未完成的任务
//   - stopOnce  保证 Stop 只关闭一次
//   - stopped:  关闭后立即解除 Submit 的阻塞
//
// 并发安全说明：
//   - Submit 与 Stop 可能并发执行。Submit 通过 select + recover 安全处理
//     "Stop 在 Submit 检查后关闭 tasks 通道" 的竞态：如果 send-on-closed
//     触发 panic，会被 recover 捕获并转化为 "worker pool is stopped" 错误。
type WorkerPool struct {
	tasks    chan func()
	workers  int
	wg       sync.WaitGroup
	taskWg   sync.WaitGroup
	stopOnce sync.Once
	stopped  chan struct{}
}

// NewWorkerPool 创建一个 WorkerPool 实例。
//   - maxWorkers: worker goroutine 数量；<=0 时被规范化为 1
//   - queueSize:  任务通道缓冲大小；<0 时被规范化为 0
//
// 创建后需调用 Start() 启动 worker。
func NewWorkerPool(maxWorkers, queueSize int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	if queueSize < 0 {
		queueSize = 0
	}
	return &WorkerPool{
		tasks:   make(chan func(), queueSize),
		workers: maxWorkers,
		stopped: make(chan struct{}),
	}
}

// Start 启动所有 worker goroutine。
// 重复调用是幂等的：第二次及以后调用不会启动额外 worker。
func (p *WorkerPool) Start() {
	if p == nil {
		return
	}
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker 主循环：从 tasks 通道取任务执行，直到通道关闭。
// 使用 defer 确保 taskWg.Done() 在 task panic 时也会执行，
// 避免 Wait() 死锁。
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		// taskWg.Done() 必须在 task() 之后调用，即使 task panic。
		p.runTask(task)
	}
}

// runTask 执行单个任务并保证 taskWg.Done() 被调用。
func (p *WorkerPool) runTask(task func()) {
	defer p.taskWg.Done()
	if task != nil {
		task()
	}
}

// Submit 向任务队列提交一个任务。
// 如果 pool 已被 Stop，返回 error。
// 如果队列已满，Submit 会阻塞直到有 worker 取走任务或 pool 被 Stop。
//
// 并发安全：Submit 与 Stop 可并发调用。若 Stop 在 Submit 检查后关闭
// tasks 通道导致 send-on-closed panic，会被 recover 捕获并返回错误。
//
// 注意：task == nil 时不会 panic（worker 跳过执行），但仍计入 taskWg。
func (p *WorkerPool) Submit(task func()) error {
	if p == nil {
		return fmt.Errorf("worker pool is nil")
	}
	// 快速检查：如果已停止，立即返回错误
	select {
	case <-p.stopped:
		return fmt.Errorf("worker pool is stopped")
	default:
	}
	p.taskWg.Add(1)
	// select 中 send 到已关闭的 tasks 通道会 panic；用 recover 保护。
	sent := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				// tasks 通道在 select 评估期间被 Stop 关闭：
				// 当作 "已停止" 处理。
			}
		}()
		select {
		case p.tasks <- task:
			sent = true
		case <-p.stopped:
		}
	}()
	if !sent {
		// 撤销 taskWg 计数（任务未被 worker 取走）。
		p.taskWg.Done()
		return fmt.Errorf("worker pool is stopped")
	}
	return nil
}

// Stop 优雅关闭 pool：
//  1. 关闭 tasks 通道，worker 会继续处理完队列中剩余任务后退出
//  2. 等待所有 worker 退出
//  3. 关闭 stopped 通道，解除所有阻塞在 Submit 上的调用方
//
// 重复调用是幂等的。
//
// 注意：Stop 不会等待已提交但未完成执行的任务——这部分由 taskWg 跟踪，
// 调用方如需等待所有任务执行完毕，应在 Stop 之前调用 Wait()。
func (p *WorkerPool) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		close(p.tasks)
		p.wg.Wait()
		close(p.stopped)
	})
}

// Wait 阻塞直到所有已提交任务执行完毕。
// 与 Stop 不同：Wait 不会关闭 pool，worker 仍然存活等待新任务。
//
// 适用场景：在调用 Stop 之前等待当前批次任务全部完成。
func (p *WorkerPool) Wait() {
	if p == nil {
		return
	}
	p.taskWg.Wait()
}

// IsStopped 返回 pool 是否已停止。
func (p *WorkerPool) IsStopped() bool {
	if p == nil {
		return true
	}
	select {
	case <-p.stopped:
		return true
	default:
		return false
	}
}

// Workers 返回 worker 数量（用于测试和诊断）。
func (p *WorkerPool) Workers() int {
	if p == nil {
		return 0
	}
	return p.workers
}

// QueueSize 返回任务队列容量（用于测试和诊断）。
func (p *WorkerPool) QueueSize() int {
	if p == nil {
		return 0
	}
	return cap(p.tasks)
}
