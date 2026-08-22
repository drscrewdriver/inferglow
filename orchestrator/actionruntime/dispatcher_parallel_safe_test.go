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
	"sync"
	"testing"
	"time"

	"github.com/inferglow/action"
)

// ---------- 测试辅助：执行区间记录 ----------

// execSpan 记录一次 Action 执行的时间区间 [start, end]，用于时序断言。
type execSpan struct {
	start time.Time
	end   time.Time
}

// spanRecorder 并发安全地按 action 名字记录执行区间。
// dispatcher 会并发执行调用，因此记录必须加锁。
type spanRecorder struct {
	mu    sync.Mutex
	spans map[string]execSpan
}

func newSpanRecorder() *spanRecorder {
	return &spanRecorder{spans: make(map[string]execSpan)}
}

func (r *spanRecorder) record(name string, start, end time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans[name] = execSpan{start: start, end: end}
}

func (r *spanRecorder) get(name string) execSpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spans[name]
}

// overlaps 判断两个执行区间是否在时间上重叠。
func overlaps(a, b execSpan) bool {
	return a.start.Before(b.end) && b.start.Before(a.end)
}

// boolPtr 返回指向 b 的指针，便于构造 *bool 字段。
func boolPtr(b bool) *bool { return &b }

// mustRegister 批量注册 Action，失败即终止测试。
func mustRegister(t *testing.T, r *action.ActionRegistry, actions ...*action.Action) {
	t.Helper()
	for _, a := range actions {
		if err := r.Register(a); err != nil {
			t.Fatalf("注册 Action %q 失败: %v", a.Name, err)
		}
	}
}

// ---------- 测试辅助：fake executor ----------

// timedExecutor 是带固定 sleep 的 fake executor：
// 睡眠 sleep 时长，把执行区间记录进 recorder，并以自己的名字作为成功结果返回，
// 从而可以同时断言「执行时序」与「结果按调用位置对应」。
type timedExecutor struct {
	name     string
	sleep    time.Duration
	recorder *spanRecorder
}

func (e *timedExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	start := time.Now()
	time.Sleep(e.sleep)
	e.recorder.record(e.name, start, time.Now())
	return &action.ActionResult{OK: true, Status: "success", Result: e.name}, nil
}

// ctxAwareSlowExecutor 模拟尊重 ctx 的慢工具：
// 要么在 dur 后成功返回，要么在 ctx 取消时立刻返回错误，
// 用于验证 preempt 取消语义。
type ctxAwareSlowExecutor struct {
	name     string
	dur      time.Duration
	recorder *spanRecorder
}

func (e *ctxAwareSlowExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	start := time.Now()
	defer func() { e.recorder.record(e.name, start, time.Now()) }()
	select {
	case <-ctx.Done():
		return &action.ActionResult{OK: false, Status: "error", Error: ctx.Err().Error()}, nil
	case <-time.After(e.dur):
		return &action.ActionResult{OK: true, Status: "success", Result: e.name}, nil
	}
}

// newTimedAction 构造一个携带 Spec 声明与 timedExecutor 的 Action。
// spec 为 nil 表示不携带规格（应按默认并行安全处理）。
func newTimedAction(name string, sleep time.Duration, recorder *spanRecorder, spec *action.ActionSpec) *action.Action {
	return &action.Action{
		Name:        name,
		Description: "test action " + name,
		Executor:    &timedExecutor{name: name, sleep: sleep, recorder: recorder},
		Spec:        spec,
	}
}

// newCtxAwareAction 构造一个携带 Spec 声明与 ctxAwareSlowExecutor 的 Action。
func newCtxAwareAction(name string, dur time.Duration, recorder *spanRecorder, spec *action.ActionSpec) *action.Action {
	return &action.Action{
		Name:        name,
		Description: "test action " + name,
		Executor:    &ctxAwareSlowExecutor{name: name, dur: dur, recorder: recorder},
		Spec:        spec,
	}
}

