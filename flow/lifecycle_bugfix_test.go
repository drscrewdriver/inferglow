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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// BUG-13 / F-MEDIUM-2: LifecycleMachine Close/Fail TOCTOU 竞态条件
//
// 现状（修复前）：Close 和 Fail 方法先在锁内读取 m.current，解锁后再调用
// m.Transition(from, ...) 重新加锁。在解锁和重新加锁之间，状态可能被其他
// goroutine 改变。若新状态同样可转换到目标态（例如 running→waiting 都可
// closed），Close 仍会因使用过期的 from 而返回错误。
//
// 修复要求：Close 和 Fail 的 check-and-set 必须原子（持有锁整个操作）。
//
// TDD 方向：测试断言"在并发扰动下，Close/Fail 不应返回错误，且错误中
// 不应提到任何可 closeable/failable 的状态"。修复前因 TOCTOU 竞态，
// Close/Fail 会偶尔失败并返回 "current state is X" where X is closeable.
// ============================================================================

// closeableFromStates 是 Close 接受的所有 from 状态。
var closeableFromStates = []LifecycleState{
	LifecycleOpen, LifecycleSealed, LifecycleRunning,
	LifecycleFailed, LifecycleWaiting,
}

// failableFromStates 是 Fail 接受的所有 from 状态。
var failableFromStates = []LifecycleState{LifecycleRunning, LifecycleWaiting}

// errMentionsCloseableState 检查 err 是否提到一个可 closeable 的状态。
// 例如 "current state is waiting" 或 "cannot close from state: running"
// 都表示 Close 因 TOCTOU 而错误地失败了。
func errMentionsCloseableState(err error) (LifecycleState, bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	for _, s := range closeableFromStates {
		if strings.Contains(msg, fmt.Sprintf("current state is %s", s)) ||
			strings.Contains(msg, fmt.Sprintf("cannot close from state: %s", s)) {
			return s, true
		}
	}
	return "", false
}

func errMentionsFailableState(err error) (LifecycleState, bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	for _, s := range failableFromStates {
		if strings.Contains(msg, fmt.Sprintf("current state is %s", s)) ||
			strings.Contains(msg, fmt.Sprintf("cannot fail from state: %s", s)) {
			return s, true
		}
	}
	return "", false
}

// startPerturbers 启动 n 个 perturber goroutine，持续在 running/waiting 之间
// 切换状态。返回 stop channel 和 WaitGroup。close(stop) 后所有 perturber 退出，
// wg.Wait() 等待它们全部结束。
// holdDuration 控制 perturber 在 Wait 成功后、Resume 之前 sleep 多久，用于
// 拉宽 TOCTOU 窗口。
func startPerturbers(m *LifecycleMachine, n int, holdDuration time.Duration) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// 尝试 Wait，若成功则短暂保持 waiting 状态再 Resume
				if err := m.Wait(); err == nil {
					if holdDuration > 0 {
						time.Sleep(holdDuration)
					}
					_ = m.Resume()
				}
			}
		}()
	}
	return stop, &wg
}

// TestLifecycleMachine_CloseAtomic_NoTOCTOU 验证 Close 在并发扰动下不会
// 因 TOCTOU 而失败。修复前此测试会失败（Close 返回 "current state is X"
// 而 X 是可 closeable 的状态）。
func TestLifecycleMachine_CloseAtomic_NoTOCTOU(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	const iterations = 200
	var racesFound int64

	for i := 0; i < iterations; i++ {
		m := NewLifecycleMachine()
		if err := m.Start(); err != nil {
			t.Fatalf("iter %d: Start failed: %v", i, err)
		}

		// 启动 4 个 perturber，每个在 Wait 后 hold 50µs 再 Resume
		// 这会拉宽 TOCTOU 窗口，使 Close 更容易命中
		stop, perturberWG := startPerturbers(m, 4, 50*time.Microsecond)

		// 主线程调用 Close
		err := m.Close("done")

		close(stop)
		perturberWG.Wait()

		final := m.Current()
		if err == nil {
			if final != LifecycleClosed {
				t.Fatalf("iter %d: Close succeeded but final state = %q, want closed", i, final)
			}
		} else {
			// Close 失败 → 错误中不能提到任何可 closeable 的状态
			if s, ok := errMentionsCloseableState(err); ok {
				atomic.AddInt64(&racesFound, 1)
				t.Logf("iter %d: TOCTOU race: Close failed with %q but %s is closeable", i, err.Error(), s)
			}
		}
	}

	if racesFound > 0 {
		t.Fatalf("TOCTOU race detected %d times in %d Close calls - Close is not atomic", racesFound, iterations)
	}
}

// TestLifecycleMachine_FailAtomic_NoTOCTOU 验证 Fail 在并发扰动下不会
// 因 TOCTOU 而失败。修复前此测试会失败。
func TestLifecycleMachine_FailAtomic_NoTOCTOU(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	const iterations = 200
	var racesFound int64

	for i := 0; i < iterations; i++ {
		m := NewLifecycleMachine()
		if err := m.Start(); err != nil {
			t.Fatalf("iter %d: Start failed: %v", i, err)
		}

		stop, perturberWG := startPerturbers(m, 4, 50*time.Microsecond)

		err := m.Fail("transient")

		close(stop)
		perturberWG.Wait()

		final := m.Current()
		if err == nil {
			if final != LifecycleFailed {
				t.Fatalf("iter %d: Fail succeeded but final state = %q, want failed", i, final)
			}
		} else {
			if s, ok := errMentionsFailableState(err); ok {
				atomic.AddInt64(&racesFound, 1)
				t.Logf("iter %d: TOCTOU race: Fail failed with %q but %s is failable", i, err.Error(), s)
			}
		}
	}

	if racesFound > 0 {
		t.Fatalf("TOCTOU race detected %d times in %d Fail calls - Fail is not atomic", racesFound, iterations)
	}
}

// TestLifecycleMachine_CloseFromRunningSucceedsUnderConcurrency
// 是一个更直接的测试：state=running 时，Close 必须成功（因为 running
// 是 closeable 的）。如果有任何 Close 调用返回错误，说明 TOCTOU 把状态
// 改成了另一个 closeable 状态，而 Close 用过期的 from 调用 Transition 失败。
func TestLifecycleMachine_CloseFromRunningSucceedsUnderConcurrency(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	const iterations = 100
	var failures int64

	for i := 0; i < iterations; i++ {
		m := NewLifecycleMachine()
		_ = m.Start()

		stop, perturberWG := startPerturbers(m, 4, 100*time.Microsecond)

		// 并发 4 个 Close
		var closeWG sync.WaitGroup
		closeErrs := make([]error, 4)
		for c := 0; c < 4; c++ {
			closeWG.Add(1)
			go func(idx int) {
				defer closeWG.Done()
				closeErrs[idx] = m.Close("done")
			}(c)
		}
		closeWG.Wait()
		close(stop)
		perturberWG.Wait()

		final := m.Current()
		for _, e := range closeErrs {
			if e != nil {
				// 错误中不应提到任何可 closeable 的状态
				if _, ok := errMentionsCloseableState(e); ok {
					atomic.AddInt64(&failures, 1)
				}
			}
		}
		// 至少一个 Close 应该成功（state 应该是 closed）
		if final != LifecycleClosed {
			t.Fatalf("iter %d: expected final state closed, got %q (no Close succeeded)", i, final)
		}
	}

	if failures > 0 {
		t.Fatalf("Close failed with TOCTOU error %d times in %d iterations", failures, iterations)
	}
}
