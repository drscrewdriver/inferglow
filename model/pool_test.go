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

package model

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider 是用于测试的 ModelRequester 实现。
// 通过 controlling 字段可控制 RequestModel 是否返回错误，模拟失败场景。
type mockProvider struct {
	name     string
	failRate int32 // 0=永不失败；>0 时 RequestModel 以该概率失败（这里简化为 >0 即失败）
	calls    int32
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	return &RequestData{Model: m.name, Messages: []ChatMessage{{Role: "user", Content: "test"}}}, nil
}
func (m *mockProvider) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	atomic.AddInt32(&m.calls, 1)
	if atomic.LoadInt32(&m.failRate) > 0 {
		return nil, errors.New("mock provider failure")
	}
	ch := make(chan *StreamChunk, 1)
	ch <- &StreamChunk{Delta: "ok", IsDone: true}
	close(ch)
	return ch, nil
}
func (m *mockProvider) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	events := make(chan *ResultEvent, 1)
	events <- &ResultEvent{EventType: EventDone}
	close(events)
	return events, nil
}

func (m *mockProvider) CallCount() int32 { return atomic.LoadInt32(&m.calls) }

// newMockProvider 创建一个名为 name 的 mock Provider。
func newMockProvider(name string) *mockProvider {
	return &mockProvider{name: name}
}

// === Register / Get / List ===

func TestModelPoolRegisterAndGet(t *testing.T) {
	pool := NewModelPool()
	p := newMockProvider("alpha")
	if err := pool.Register(p); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := pool.Get("alpha")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name() != "alpha" {
		t.Errorf("got name = %q, want %q", got.Name(), "alpha")
	}
}

func TestModelPoolRegisterDuplicate(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("dup"))
	err := pool.Register(newMockProvider("dup"))
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestModelPoolRegisterNil(t *testing.T) {
	pool := NewModelPool()
	if err := pool.Register(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestModelPoolGetMissing(t *testing.T) {
	pool := NewModelPool()
	_, err := pool.Get("missing")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestModelPoolList(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("a"))
	_ = pool.Register(newMockProvider("b"))
	_ = pool.Register(newMockProvider("c"))

	list := pool.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(list))
	}
}

// === Fallback chain ===

func TestModelPoolSetFallback(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("primary"))
	_ = pool.Register(newMockProvider("secondary"))

	pool.SetFallback([]string{"primary", "secondary"})

	chain := pool.FallbackChain()
	if len(chain) != 2 {
		t.Fatalf("expected fallback chain length 2, got %d", len(chain))
	}
	if chain[0] != "primary" || chain[1] != "secondary" {
		t.Errorf("chain = %v, want [primary secondary]", chain)
	}
}

func TestModelPoolSetFallbackEmpty(t *testing.T) {
	pool := NewModelPool()
	pool.SetFallback([]string{"x"})
	pool.SetFallback(nil)
	if len(pool.FallbackChain()) != 0 {
		t.Error("expected empty fallback chain")
	}
}

// SetFallback 应该复制入参 slice，外部修改不应影响内部状态。
func TestModelPoolSetFallbackDefensiveCopy(t *testing.T) {
	pool := NewModelPool()
	input := []string{"a", "b"}
	pool.SetFallback(input)
	input[0] = "modified"

	chain := pool.FallbackChain()
	if chain[0] != "a" {
		t.Errorf("expected defensive copy; chain[0] = %q", chain[0])
	}
}

// === Route ===

func TestModelPoolRouteEmpty(t *testing.T) {
	pool := NewModelPool()
	_, err := pool.Route(context.Background())
	if !errors.Is(err, ErrEmptyPool) {
		t.Errorf("expected ErrEmptyPool, got %v", err)
	}
}

func TestModelPoolRouteFallbackChain(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("primary"))
	_ = pool.Register(newMockProvider("secondary"))
	pool.SetFallback([]string{"primary", "secondary"})

	got, err := pool.Route(context.Background())
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if got.Name() != "primary" {
		t.Errorf("expected primary, got %q", got.Name())
	}
}

