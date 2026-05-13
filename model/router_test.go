package model

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// === PricingInfo ===

func TestPricingInfoEffectivePerMillion(t *testing.T) {
	cases := []struct {
		name string
		p    PricingInfo
		want float64
	}{
		{"reasoning=0 uses completion", PricingInfo{PromptPerMillion: 1, CompletionPerMillion: 5, ReasoningPerMillion: 0}, 5},
		{"reasoning<completion uses completion", PricingInfo{PromptPerMillion: 1, CompletionPerMillion: 10, ReasoningPerMillion: 3}, 10},
		{"reasoning>completion uses reasoning", PricingInfo{PromptPerMillion: 1, CompletionPerMillion: 5, ReasoningPerMillion: 15}, 15},
		{"all zero", PricingInfo{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.EffectivePerMillion(); got != tc.want {
				t.Errorf("EffectivePerMillion = %v, want %v", got, tc.want)
			}
		})
	}
}

// === Router 创建与配置 ===

func TestNewRouter(t *testing.T) {
	pool := NewModelPool()
	r := NewRouter(pool, RoutingFirst)
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if r.Policy() != RoutingFirst {
		t.Errorf("Policy = %v, want RoutingFirst", r.Policy())
	}
}

func TestRouterSetPolicy(t *testing.T) {
	pool := NewModelPool()
	r := NewRouter(pool, RoutingFirst)
	r.SetPolicy(RoutingLatency)
	if r.Policy() != RoutingLatency {
		t.Errorf("Policy = %v, want RoutingLatency", r.Policy())
	}
}

func TestRouterSetPricing(t *testing.T) {
	pool := NewModelPool()
	r := NewRouter(pool, RoutingCost)
	r.SetPricing("alpha", PricingInfo{PromptPerMillion: 1, CompletionPerMillion: 5})

	got := r.Pricing("alpha")
	if got.CompletionPerMillion != 5 {
		t.Errorf("Pricing.CompletionPerMillion = %v, want 5", got.CompletionPerMillion)
	}
}

func TestRouterPricingMissing(t *testing.T) {
	pool := NewModelPool()
	r := NewRouter(pool, RoutingCost)
	got := r.Pricing("nope")
	if got.CompletionPerMillion != 0 {
		t.Errorf("expected zero-value PricingInfo for missing provider")
	}
}

// === Select: RoutingFirst ===

func TestRouterSelectFirst(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})

	r := NewRouter(pool, RoutingFirst)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "p1" {
		t.Errorf("expected p1 (first in fallback), got %q", got.Name())
	}
}

func TestRouterSelectEmptyPool(t *testing.T) {
	pool := NewModelPool()
	r := NewRouter(pool, RoutingFirst)
	_, err := r.Select(context.Background())
	if !errors.Is(err, ErrEmptyPool) {
		t.Errorf("expected ErrEmptyPool, got %v", err)
	}
}

func TestRouterSelectNilPool(t *testing.T) {
	r := &Router{pool: nil, pricing: map[string]PricingInfo{}}
	_, err := r.Select(context.Background())
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestRouterSelectContextCancelled(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p"))
	r := NewRouter(pool, RoutingFirst)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Select(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// === Select: RoutingRandom ===

func TestRouterSelectRandom(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	_ = pool.Register(newMockProvider("p3"))

	r := NewRouter(pool, RoutingRandom)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil provider")
	}
	// 不能断言具体名字，但应该在已注册列表中
	names := pool.List()
	found := false
	for _, n := range names {
		if n == got.Name() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("selected provider %q not in pool list %v", got.Name(), names)
	}
}

// === Select: RoutingCost ===

func TestRouterSelectCost(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("cheap"))
	_ = pool.Register(newMockProvider("mid"))
	_ = pool.Register(newMockProvider("expensive"))

	r := NewRouter(pool, RoutingCost)
	r.SetPricing("cheap", PricingInfo{CompletionPerMillion: 1})
	r.SetPricing("mid", PricingInfo{CompletionPerMillion: 5})
	r.SetPricing("expensive", PricingInfo{CompletionPerMillion: 20})

	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "cheap" {
		t.Errorf("expected cheap, got %q", got.Name())
	}
}

// 未配置 pricing 的 Provider 视为最贵（不被选中，除非全部未配置）。
func TestRouterSelectCostMixedPricing(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("priced"))
	_ = pool.Register(newMockProvider("unpriced"))

	r := NewRouter(pool, RoutingCost)
	r.SetPricing("priced", PricingInfo{CompletionPerMillion: 100})

	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "priced" {
		t.Errorf("expected priced provider, got %q", got.Name())
	}
}

// reasoning 单价高于 completion 时，EffectivePerMillion 取 reasoning。
func TestRouterSelectCostWithReasoning(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("a"))
	_ = pool.Register(newMockProvider("b"))

	r := NewRouter(pool, RoutingCost)
	// a: completion=10, reasoning=50 → effective=50
	r.SetPricing("a", PricingInfo{CompletionPerMillion: 10, ReasoningPerMillion: 50})
	// b: completion=20, reasoning=0 → effective=20
	r.SetPricing("b", PricingInfo{CompletionPerMillion: 20})

	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "b" {
		t.Errorf("expected b (lower effective), got %q", got.Name())
	}
}

// === Select: RoutingLatency ===

func TestRouterSelectLatency(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("fast"))
	_ = pool.Register(newMockProvider("slow"))

	// 注入历史指标
	pool.RecordResult("fast", true, 1_000_000)  // 1ms
	pool.RecordResult("slow", true, 10_000_000) // 10ms

	r := NewRouter(pool, RoutingLatency)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "fast" {
		t.Errorf("expected fast, got %q", got.Name())
	}
}

