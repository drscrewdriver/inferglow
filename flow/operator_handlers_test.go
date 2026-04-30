package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// ChunkHandler 测试
// ============================================================================

// Check: ChunkHandler.Kind() 返回 OpChunk
func TestChunkHandlerKind(t *testing.T) {
	h := &ChunkHandler{}
	if h.Kind() != OpChunk {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpChunk)
	}
}

// Check: ChunkHandler 透传输入
func TestChunkHandlerPassthrough(t *testing.T) {
	h := &ChunkHandler{}
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpChunk, Name: "my_chunk"},
		Input:    "test_input",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "test_input" {
		t.Errorf("output = %v, want test_input", out)
	}
}

// Check: ChunkHandler 通过 EmitSignal 发射 "Chunk[<name>]" 信号
func TestChunkHandlerEmitSignal(t *testing.T) {
	h := &ChunkHandler{}
	var emitted []Signal
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{ID: "op-1", Kind: OpChunk, Name: "my_chunk"},
		Input:    "payload",
		EmitSignal: func(s Signal) {
			emitted = append(emitted, s)
		},
	}
	_, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(emitted) != 1 {
		t.Fatalf("expected 1 signal emitted, got %d", len(emitted))
	}
	sig := emitted[0]
	if sig.TriggerEvent != "Chunk[my_chunk]" {
		t.Errorf("TriggerEvent = %q, want 'Chunk[my_chunk]'", sig.TriggerEvent)
	}
	if sig.Value != "payload" {
		t.Errorf("Value = %v, want payload", sig.Value)
	}
}

// Check: ChunkHandler 在 EmitSignal 为 nil 时不 panic
func TestChunkHandlerNilEmitSignal(t *testing.T) {
	h := &ChunkHandler{}
	oc := &OperatorContext{
		Ctx:        context.Background(),
		Operator:   &Operator{ID: "op-1", Kind: OpChunk, Name: "my_chunk"},
		Input:      "x",
		EmitSignal: nil,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "x" {
		t.Errorf("output = %v, want x", out)
	}
}

// ============================================================================
// SignalGateHandler 测试
// ============================================================================

// Check: SignalGateHandler.Kind() 返回 OpSignalGate
func TestSignalGateHandlerKind(t *testing.T) {
	h := &SignalGateHandler{}
	if h.Kind() != OpSignalGate {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpSignalGate)
	}
}

// Check: SignalGateHandler 全部 required signals 已接受时透传
func TestSignalGateHandlerAllAccepted(t *testing.T) {
	sn := NewSignalNet()
	sn.AcceptSignal(&Signal{ID: "sig_a", TriggerEvent: "A"})
	sn.AcceptSignal(&Signal{ID: "sig_b", TriggerEvent: "B"})

	h := &SignalGateHandler{}
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSignalGate,
			Name: "gate",
			Options: map[string]any{
				"required_signals": []any{"sig_a", "sig_b"},
			},
		},
		Input:     "passed_through",
		SignalNet: sn,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "passed_through" {
		t.Errorf("output = %v, want passed_through", out)
	}
}

// Check: SignalGateHandler 部分满足时返回 (nil, nil)
func TestSignalGateHandlerPartialAccepted(t *testing.T) {
	sn := NewSignalNet()
	sn.AcceptSignal(&Signal{ID: "sig_a", TriggerEvent: "A"})
	// sig_b 未接受

	h := &SignalGateHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSignalGate,
			Name: "gate",
			Options: map[string]any{
				"required_signals": []any{"sig_a", "sig_b"},
			},
		},
		Input:     "should_not_pass",
		SignalNet: sn,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute should not return error, got: %v", err)
	}
	if out != nil {
		t.Errorf("output = %v, want nil (gate not satisfied)", out)
	}
}