// Route 在 fallback 链中第一个 Provider 名字未注册时应跳过它，使用下一个。
func TestModelPoolRouteFallbackSkipsMissing(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("real"))
	pool.SetFallback([]string{"ghost", "real"})

	got, err := pool.Route(context.Background())
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if got.Name() != "real" {
		t.Errorf("expected real (skipping ghost), got %q", got.Name())
	}
}

func TestModelPoolRouteNoFallbackIteratesAll(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))

	got, err := pool.Route(context.Background())
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestModelPoolRouteContextCancelled(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := pool.Route(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestModelPoolRouteByName(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("alpha"))

	got, err := pool.RouteByName("alpha")
	if err != nil {
		t.Fatalf("RouteByName failed: %v", err)
	}
	if got.Name() != "alpha" {
		t.Errorf("got = %q", got.Name())
	}
}

func TestModelPoolRouteByNameMissing(t *testing.T) {
	pool := NewModelPool()
	_, err := pool.RouteByName("nope")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

// === API Key rotation ===

// SetKeyRotation 配置 Key 轮询后，applyKeyToProvider 应返回带新 Key 的 *OpenAICompatibleProvider 副本。
func TestModelPoolKeyRotation(t *testing.T) {
	pool := NewModelPool()
	oai := &OpenAICompatibleProvider{Model: "gpt", ProviderName: "oai", APIKey: "original"}
	_ = pool.Register(oai)

	if err := pool.SetKeyRotation("oai", []string{"k1", "k2", "k3"}); err != nil {
		t.Fatalf("SetKeyRotation failed: %v", err)
	}

	// 连续 3 次调用应该轮询到 3 个不同的 Key
	seen := map[string]int{}
	for i := 0; i < 3; i++ {
		got, err := pool.RouteByName("oai")
		if err != nil {
			t.Fatalf("RouteByName failed: %v", err)
		}
		clone, ok := got.(*OpenAICompatibleProvider)
		if !ok {
			t.Fatalf("expected *OpenAICompatibleProvider, got %T", got)
		}
		seen[clone.APIKey]++
	}

	if len(seen) != 3 {
		t.Errorf("expected 3 distinct keys, got %d: %v", len(seen), seen)
	}
}

// Key 轮询应循环：第 4 次回到 k1。
func TestModelPoolKeyRotationWraps(t *testing.T) {
	pool := NewModelPool()
	oai := &OpenAICompatibleProvider{ProviderName: "oai", APIKey: "original"}
	_ = pool.Register(oai)
	_ = pool.SetKeyRotation("oai", []string{"k1", "k2"})

	get := func() string {
		got, _ := pool.RouteByName("oai")
		return got.(*OpenAICompatibleProvider).APIKey
	}
	first := get()
	second := get()
	third := get()
	if first != third {
		t.Errorf("expected wrap-around: first=%q third=%q", first, third)
	}
	if first == second {
		t.Errorf("expected distinct keys: first=%q second=%q", first, second)
	}
}

// Key 轮询不应污染原 Provider 实例（clone 副本）。
func TestModelPoolKeyRotationDoesNotMutateOriginal(t *testing.T) {
	pool := NewModelPool()
	oai := &OpenAICompatibleProvider{ProviderName: "oai", APIKey: "original"}
	_ = pool.Register(oai)
	_ = pool.SetKeyRotation("oai", []string{"k1", "k2"})

	_, _ = pool.RouteByName("oai")
	if oai.APIKey != "original" {
		t.Errorf("original provider APIKey mutated: %q", oai.APIKey)
	}
}

// 非 OpenAICompatibleProvider 的 Provider 配置 Key 轮询应无副作用（返回原实例）。
func TestModelPoolKeyRotationNonOpenAIProvider(t *testing.T) {
	pool := NewModelPool()
	mp := newMockProvider("mock")
	_ = pool.Register(mp)
	_ = pool.SetKeyRotation("mock", []string{"k1"})

	got, err := pool.RouteByName("mock")
	if err != nil {
		t.Fatalf("RouteByName failed: %v", err)
	}
	if got != mp {
		t.Error("expected same mock provider instance for non-OpenAI provider")
	}
}

func TestModelPoolSetKeyRotationMissingProvider(t *testing.T) {
	pool := NewModelPool()
	err := pool.SetKeyRotation("nope", []string{"k"})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

// === RecordResult / Metrics ===

func TestModelPoolRecordResult(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p"))

	pool.RecordResult("p", true, 1_000_000)
	pool.RecordResult("p", true, 3_000_000)
	pool.RecordResult("p", false, 2_000_000)

	m := pool.Metrics("p")
	if m.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", m.SuccessCount)
	}
	if m.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", m.FailureCount)
	}
	if m.LatencySumNS != 6_000_000 {
		t.Errorf("LatencySumNS = %d, want 6000000", m.LatencySumNS)
	}
}

func TestModelPoolMetricsMissing(t *testing.T) {
	pool := NewModelPool()
	m := pool.Metrics("nope")
	if m.SuccessCount != 0 || m.FailureCount != 0 {
		t.Error("expected zero metrics for missing provider")
	}
}

// === ProviderMetrics 辅助方法 ===

func TestProviderMetricsAverageLatencyMS(t *testing.T) {
	m := ProviderMetrics{}
	m.LatencySumNS.Store(3_000_000) // 3ms in ns
	m.SuccessCount.Store(1)
	m.FailureCount.Store(2)
	// avg = 3_000_000 / 3 / 1e6 = 1.0ms
	if got := m.AverageLatencyMS(); got != 1.0 {
		t.Errorf("AverageLatencyMS = %v, want 1.0", got)
	}
}

func TestProviderMetricsAverageLatencyMSNoData(t *testing.T) {
	m := ProviderMetrics{}
	if got := m.AverageLatencyMS(); got != 0 {
		t.Errorf("AverageLatencyMS = %v, want 0", got)
	}
}

func TestProviderMetricsSuccessRate(t *testing.T) {
	m := ProviderMetrics{}
	m.SuccessCount.Store(3)
	m.FailureCount.Store(1)
	// rate = 3 / 4 = 0.75
	if got := m.SuccessRate(); got != 0.75 {
		t.Errorf("SuccessRate = %v, want 0.75", got)
	}
}

func TestProviderMetricsSuccessRateNoData(t *testing.T) {
	m := ProviderMetrics{}
	if got := m.SuccessRate(); got != 0 {
		t.Errorf("SuccessRate = %v, want 0", got)
	}
}

// === 线程安全 ===

// 并发 Register 不应 panic 且最终数量正确。
func TestModelPoolConcurrentRegister(t *testing.T) {
	pool := NewModelPool()
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_ = pool.Register(newMockProvider(string(rune('a'+i%26)) + "-" + itoa(i)))
		}(i)
	}
	wg.Wait()
	if len(pool.List()) != N {
		t.Errorf("expected %d providers, got %d", N, len(pool.List()))
	}
}

