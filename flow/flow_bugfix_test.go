package flow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// F-MEDIUM-12: Flow sync.RWMutex
//
// 现状（修复前）：Flow 结构体无 mutex 保护，f.steps / f.edges / f.branches
// 等 map/slice 字段在并发访问下会触发 Go runtime 的
// "fatal error: concurrent map read and map write"（runtime 内置检测，
// 不需要 -race flag 也能触发）。
//
// 典型场景：一个 goroutine 通过 FlowBuilder.AddStep 修改 flow.steps
// （写 map），另一个 goroutine 通过 Flow.Execute 读取 flow.steps
// （读 map）。runtime 检测到并发 map 读写会立即终止进程。
//
// 修复要求：
//   - Flow 添加 mu sync.RWMutex 字段
//   - Flow.Execute / Flow.Resume 加读锁（RLock）
//   - FlowBuilder.AddStep / To / If 加写锁（Lock）
// ============================================================================

// TestFlow_ConcurrentExecuteAndBuilderAccess 验证 Flow 在并发 Execute +
// AddStep 下不触发 fatal error。
//
// 修复前（RED）：Go runtime 检测到 concurrent map read and map write，
// 抛出 fatal error，测试进程崩溃，go test 报告 FAIL。
// 修复后（GREEN）：RWMutex 序列化 map 访问，无 fatal error，测试通过。
func TestFlow_ConcurrentExecuteAndBuilderAccess(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input, nil
	}).Build()

	builder := NewFlow()
	builder.AddStep(stepA)
	flow := builder.Build()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: 持续通过 builder.AddStep 修改 flow.steps（map 写）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				step := NewStep(fmt.Sprintf("dyn-%d", i), func(ctx context.Context, input any) (any, error) {
					return input, nil
				}).Build()
				builder.AddStep(step)
			}
		}
	}()

	// Reader: 持续调用 flow.Execute（读 flow.steps map）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = flow.Execute(context.Background(), "input")
			}
		}
	}()

	// 运行足够长时间以触发 race（如未修复）
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
	// 若执行到此处说明未触发 fatal error。
}

// TestFlow_ConcurrentExecuteReadOnly 验证并发 Execute（纯读）安全且结果正确。
// 这是 F-MEDIUM-12 的回归测试：加锁后 Execute 仍能并发正确执行。
func TestFlow_ConcurrentExecuteReadOnly(t *testing.T) {
	stepA := NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "-A", nil
	}).Build()

	flow := NewFlow().AddStep(stepA).Build()

	var wg sync.WaitGroup
	var failed atomic.Bool
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				exec := flow.Execute(context.Background(), "start")
				if exec.State.Status != StatusCompleted {
					failed.Store(true)
					return
				}
				result, ok := exec.State.Result.(string)
				if !ok || result != "start-A" {
					failed.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()

	if failed.Load() {
		t.Error("concurrent Execute produced incorrect results")
	}
}