// Check: SignalGateHandler 全部未满足时返回 (nil, nil)
func TestSignalGateHandlerNoneAccepted(t *testing.T) {
	sn := NewSignalNet()

	h := &SignalGateHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSignalGate,
			Name: "gate",
			Options: map[string]any{
				"required_signals": []any{"sig_a", "sig_b"},
			},
		},
		SignalNet: sn,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute should not return error, got: %v", err)
	}
	if out != nil {
		t.Errorf("output = %v, want nil", out)
	}
}

// Check: SignalGateHandler 接受 []string 形式的 required_signals
func TestSignalGateHandlerStringSlice(t *testing.T) {
	sn := NewSignalNet()
	sn.AcceptSignal(&Signal{ID: "sig_a", TriggerEvent: "A"})

	h := &SignalGateHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSignalGate,
			Name: "gate",
			Options: map[string]any{
				"required_signals": []string{"sig_a"},
			},
		},
		Input:     "ok",
		SignalNet: sn,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %v, want ok", out)
	}
}

// ============================================================================
// BatchFanoutHandler 测试
// ============================================================================

// Check: BatchFanoutHandler.Kind() 返回 OpBatchFanout
func TestBatchFanoutHandlerKind(t *testing.T) {
	h := &BatchFanoutHandler{}
	if h.Kind() != OpBatchFanout {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpBatchFanout)
	}
}

// Check: BatchFanoutHandler N=3 fanout 并发处理
func TestBatchFanoutHandlerN3(t *testing.T) {
	h := &BatchFanoutHandler{}
	input := []any{1, 2, 3}
	processed := int64(0)
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"handler": func(v any) (any, error) {
					atomic.AddInt64(&processed, 1)
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
		t.Fatalf("expected []any output, got %T", out)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, want := range []int{10, 20, 30} {
		if results[i] != want {
			t.Errorf("results[%d] = %v, want %d", i, results[i], want)
		}
	}
	if atomic.LoadInt64(&processed) != 3 {
		t.Errorf("processed = %d, want 3", processed)
	}
}

// Check: BatchFanoutHandler 无 handler 时使用透传函数
func TestBatchFanoutHandlerPassthrough(t *testing.T) {
	h := &BatchFanoutHandler{}
	input := []any{"a", "b"}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpBatchFanout,
			Name:    "fanout",
			Options: map[string]any{},
		},
		Input: input,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results := out.([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0] != "a" || results[1] != "b" {
		t.Errorf("results = %v, want [a b]", results)
	}
}

// Check: BatchFanoutHandler 为每个结果发射 BatchItem[i] 信号
func TestBatchFanoutHandlerEmitSignals(t *testing.T) {
	h := &BatchFanoutHandler{}
	var mu sync.Mutex
	var emitted []Signal
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
		},
		Input: []any{10, 20, 30},
		EmitSignal: func(s Signal) {
			mu.Lock()
			defer mu.Unlock()
			emitted = append(emitted, s)
		},
	}
	_, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(emitted) != 3 {
		t.Fatalf("expected 3 emitted signals, got %d", len(emitted))
	}
	// 检查所有 BatchItem 信号都存在
	found := map[string]bool{}
	for _, s := range emitted {
		found[s.TriggerEvent] = true
	}
	for i := 0; i < 3; i++ {
		key := "BatchItem[" + intToStr(i) + "]"
		if !found[key] {
			t.Errorf("expected signal %q to be emitted", key)
		}
	}
}

// Check: BatchFanoutHandler 输入类型错误返回 error
func TestBatchFanoutHandlerBadInput(t *testing.T) {
	h := &BatchFanoutHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
		},
		Input: "not_a_slice",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for non-slice input")
	}
}

// Check: BatchFanoutHandler 任意一项失败则整体失败
func TestBatchFanoutHandlerItemError(t *testing.T) {
	h := &BatchFanoutHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"handler": func(v any) (any, error) {
					if v.(int) == 2 {
						return nil, errors.New("item 2 failed")
					}
					return v, nil
				},
			},
		},
		Input: []any{1, 2, 3},
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for failed item")
	}
	if !strings.Contains(err.Error(), "item 2 failed") {
		t.Errorf("error %q should contain 'item 2 failed'", err.Error())
	}
}