// 并发 Route + RecordResult 不应 panic。
func TestModelPoolConcurrentRouteAndRecord(t *testing.T) {
	pool := NewModelPool()
	_ = pool.Register(newMockProvider("p1"))
	_ = pool.Register(newMockProvider("p2"))
	pool.SetFallback([]string{"p1", "p2"})

	const N = 30
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = pool.Route(context.Background())
		}()
		go func() {
			defer wg.Done()
			pool.RecordResult("p1", true, 1_000)
		}()
	}
	wg.Wait()
}

// === 集成测试：注册 3 个 Provider + 1 个故意失败 → 验证自动降级 ===

// G1-08 验收标准：完整集成测试：注册 3 个 Provider + 1 个故意失败 → 验证自动降级
func TestModelPoolIntegrationFallbackOnFailure(t *testing.T) {
	pool := NewModelPool()
	primary := newMockProvider("primary")
	primary.failRate = 1 // 故意失败
	secondary := newMockProvider("secondary")
	tertiary := newMockProvider("tertiary")

	_ = pool.Register(primary)
	_ = pool.Register(secondary)
	_ = pool.Register(tertiary)
	pool.SetFallback([]string{"primary", "secondary", "tertiary"})

	// 通过 ExecuteSelect-like 流程模拟：先尝试 primary（失败），再降级
	// 这里直接验证 Route 返回的 Provider（基于 fallback 链，第一个可用即返回）
	// 由于 Route 不做实际请求探测，只能验证链顺序。
	// 我们通过 RecordResult + 重试逻辑模拟真实降级流程。

	ctx := context.Background()
	var selected ModelRequester
	for _, name := range pool.FallbackChain() {
		provider, err := pool.RouteByName(name)
		if err != nil {
			continue
		}
		// 模拟调用
		_, err = provider.RequestModel(ctx, &RequestData{Model: name})
		start := time.Now()
		elapsed := time.Since(start).Nanoseconds()
		pool.RecordResult(name, err == nil, elapsed)
		if err == nil {
			selected = provider
			break
		}
	}

	if selected == nil {
		t.Fatal("expected fallback to find a working provider")
	}
	if selected.Name() != "secondary" {
		t.Errorf("expected secondary to be selected, got %q", selected.Name())
	}

	// primary 应有 1 次失败记录
	mPrimary := pool.Metrics("primary")
	if mPrimary.FailureCount != 1 {
		t.Errorf("primary FailureCount = %d, want 1", mPrimary.FailureCount)
	}
	// secondary 应有 1 次成功记录
	mSecondary := pool.Metrics("secondary")
	if mSecondary.SuccessCount != 1 {
		t.Errorf("secondary SuccessCount = %d, want 1", mSecondary.SuccessCount)
	}
}