// 所有 Provider 都无历史数据时回退到 RoutingFirst（走 fallback 链）。
func TestRouterSelectLatencyNoDataFallback(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})

	r := NewRouter(pool, RoutingLatency)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "p1" {
		t.Errorf("expected p1 (fallback), got %q", got.Name())
	}
}

// 有部分 Provider 有数据时，应优先选有数据中延迟最低的。
func TestRouterSelectLatencyPartialData(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("hasdata"))
	_ = pool.Register(newMockProvider("nodata"))
	pool.SetFallback([]string{"nodata", "hasdata"}) // 故意把 nodata 放前面

	pool.RecordResult("hasdata", true, 2_000_000) // 2ms

	r := NewRouter(pool, RoutingLatency)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "hasdata" {
		t.Errorf("expected hasdata (only one with latency data), got %q", got.Name())
	}
}

// === Select: RoutingQuality ===

func TestRouterSelectQuality(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("good"))
	_ = pool.Register(newMockProvider("bad"))

	// good: 9 成功 + 1 失败 → 0.9
	for i := 0; i < 9; i++ {
		pool.RecordResult("good", true, 1_000_000)
	}
	pool.RecordResult("good", false, 1_000_000)
	// bad: 1 成功 + 9 失败 → 0.1
	pool.RecordResult("bad", true, 1_000_000)
	for i := 0; i < 9; i++ {
		pool.RecordResult("bad", false, 1_000_000)
	}

	r := NewRouter(pool, RoutingQuality)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "good" {
		t.Errorf("expected good, got %q", got.Name())
	}
}

// 所有 Provider 都无历史数据时回退到 RoutingFirst。
func TestRouterSelectQualityNoDataFallback(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})

	r := NewRouter(pool, RoutingQuality)
	got, err := r.Select(context.Background())
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if got.Name() != "p1" {
		t.Errorf("expected p1 (fallback), got %q", got.Name())
	}
}

// === ExecuteSelect ===

func TestRouterExecuteSelectSuccess(t *testing.T) {
	pool := NewModelPool()
	mp := newMockProvider("p")
	_ = pool.Register(mp)
	pool.SetFallback([]string{"p"})

	r := NewRouter(pool, RoutingFirst)

	called := false
	err := r.ExecuteSelect(context.Background(), func(provider ModelRequester) error {
		called = true
		if provider.Name() != "p" {
			t.Errorf("expected p, got %q", provider.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteSelect failed: %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}

	// 应记录 1 次成功
	m := pool.Metrics("p")
	if m.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", m.SuccessCount)
	}
}

func TestRouterExecuteSelectFailure(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p"))
	pool.SetFallback([]string{"p"})

	r := NewRouter(pool, RoutingFirst)

	wantErr := errors.New("request failed")
	err := r.ExecuteSelect(context.Background(), func(provider ModelRequester) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}

	// 应记录 1 次失败
	m := pool.Metrics("p")
	if m.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", m.FailureCount)
	}
}

// Select 阶段失败时不应调用 fn。
func TestRouterExecuteSelectSelectFails(t *testing.T) {
	pool := NewModelPool() // 空 pool
	r := NewRouter(pool, RoutingFirst)

	called := false
	err := r.ExecuteSelect(context.Background(), func(provider ModelRequester) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error from empty pool")
	}
	if called {
		t.Error("fn should not be called when Select fails")
	}
}

// === 策略切换 ===

// 运行时切换策略应立即生效。
func TestRouterSwitchPolicyAtRuntime(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})
	pool.RecordResult("p2", true, 1_000_000) // p2 有更好的延迟数据

	r := NewRouter(pool, RoutingFirst)

	// RoutingFirst 应选 p1
	got, _ := r.Select(context.Background())
	if got.Name() != "p1" {
		t.Errorf("RoutingFirst: expected p1, got %q", got.Name())
	}

	// 切换到 RoutingLatency 应选 p2
	r.SetPolicy(RoutingLatency)
	got, _ = r.Select(context.Background())
	if got.Name() != "p2" {
		t.Errorf("RoutingLatency: expected p2, got %q", got.Name())
	}
}

// === 并发安全 ===

// 并发 Select + SetPolicy 不应 panic。
func TestRouterConcurrentSelectAndSetPolicy(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})

	r := NewRouter(pool, RoutingFirst)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = r.Select(context.Background())
		}
	}()
	go func() {
		defer wg.Done()
		policies := []RoutingPolicy{RoutingFirst, RoutingRandom, RoutingLatency, RoutingQuality}
		for i := 0; i < 50; i++ {
			r.SetPolicy(policies[i%len(policies)])
		}
	}()
	wg.Wait()
}

// 并发 ExecuteSelect 不应导致指标竞争错误。
func TestRouterConcurrentExecuteSelect(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p"))
	pool.SetFallback([]string{"p"})

	r := NewRouter(pool, RoutingFirst)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	var success int32
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			err := r.ExecuteSelect(context.Background(), func(provider ModelRequester) error {
				return nil
			})
			if err == nil {
				atomic.AddInt32(&success, 1)
			}
		}()
	}
	wg.Wait()

	if success != N {
		t.Errorf("success count = %d, want %d", success, N)
	}
	m := pool.Metrics("p")
	if m.SuccessCount != int64(N) {
		t.Errorf("SuccessCount = %d, want %d", m.SuccessCount, N)
	}
}