// Check: BatchFanoutHandler 并发数限制为 min(N, 8)
func TestBatchFanoutHandlerConcurrencyLimit(t *testing.T) {
	h := &BatchFanoutHandler{}
	// 10 个元素，并发数应为 8
	input := make([]any, 10)
	for i := range input {
		input[i] = i
	}

	var current, max ConcurrentCounter
	var mu sync.Mutex
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
			Options: map[string]any{
				"handler": func(v any) (any, error) {
					mu.Lock()
					current++
					if current > max {
						max = current
					}
					mu.Unlock()
					time.Sleep(5 * time.Millisecond)
					mu.Lock()
					current--
					mu.Unlock()
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
	if max > 8 {
		t.Errorf("max concurrency = %d, should be <= 8", max)
	}
}

// ConcurrentCounter 是 int64 的别名，便于阅读
type ConcurrentCounter = int64

// Check: BatchFanoutHandler 空切片返回空切片
func TestBatchFanoutHandlerEmptySlice(t *testing.T) {
	h := &BatchFanoutHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchFanout,
			Name: "fanout",
		},
		Input: []any{},
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %v", results)
	}
}

// intToStr 测试中避免引入 strconv
func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ============================================================================
// BatchCollectHandler 测试
// ============================================================================

// Check: BatchCollectHandler.Kind() 返回 OpBatchCollect
func TestBatchCollectHandlerKind(t *testing.T) {
	h := &BatchCollectHandler{}
	if h.Kind() != OpBatchCollect {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpBatchCollect)
	}
}

// Check: BatchCollectHandler 等待 3 个信号并按 index 顺序合并
func TestBatchCollectHandlerMerge3(t *testing.T) {
	sn := NewSignalNet()

	h := &BatchCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchCollect,
			Name: "collect",
			Options: map[string]any{
				"expected_count": 3,
			},
		},
		SignalNet: sn,
	}

	// 在另一个 goroutine 中异步接受信号，模拟信号陆续到达
	go func() {
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[0]", TriggerEvent: "BatchItem[0]", Value: "result_0"})
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[1]", TriggerEvent: "BatchItem[1]", Value: "result_1"})
		time.Sleep(5 * time.Millisecond)
		sn.AcceptSignal(&Signal{ID: "BatchItem[2]", TriggerEvent: "BatchItem[2]", Value: "result_2"})
	}()

	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	want := []string{"result_0", "result_1", "result_2"}
	for i, w := range want {
		if results[i] != w {
			t.Errorf("results[%d] = %v, want %s", i, results[i], w)
		}
	}
}

// Check: BatchCollectHandler 信号已经全部就绪时立即返回
func TestBatchCollectHandlerAlreadyReady(t *testing.T) {
	sn := NewSignalNet()
	sn.AcceptSignal(&Signal{ID: "BatchItem[0]", TriggerEvent: "BatchItem[0]", Value: 100})
	sn.AcceptSignal(&Signal{ID: "BatchItem[1]", TriggerEvent: "BatchItem[1]", Value: 200})

	h := &BatchCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchCollect,
			Name: "collect",
			Options: map[string]any{
				"expected_count": 2,
			},
		},
		SignalNet: sn,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results := out.([]any)
	if results[0] != 100 || results[1] != 200 {
		t.Errorf("results = %v, want [100 200]", results)
	}
}

// Check: BatchCollectHandler expected_count 缺失返回 error
func TestBatchCollectHandlerMissingExpectedCount(t *testing.T) {
	h := &BatchCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpBatchCollect,
			Name:    "collect",
			Options: map[string]any{},
		},
		SignalNet: NewSignalNet(),
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for missing expected_count")
	}
}

// Check: BatchCollectHandler expected_count=0 返回空切片
func TestBatchCollectHandlerZeroCount(t *testing.T) {
	h := &BatchCollectHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpBatchCollect,
			Name: "collect",
			Options: map[string]any{
				"expected_count": 0,
			},
		},
		SignalNet: NewSignalNet(),
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	results, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %v", results)
	}
}