// itoa 是一个简易的 int → string 转换，避免引入 strconv 仅为此。
func itoa(i int) string {
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

// === G1-08: ModelPool 路由降级测试 ===

// drainStream 消费 stream 中所有 chunk，避免 goroutine 泄漏。
func drainStream(stream <-chan *StreamChunk) {
	if stream == nil {
		return
	}
	for range stream {
	}
}

// TestPoolRequestModelNormalRouting 验证 primary 可用时使用 primary。
func TestPoolRequestModelNormalRouting(t *testing.T) {
	primary := newMockProvider("primary")
	secondary := newMockProvider("secondary")
	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithProvider(primary),
		WithProvider(secondary),
	)

	stream, err := pool.RequestModel(context.Background(), &RequestData{Model: "test"})
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	drainStream(stream)

	if primary.CallCount() != 1 {
		t.Errorf("primary CallCount = %d, want 1", primary.CallCount())
	}
	if secondary.CallCount() != 0 {
		t.Errorf("secondary CallCount = %d, want 0", secondary.CallCount())
	}
}

// TestPoolRequestModelFallbackOnFailure 验证 primary 连续失败超过阈值后切换到 fallback。
func TestPoolRequestModelFallbackOnFailure(t *testing.T) {
	primary := newMockProvider("primary")
	primary.failRate = 1 // 始终失败
	secondary := newMockProvider("secondary")

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(3),
		WithProvider(primary),
		WithProvider(secondary),
	)

	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// 3 次调用：primary 每次失败，secondary 每次成功
	// 第 3 次调用时 primary 失败计数达到阈值，被标记为降级
	for i := 0; i < 3; i++ {
		stream, err := pool.RequestModel(ctx, data)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		drainStream(stream)
	}

	if primary.CallCount() != 3 {
		t.Errorf("primary CallCount = %d, want 3", primary.CallCount())
	}
	if pool.FailureCount("primary") != 3 {
		t.Errorf("primary FailureCount = %d, want 3", pool.FailureCount("primary"))
	}

	// 第 4 次调用：primary 已降级（跳过），secondary 直接被调用
	stream, err := pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("4th call failed: %v", err)
	}
	drainStream(stream)

	if primary.CallCount() != 3 {
		t.Errorf("primary CallCount = %d, want 3 (should be skipped after downgrade)", primary.CallCount())
	}
	if secondary.CallCount() != 4 {
		t.Errorf("secondary CallCount = %d, want 4", secondary.CallCount())
	}
}