// ---------- Execute：按 ParallelSafe 声明分流 ----------

// TestDispatcher_ExecuteParallelSafeRouting 验证同轮混合调用的分流（R5）：
// 两个 ParallelSafe=false 的调用按调用顺序串行执行（区间不重叠且先后有序），
// 两个默认调用的执行区间重叠（并发），结果按原始调用位置一一对应返回。
func TestDispatcher_ExecuteParallelSafeRouting(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()
	recorder := newSpanRecorder()

	const sleep = 60 * time.Millisecond
	serialSpec := &action.ActionSpec{ParallelSafe: boolPtr(false)}
	// 调用顺序故意交错：串行(0) 并行(1) 串行(2) 并行(3)，
	// 验证分组内仍保持原始相对顺序。
	mustRegister(t, registry,
		newTimedAction("serial_a", sleep, recorder, serialSpec),
		newTimedAction("par_a", sleep, recorder, nil),
		newTimedAction("serial_b", sleep, recorder, serialSpec),
		newTimedAction("par_b", sleep, recorder, nil),
	)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "serial_a"},
		{Name: "par_a"},
		{Name: "serial_b"},
		{Name: "par_b"},
	}
	results := dispatcher.Execute(ctx, calls)

	if len(results) != len(calls) {
		t.Fatalf("期望 %d 个结果，实际 %d", len(calls), len(results))
	}
	// 结果必须按原始调用位置一一对应：每个 executor 返回自己的名字。
	for i, c := range calls {
		if results[i] == nil || !results[i].OK {
			t.Fatalf("results[%d] 应为成功结果，实际 %+v", i, results[i])
		}
		if got, _ := results[i].Result.(string); got != c.Name {
			t.Errorf("results[%d].Result = %v，期望 %q（结果位置与调用顺序不对应）", i, results[i].Result, c.Name)
		}
	}

	sa := recorder.get("serial_a")
	sb := recorder.get("serial_b")
	pa := recorder.get("par_a")
	pb := recorder.get("par_b")

	// 默认调用应并发执行：区间重叠。
	if !overlaps(pa, pb) {
		t.Errorf("默认并行安全的两个调用应并发执行（区间重叠）：par_a=[%v,%v] par_b=[%v,%v]",
			pa.start, pa.end, pb.start, pb.end)
	}
	// 声明串行的两个调用：区间不重叠。
	if overlaps(sa, sb) {
		t.Errorf("ParallelSafe=false 的两个调用不应重叠执行：serial_a=[%v,%v] serial_b=[%v,%v]",
			sa.start, sa.end, sb.start, sb.end)
	}
	// 且按调用顺序：serial_a（调用位置 0）先于 serial_b（调用位置 2）。
	if sa.end.After(sb.start) {
		t.Errorf("串行调用应按调用顺序执行：serial_a 结束于 %v，但 serial_b 开始于 %v", sa.end, sb.start)
	}
}