// ============================================================================
// MatchRouteHandler 测试
// ============================================================================

// Check: MatchRouteHandler.Kind() 返回 OpMatchRoute
func TestMatchRouteHandlerKind(t *testing.T) {
	h := &MatchRouteHandler{}
	if h.Kind() != OpMatchRoute {
		t.Errorf("Kind = %q, want %q", h.Kind(), OpMatchRoute)
	}
}

// Check: MatchRouteHandler 命中 case
func TestMatchRouteHandlerHitCase(t *testing.T) {
	h := &MatchRouteHandler{}
	matcher := func(in any) string {
		return in.(string)
	}
	cases := map[string]Handler{
		"hello": func(rd *TriggerFlowRuntimeData) (any, error) {
			return "matched_hello", nil
		},
		"world": func(rd *TriggerFlowRuntimeData) (any, error) {
			return "matched_world", nil
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"matcher": matcher,
				"cases":   cases,
			},
		},
		Input: "hello",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "matched_hello" {
		t.Errorf("output = %v, want matched_hello", out)
	}
}

// Check: MatchRouteHandler 未命中 case 走 default
func TestMatchRouteHandlerMissWithDefault(t *testing.T) {
	h := &MatchRouteHandler{}
	matcher := func(in any) string { return in.(string) }
	cases := map[string]Handler{
		"hello": func(rd *TriggerFlowRuntimeData) (any, error) {
			return "matched_hello", nil
		},
	}
	// 注意：存入 Options[any] 时必须显式转换为 Handler 类型，否则类型断言失败
	var defaultHandler Handler = func(rd *TriggerFlowRuntimeData) (any, error) {
		return "default_called", nil
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"matcher": matcher,
				"cases":   cases,
				"default": defaultHandler,
			},
		},
		Input: "unknown",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "default_called" {
		t.Errorf("output = %v, want default_called", out)
	}
}

// Check: MatchRouteHandler 未命中且无 default 返回 error
func TestMatchRouteHandlerMissNoDefault(t *testing.T) {
	h := &MatchRouteHandler{}
	matcher := func(in any) string { return in.(string) }
	cases := map[string]Handler{
		"hello": func(rd *TriggerFlowRuntimeData) (any, error) {
			return "matched_hello", nil
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"matcher": matcher,
				"cases":   cases,
			},
		},
		Input: "unknown",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for unmatched case without default")
	}
}

// Check: MatchRouteHandler 缺少 matcher 返回 error
func TestMatchRouteHandlerMissingMatcher(t *testing.T) {
	h := &MatchRouteHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"cases": map[string]Handler{},
			},
		},
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for missing matcher")
	}
}

// Check: MatchRouteHandler 缺少 cases 返回 error
func TestMatchRouteHandlerMissingCases(t *testing.T) {
	h := &MatchRouteHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"matcher": func(in any) string { return "" },
			},
		},
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error for missing cases")
	}
}

// Check: MatchRouteHandler 传递 TriggerFlowRuntimeData 给 case handler
func TestMatchRouteHandlerRuntimeData(t *testing.T) {
	h := &MatchRouteHandler{}
	matcher := func(in any) string { return "case1" }

	var captured *TriggerFlowRuntimeData
	cases := map[string]Handler{
		"case1": func(rd *TriggerFlowRuntimeData) (any, error) {
			captured = rd
			return "ok", nil
		},
	}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpMatchRoute,
			Name: "route",
			Options: map[string]any{
				"matcher": matcher,
				"cases":   cases,
			},
		},
		Input: "payload",
	}
	_, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if captured == nil {
		t.Fatal("expected TriggerFlowRuntimeData to be captured")
	}
	if captured.Signal == nil {
		t.Fatal("expected Signal to be set")
	}
	if captured.Signal.Value != "payload" {
		t.Errorf("Signal.Value = %v, want payload", captured.Signal.Value)
	}
	if captured.Result != "payload" {
		t.Errorf("Result = %v, want payload", captured.Result)
	}
}