// TestPoolRequestModelAllProvidersFail 验证所有 Provider 都失败时返回聚合错误。
func TestPoolRequestModelAllProvidersFail(t *testing.T) {
	primary := newMockProvider("primary")
	primary.failRate = 1
	secondary := newMockProvider("secondary")
	secondary.failRate = 1

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(2),
		WithProvider(primary),
		WithProvider(secondary),
	)

	_, err := pool.RequestModel(context.Background(), &RequestData{Model: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !errors.Is(err, ErrAllProvidersDown) {
		t.Errorf("expected ErrAllProvidersDown, got %v", err)
	}
}

// TestPoolRequestModelSuccessResetsCount 验证成功后重置失败计数。
func TestPoolRequestModelSuccessResetsCount(t *testing.T) {
	primary := newMockProvider("primary")
	secondary := newMockProvider("secondary")

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(3),
		WithProvider(primary),
		WithProvider(secondary),
	)

	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// primary 失败 2 次
	primary.failRate = 1
	for i := 0; i < 2; i++ {
		stream, _ := pool.RequestModel(ctx, data)
		drainStream(stream)
	}
	if pool.FailureCount("primary") != 2 {
		t.Fatalf("primary FailureCount = %d, want 2", pool.FailureCount("primary"))
	}

	// primary 成功 1 次，失败计数应重置
	primary.failRate = 0
	stream, err := pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	drainStream(stream)
	if pool.FailureCount("primary") != 0 {
		t.Errorf("primary FailureCount = %d, want 0 after success", pool.FailureCount("primary"))
	}

	// 再次失败 2 次，primary 不应被降级（计数从 0 重新开始）
	primary.failRate = 1
	for i := 0; i < 2; i++ {
		stream, _ := pool.RequestModel(ctx, data)
		drainStream(stream)
	}
	if pool.FailureCount("primary") != 2 {
		t.Errorf("primary FailureCount = %d, want 2", pool.FailureCount("primary"))
	}

	// 第 3 次失败才触发降级
	stream, _ = pool.RequestModel(ctx, data)
	drainStream(stream)
	if pool.FailureCount("primary") != 3 {
		t.Errorf("primary FailureCount = %d, want 3", pool.FailureCount("primary"))
	}

	// 第 4 次调用 primary 应被跳过
	primary.failRate = 0
	beforePrimary := primary.CallCount()
	stream, err = pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("expected secondary success: %v", err)
	}
	drainStream(stream)
	if primary.CallCount() != beforePrimary {
		t.Errorf("primary should be skipped (down), but CallCount changed: %d -> %d", beforePrimary, primary.CallCount())
	}
}