// TestDispatcher_ExecuteAllDefaultStillConcurrent 验证全部默认声明时行为与现状一致：
// 未携带 Spec、ParallelSafe 零值（nil）、显式 true 三种情况均并发执行。
func TestDispatcher_ExecuteAllDefaultStillConcurrent(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()
	recorder := newSpanRecorder()

	const sleep = 60 * time.Millisecond
	mustRegister(t, registry,
		newTimedAction("act_no_spec", sleep, recorder, nil),
		newTimedAction("act_zero_spec", sleep, recorder, &action.ActionSpec{}),
		newTimedAction("act_true_spec", sleep, recorder, &action.ActionSpec{ParallelSafe: boolPtr(true)}),
	)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "act_no_spec"},
		{Name: "act_zero_spec"},
		{Name: "act_true_spec"},
	}
	start := time.Now()
	results := dispatcher.Execute(ctx, calls)
	elapsed := time.Since(start)

	for i, c := range calls {
		if results[i] == nil || !results[i].OK {
			t.Fatalf("results[%d] 应为成功结果，实际 %+v", i, results[i])
		}
		if got, _ := results[i].Result.(string); got != c.Name {
			t.Errorf("results[%d].Result = %v，期望 %q", i, results[i].Result, c.Name)
		}
	}
	// 三个各睡 60ms 的调用若串行至少 180ms；并发应远小于该值。
	if elapsed >= 3*sleep {
		t.Errorf("全部默认声明应并发执行（<%v），实际耗时 %v", 3*sleep, elapsed)
	}
	// ActionSpec.ParallelSafe 零值（nil）与未携带 Spec 的调用区间应重叠。
	if !overlaps(recorder.get("act_no_spec"), recorder.get("act_zero_spec")) {
		t.Errorf("ParallelSafe 零值（nil）应视为并行安全（区间应重叠）")
	}
}

// ---------- ExecuteInterruptible：同样的分流 + preempt 语义保持 ----------

// TestDispatcher_ExecuteInterruptibleParallelSafeRouting 验证 ExecuteInterruptible
// 同样按 ParallelSafe 声明分流：串行组按调用顺序不重叠、默认组并发重叠、
// 结果按原始位置返回，且未触发 preempt 时返回 preempted=false。
func TestDispatcher_ExecuteInterruptibleParallelSafeRouting(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()
	recorder := newSpanRecorder()

	const sleep = 60 * time.Millisecond
	serialSpec := &action.ActionSpec{ParallelSafe: boolPtr(false)}
	mustRegister(t, registry,
		newTimedAction("serial_a", sleep, recorder, serialSpec),
		newTimedAction("par_a", sleep, recorder, nil),
		newTimedAction("serial_b", sleep, recorder, serialSpec),
		newTimedAction("par_b", sleep, recorder, nil),
	)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "serial_a"},
		{Name: "par_a"},
		{Name: "serial_b"},
		{Name: "par_b"},
	}
	preemptCh := make(chan struct{}) // 永不关闭
	results, preempted := dispatcher.ExecuteInterruptible(ctx, calls, preemptCh)

	if preempted {
		t.Fatal("未触发 preempt 时 preempted 应为 false")
	}
	if len(results) != len(calls) {
		t.Fatalf("期望 %d 个结果，实际 %d", len(calls), len(results))
	}
	for i, c := range calls {
		if results[i] == nil || !results[i].OK {
			t.Fatalf("results[%d] 应为成功结果，实际 %+v", i, results[i])
		}
		if got, _ := results[i].Result.(string); got != c.Name {
			t.Errorf("results[%d].Result = %v，期望 %q（结果位置与调用顺序不对应）", i, results[i].Result, c.Name)
		}
	}

	sa := recorder.get("serial_a")
	sb := recorder.get("serial_b")
	pa := recorder.get("par_a")
	pb := recorder.get("par_b")

	if !overlaps(pa, pb) {
		t.Errorf("默认并行安全的两个调用应并发执行（区间重叠）：par_a=[%v,%v] par_b=[%v,%v]",
			pa.start, pa.end, pb.start, pb.end)
	}
	if overlaps(sa, sb) {
		t.Errorf("ParallelSafe=false 的两个调用不应重叠执行：serial_a=[%v,%v] serial_b=[%v,%v]",
			sa.start, sa.end, sb.start, sb.end)
	}
	if sa.end.After(sb.start) {
		t.Errorf("串行调用应按调用顺序执行：serial_a 结束于 %v，但 serial_b 开始于 %v", sa.end, sb.start)
	}
}