// TestPoolRequestModelIndependentFailureCounts 验证多 Provider 独立失败计数。
func TestPoolRequestModelIndependentFailureCounts(t *testing.T) {
	primary := newMockProvider("primary")
	primary.failRate = 1
	secondary := newMockProvider("secondary")
	tertiary := newMockProvider("tertiary")

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary", "tertiary"),
		WithFailureThreshold(2),
		WithProvider(primary),
		WithProvider(secondary),
		WithProvider(tertiary),
	)

	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// 2 次调用：primary 失败 2 次 → 降级；secondary 成功
	for i := 0; i < 2; i++ {
		stream, _ := pool.RequestModel(ctx, data)
		drainStream(stream)
	}
	if pool.FailureCount("primary") != 2 {
		t.Errorf("primary FailureCount = %d, want 2", pool.FailureCount("primary"))
	}
	if pool.FailureCount("secondary") != 0 {
		t.Errorf("secondary FailureCount = %d, want 0", pool.FailureCount("secondary"))
	}

	// 现在 primary 已降级；让 secondary 也失败
	secondary.failRate = 1
	for i := 0; i < 2; i++ {
		stream, _ := pool.RequestModel(ctx, data)
		drainStream(stream)
	}
	if pool.FailureCount("secondary") != 2 {
		t.Errorf("secondary FailureCount = %d, want 2", pool.FailureCount("secondary"))
	}
	// tertiary 成功，失败计数为 0
	if pool.FailureCount("tertiary") != 0 {
		t.Errorf("tertiary FailureCount = %d, want 0", pool.FailureCount("tertiary"))
	}

	// primary 和 secondary 都已降级；tertiary 仍可用
	primary.failRate = 0
	secondary.failRate = 0
	beforePrimary := primary.CallCount()
	beforeSecondary := secondary.CallCount()
	stream, err := pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("expected tertiary success: %v", err)
	}
	drainStream(stream)
	if primary.CallCount() != beforePrimary {
		t.Errorf("primary should be skipped (down)")
	}
	if secondary.CallCount() != beforeSecondary {
		t.Errorf("secondary should be skipped (down)")
	}
	if tertiary.CallCount() != 3 {
		t.Errorf("tertiary CallCount = %d, want 3", tertiary.CallCount())
	}
}

// TestPoolRequestModelRoutingPolicies 验证不同路由策略的选择逻辑。
func TestPoolRequestModelRoutingPolicies(t *testing.T) {
	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// RoutingFallback：primary 优先
	primary := newMockProvider("a")
	secondary := newMockProvider("b")
	pool := NewModelPool(
		WithPolicy(RoutingFallback),
		WithPrimary("a"),
		WithFallback("b", "a"),
		WithProvider(primary),
		WithProvider(secondary),
	)
	stream, err := pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	drainStream(stream)
	if primary.CallCount() != 1 || secondary.CallCount() != 0 {
		t.Errorf("RoutingFallback: primary CallCount=%d, secondary CallCount=%d; want 1, 0", primary.CallCount(), secondary.CallCount())
	}

	// RoutingCost：按 fallback 链顺序（简化实现），不特殊对待 primary
	cPrimary := newMockProvider("a")
	cSecondary := newMockProvider("b")
	cPool := NewModelPool(
		WithPolicy(RoutingCost),
		WithPrimary("a"),
		WithFallback("b", "a"),
		WithProvider(cPrimary),
		WithProvider(cSecondary),
	)
	stream, err = cPool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	drainStream(stream)
	if cSecondary.CallCount() != 1 || cPrimary.CallCount() != 0 {
		t.Errorf("RoutingCost: first CallCount=%d, second CallCount=%d; want b=1, a=0", cSecondary.CallCount(), cPrimary.CallCount())
	}

	// RoutingLatency：同样按 fallback 链顺序
	lPrimary := newMockProvider("a")
	lSecondary := newMockProvider("b")
	lPool := NewModelPool(
		WithPolicy(RoutingLatency),
		WithPrimary("a"),
		WithFallback("b", "a"),
		WithProvider(lPrimary),
		WithProvider(lSecondary),
	)
	stream, err = lPool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	drainStream(stream)
	if lSecondary.CallCount() != 1 || lPrimary.CallCount() != 0 {
		t.Errorf("RoutingLatency: first CallCount=%d, second CallCount=%d; want b=1, a=0", lSecondary.CallCount(), lPrimary.CallCount())
	}

	// RoutingQuality：同样按 fallback 链顺序
	qPrimary := newMockProvider("a")
	qSecondary := newMockProvider("b")
	qPool := NewModelPool(
		WithPolicy(RoutingQuality),
		WithPrimary("a"),
		WithFallback("b", "a"),
		WithProvider(qPrimary),
		WithProvider(qSecondary),
	)
	stream, err = qPool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("RequestModel failed: %v", err)
	}
	drainStream(stream)
	if qSecondary.CallCount() != 1 || qPrimary.CallCount() != 0 {
		t.Errorf("RoutingQuality: first CallCount=%d, second CallCount=%d; want b=1, a=0", qSecondary.CallCount(), qPrimary.CallCount())
	}
}

// TestPoolSwitchEventAuditHook 验证 Provider 切换时 auditHook 被调用。
func TestPoolSwitchEventAuditHook(t *testing.T) {
	primary := newMockProvider("primary")
	primary.failRate = 1
	secondary := newMockProvider("secondary")

	var events []PoolSwitchEvent
	var eventMu sync.Mutex
	hook := func(e PoolSwitchEvent) {
		eventMu.Lock()
		defer eventMu.Unlock()
		events = append(events, e)
	}

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(3),
		WithAuditHook(hook),
		WithProvider(primary),
		WithProvider(secondary),
	)

	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// 3 次调用后 primary 失败计数达到 3，触发降级
	for i := 0; i < 3; i++ {
		stream, _ := pool.RequestModel(ctx, data)
		drainStream(stream)
	}

	eventMu.Lock()
	defer eventMu.Unlock()
	if len(events) != 1 {
		t.Fatalf("expected 1 switch event, got %d", len(events))
	}
	e := events[0]
	if e.From != "primary" {
		t.Errorf("event.From = %q, want %q", e.From, "primary")
	}
	if e.To != "secondary" {
		t.Errorf("event.To = %q, want %q", e.To, "secondary")
	}
	if e.FailureCount != 3 {
		t.Errorf("event.FailureCount = %d, want 3", e.FailureCount)
	}
	if e.Reason == "" {
		t.Error("event.Reason should not be empty")
	}
	if e.Timestamp.IsZero() {
		t.Error("event.Timestamp should not be zero")
	}
}

// TestModelPoolImplementsModelRequester 验证 ModelPool 实现 ModelRequester 接口。
func TestModelPoolImplementsModelRequester(t *testing.T) {
	var _ ModelRequester = (*ModelPool)(nil)
	var _ ModelRequester = NewModelPool()
}

// TestPoolRequestModelEmptyPool 验证空池返回 ErrEmptyPool。
func TestPoolRequestModelEmptyPool(t *testing.T) {
	pool := NewModelPool()
	_, err := pool.RequestModel(context.Background(), &RequestData{Model: "test"})
	if !errors.Is(err, ErrEmptyPool) {
		t.Errorf("expected ErrEmptyPool, got %v", err)
	}
}