// TestDispatcher_ExecuteInterruptiblePreemptCancelsDefault 验证加入分流后
// preempt 通道语义保持不变：关闭 preemptCh 后 ctx 被取消，
// 尚未完成的默认（并行安全）调用以 ctx 感知方式快速返回错误结果，preempted=true。
func TestDispatcher_ExecuteInterruptiblePreemptCancelsDefault(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()
	recorder := newSpanRecorder()

	mustRegister(t, registry,
		newCtxAwareAction("slow_1", 5*time.Second, recorder, nil),
		newCtxAwareAction("slow_2", 5*time.Second, recorder, nil),
	)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "slow_1"},
		{Name: "slow_2"},
	}
	preemptCh := make(chan struct{})
	go func() {
		time.Sleep(60 * time.Millisecond)
		close(preemptCh)
	}()

	start := time.Now()
	results, preempted := dispatcher.ExecuteInterruptible(ctx, calls, preemptCh)
	elapsed := time.Since(start)

	if !preempted {
		t.Fatal("关闭 preemptCh 后 preempted 应为 true")
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("preempt 后应快速返回（ctx 取消），实际耗时 %v", elapsed)
	}
	for i, r := range results {
		if r == nil {
			t.Fatalf("results[%d] 不应为 nil", i)
		}
		if r.OK {
			t.Errorf("results[%d] 被 preempt 取消应为错误结果，实际 %+v", i, r)
		}
	}
}

// TestDispatcher_ExecuteInterruptiblePreemptCancelsSerialChain 验证 preempt 发生在
// 串行链执行中途时的语义：正在执行的串行调用被取消；后续串行调用以已取消的
// ctx 立即返回错误结果（所有结果槽位均被写入，不为 nil），preempted=true，
// 且串行链顺序仍保持。
func TestDispatcher_ExecuteInterruptiblePreemptCancelsSerialChain(t *testing.T) {
	ctx := context.Background()
	registry := action.NewRegistry()
	recorder := newSpanRecorder()

	serialSpec := &action.ActionSpec{ParallelSafe: boolPtr(false)}
	mustRegister(t, registry,
		newCtxAwareAction("serial_slow_1", 5*time.Second, recorder, serialSpec),
		newTimedAction("fast_default", 20*time.Millisecond, recorder, nil),
		newCtxAwareAction("serial_slow_2", 5*time.Second, recorder, serialSpec),
	)

	dispatcher := NewActionDispatcher(registry)
	calls := []ActionCall{
		{Name: "serial_slow_1"},
		{Name: "fast_default"},
		{Name: "serial_slow_2"},
	}
	preemptCh := make(chan struct{})
	go func() {
		time.Sleep(80 * time.Millisecond)
		close(preemptCh)
	}()

	start := time.Now()
	results, preempted := dispatcher.ExecuteInterruptible(ctx, calls, preemptCh)
	elapsed := time.Since(start)

	if !preempted {
		t.Fatal("关闭 preemptCh 后 preempted 应为 true")
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("preempt 后应快速返回，实际耗时 %v", elapsed)
	}
	// 串行链顺序保持：serial_slow_2 在 serial_slow_1 结束后才开始。
	s1 := recorder.get("serial_slow_1")
	s2 := recorder.get("serial_slow_2")
	if s1.end.After(s2.start) {
		t.Errorf("串行链在 preempt 下仍应按调用顺序执行：s1 结束 %v，s2 开始 %v", s1.end, s2.start)
	}
	// 所有槽位都必须有结果（不能因为取消留下 nil）。
	for i, r := range results {
		if r == nil {
			t.Fatalf("results[%d] 不应为 nil（取消后仍需写入结果）", i)
		}
	}
	// 执行中被取消的串行调用 → 错误结果。
	if results[0].OK {
		t.Errorf("results[0]（执行中被取消的串行调用）应为错误结果，实际 %+v", results[0])
	}
	// 抢占发生后才轮到的串行调用：拿到已取消的 ctx，立即返回错误。
	if results[2].OK {
		t.Errorf("results[2]（抢占后才执行的串行调用）应为错误结果，实际 %+v", results[2])
	}
}