// TestPoolRequestModelContextCancelled 验证 context 取消时返回错误。
func TestPoolRequestModelContextCancelled(t *testing.T) {
	pool := NewModelPool(WithProvider(newMockProvider("p")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := pool.RequestModel(ctx, &RequestData{Model: "test"})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// TestPoolOptionFunctions 验证 PoolOption 函数正确配置 ModelPool。
func TestPoolOptionFunctions(t *testing.T) {
	p := newMockProvider("p1")
	s := newMockProvider("p2")
	pool := NewModelPool(
		WithPrimary("p1"),
		WithFallback("p1", "p2"),
		WithPolicy(RoutingCost),
		WithFailureThreshold(5),
		WithProvider(p),
		WithProvider(s),
	)

	if pool.Primary() != "p1" {
		t.Errorf("Primary = %q, want %q", pool.Primary(), "p1")
	}
	if pool.Policy() != RoutingCost {
		t.Errorf("Policy = %v, want RoutingCost", pool.Policy())
	}
	if pool.FailureThreshold() != 5 {
		t.Errorf("FailureThreshold = %d, want 5", pool.FailureThreshold())
	}
	chain := pool.FallbackChain()
	if len(chain) != 2 || chain[0] != "p1" || chain[1] != "p2" {
		t.Errorf("FallbackChain = %v, want [p1 p2]", chain)
	}
	got, err := pool.Get("p1")
	if err != nil || got.Name() != "p1" {
		t.Errorf("Get(p1) failed: %v", err)
	}
}

// TestPoolName 验证 ModelPool.Name()。
func TestPoolName(t *testing.T) {
	pool := NewModelPool()
	if pool.Name() != "model-pool" {
		t.Errorf("Name() = %q, want %q", pool.Name(), "model-pool")
	}
}

// TestPoolGenerateRequestData 验证 ModelPool.GenerateRequestData 委托给可用 Provider。
func TestPoolGenerateRequestData(t *testing.T) {
	primary := newMockProvider("primary")
	primary.failRate = 1 // RequestModel 失败不影响 GenerateRequestData
	secondary := newMockProvider("secondary")
	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithProvider(primary),
		WithProvider(secondary),
	)

	data, err := pool.GenerateRequestData(context.Background(), &ModelRequest{Input: "hello"})
	if err != nil {
		t.Fatalf("GenerateRequestData failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil RequestData")
	}
}

// TestPoolBroadcastResponse 验证 ModelPool.BroadcastResponse 转发 stream 为 events。
func TestPoolBroadcastResponse(t *testing.T) {
	pool := NewModelPool()

	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Delta: "hello"}
	stream <- &StreamChunk{Reasoning: "thinking"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := pool.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var gotDelta, gotReasoning, gotDone bool
	for e := range events {
		switch e.EventType {
		case EventDelta:
			gotDelta = true
		case ReasoningDelta:
			gotReasoning = true
		case EventDone:
			gotDone = true
		}
	}
	if !gotDelta {
		t.Error("expected EventDelta")
	}
	if !gotReasoning {
		t.Error("expected ReasoningDelta")
	}
	if !gotDone {
		t.Error("expected EventDone")
	}
}

// TestPoolRequestModelDownProviderSkipped 验证已降级的 Provider 被跳过。
func TestPoolRequestModelDownProviderSkipped(t *testing.T) {
	primary := newMockProvider("primary")
	secondary := newMockProvider("secondary")

	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(1), // 1 次失败即降级
		WithProvider(primary),
		WithProvider(secondary),
	)

	ctx := context.Background()
	data := &RequestData{Model: "test"}

	// primary 失败 1 次 → 立即降级
	primary.failRate = 1
	stream, _ := pool.RequestModel(ctx, data)
	drainStream(stream)

	// primary 恢复，但应被跳过（已降级）
	primary.failRate = 0
	stream, err := pool.RequestModel(ctx, data)
	if err != nil {
		t.Fatalf("expected secondary success: %v", err)
	}
	drainStream(stream)

	if primary.CallCount() != 1 {
		t.Errorf("primary CallCount = %d, want 1 (should be skipped after downgrade)", primary.CallCount())
	}
	if secondary.CallCount() != 2 {
		t.Errorf("secondary CallCount = %d, want 2", secondary.CallCount())
	}
}

// TestPoolConcurrentRequestModel 验证并发 RequestModel 不 panic。
func TestPoolConcurrentRequestModel(t *testing.T) {
	primary := newMockProvider("primary")
	secondary := newMockProvider("secondary")
	pool := NewModelPool(
		WithPrimary("primary"),
		WithFallback("primary", "secondary"),
		WithFailureThreshold(5),
		WithProvider(primary),
		WithProvider(secondary),
	)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			stream, _ := pool.RequestModel(context.Background(), &RequestData{Model: "test"})
			drainStream(stream)
		}()
	}
	wg.Wait()
}
