package model

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// RoutingPolicy 定义路由策略类型。
type RoutingPolicy int

const (
	// RoutingFirst 表示按 fallback 链顺序选第一个可用 Provider（默认）。
	// 等同于直接调用 ModelPool.Route()。
	RoutingFirst RoutingPolicy = iota
	// RoutingRandom 在所有已注册 Provider 中随机选择一个（负载均衡）。
	// 适合多个 Provider 实例无差别分担流量的场景。
	RoutingRandom
	// RoutingCost 选择成本最低的 Provider。需要通过 SetPricing 配置单价。
	// 适合对成本敏感、希望自动选择最便宜可用 Provider 的场景。
	RoutingCost
	// RoutingLatency 选择平均延迟最低的 Provider。基于历史调用指标
	// （由 RecordResult 累积）选择延迟最小的 Provider。
	// 无历史数据时回退到 RoutingFirst。
	RoutingLatency
	// RoutingQuality 按成功率选择，优先成功率高的 Provider。
	// 适合对稳定性敏感的场景。无历史数据时回退到 RoutingFirst。
	RoutingQuality
	// RoutingFallback 故障时自动切换：先用 primary，连续失败超过阈值后
	// 按 fallbackChain 切换。由 ModelPool.RequestModel 实现。
	RoutingFallback
)

// PricingInfo 描述一个 Provider 的计费单价（每百万 token 美元）。
// 用于 RoutingCost 策略。
type PricingInfo struct {
	// PromptPerMillion 输入 token 单价（美元/百万 token）
	PromptPerMillion float64
	// CompletionPerMillion 输出 token 单价（美元/百万 token）
	CompletionPerMillion float64
	// ReasoningPerMillion 推理 token 单价（美元/百万 token）。
	// 0 表示与 CompletionPerMillion 同价。
	ReasoningPerMillion float64
}

// EffectivePerMillion 返回该 Provider 的"有效"输出单价，用于排序。
// 推理 token 单价 > 0 时取 max(Completion, Reasoning)，否则取 Completion。
// 这只是一个粗略的排序键，精确成本需结合实际 token 用量计算。
func (p PricingInfo) EffectivePerMillion() float64 {
	if p.ReasoningPerMillion > p.CompletionPerMillion {
		return p.ReasoningPerMillion
	}
	return p.CompletionPerMillion
}

// Router 在 ModelPool 之上实现多种路由策略。
// 调用方通过 Select() 选择一个 Provider，由 Execute() 执行请求并记录指标。
type Router struct {
	pool    *ModelPool
	policy  RoutingPolicy
	mu      sync.RWMutex
	pricing map[string]PricingInfo
}

// NewRouter 创建一个 Router，绑定到指定 ModelPool 和路由策略。
func NewRouter(pool *ModelPool, policy RoutingPolicy) *Router {
	return &Router{
		pool:    pool,
		policy:  policy,
		pricing: make(map[string]PricingInfo),
	}
}

// SetPricing 为指定 Provider 配置计费单价（用于 RoutingCost 策略）。
// 重复调用会覆盖旧值。
func (r *Router) SetPricing(provider string, pricing PricingInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pricing[provider] = pricing
}

// Pricing 返回指定 Provider 的计费单价副本。
// 未配置时返回零值 PricingInfo。
func (r *Router) Pricing(provider string) PricingInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pricing[provider]
}

// SetPolicy 切换路由策略。线程安全，可在运行时动态切换。
func (r *Router) SetPolicy(policy RoutingPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

// Policy 返回当前路由策略。
func (r *Router) Policy() RoutingPolicy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policy
}

// Select 按 Router 当前策略选择一个 Provider。
// 不同策略的选择逻辑：
//   - RoutingFirst: 委托给 ModelPool.Route（按 fallback 链）
//   - RoutingRandom: 在所有已注册 Provider 中随机选一个
//   - RoutingCost: 选 EffectivePerMillion 最低的
//   - RoutingLatency: 选平均延迟最低的（无数据回退到 RoutingFirst）
//   - RoutingQuality: 选成功率最高的（无数据回退到 RoutingFirst）
//
// 任何策略下，若选出的 Provider 实际不可用（如未注册），都会回退到
// ModelPool.Route 走降级链。
func (r *Router) Select(ctx context.Context) (ModelRequester, error) {
	if r.pool == nil {
		return nil, errors.New("router has no model pool")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	policy := r.policy
	pricing := r.pricing
	r.mu.RUnlock()

	switch policy {
	case RoutingRandom:
		return r.selectRandom(ctx)
	case RoutingCost:
		return r.selectByCost(ctx, pricing)
	case RoutingLatency:
		return r.selectByLatency(ctx)
	case RoutingQuality:
		return r.selectByQuality(ctx)
	default:
		// RoutingFirst 或未知策略：直接走 ModelPool 的 fallback 链。
		return r.pool.Route(ctx)
	}
}

// selectRandom 在所有已注册 Provider 中随机选一个。
// 无 Provider 时返回 ErrEmptyPool。
func (r *Router) selectRandom(ctx context.Context) (ModelRequester, error) {
	names := r.pool.List()
	if len(names) == 0 {
		return nil, ErrEmptyPool
	}
	// 使用全局 rand（Go 1.20+ 自动 seed）；不需要密码学安全的随机性。
	idx := rand.Intn(len(names))
	provider, err := r.pool.RouteByName(names[idx])
	if err != nil {
		// 选中的 Provider 异常时回退到 fallback 链。
		return r.pool.Route(ctx)
	}
	return provider, nil
}

// selectByCost 按 EffectivePerMillion 升序选择，取最便宜的可用 Provider。
// 未配置 pricing 的 Provider 视为 +∞（不会被选中，除非全部未配置）。
func (r *Router) selectByCost(ctx context.Context, pricing map[string]PricingInfo) (ModelRequester, error) {
	names := r.pool.List()
	if len(names) == 0 {
		return nil, ErrEmptyPool
	}

	// 过滤出已配置 pricing 的 Provider，按单价升序排序。
	type priced struct {
		name  string
		price float64
	}
	var pricedList []priced
	var unpriced []string
	for _, n := range names {
		if p, ok := pricing[n]; ok {
			pricedList = append(pricedList, priced{name: n, price: p.EffectivePerMillion()})
		} else {
			unpriced = append(unpriced, n)
		}
	}
	sort.Slice(pricedList, func(i, j int) bool {
		return pricedList[i].price < pricedList[j].price
	})

	// 依次尝试最便宜的 → 次便宜的 → ... → 未配置的（任意顺序）
	tryOrder := make([]string, 0, len(pricedList)+len(unpriced))
	for _, p := range pricedList {
		tryOrder = append(tryOrder, p.name)
	}
	tryOrder = append(tryOrder, unpriced...)

	for _, name := range tryOrder {
		provider, err := r.pool.RouteByName(name)
		if err == nil && provider != nil {
			return provider, nil
		}
	}
	return r.pool.Route(ctx)
}

// selectByLatency 按 AverageLatencyMS 升序选择，取延迟最低的可用 Provider。
// 所有 Provider 都无历史数据时回退到 RoutingFirst。
func (r *Router) selectByLatency(ctx context.Context) (ModelRequester, error) {
	names := r.pool.List()
	if len(names) == 0 {
		return nil, ErrEmptyPool
	}

	type latencyEntry struct {
		name    string
		latency float64
	}
	var entries []latencyEntry
	hasData := false
	for _, n := range names {
		m := r.pool.Metrics(n)
		lat := m.AverageLatencyMS()
		if lat > 0 {
			hasData = true
		}
		entries = append(entries, latencyEntry{name: n, latency: lat})
	}
	if !hasData {
		return r.pool.Route(ctx)
	}
	sort.Slice(entries, func(i, j int) bool {
		// 有数据的优先（latency > 0），相同 latency 按名字稳定排序
		if (entries[i].latency > 0) != (entries[j].latency > 0) {
			return entries[i].latency > 0
		}
		if entries[i].latency != entries[j].latency {
			return entries[i].latency < entries[j].latency
		}
		return entries[i].name < entries[j].name
	})

	for _, e := range entries {
		if e.latency == 0 {
			continue // 跳过无数据的
		}
		provider, err := r.pool.RouteByName(e.name)
		if err == nil && provider != nil {
			return provider, nil
		}
	}
	return r.pool.Route(ctx)
}

// selectByQuality 按 SuccessRate 降序选择，取成功率最高的可用 Provider。
// 所有 Provider 都无历史数据时回退到 RoutingFirst。
func (r *Router) selectByQuality(ctx context.Context) (ModelRequester, error) {
	names := r.pool.List()
	if len(names) == 0 {
		return nil, ErrEmptyPool
	}

	type qualityEntry struct {
		name string
		rate float64
	}
	var entries []qualityEntry
	hasData := false
	for _, n := range names {
		m := r.pool.Metrics(n)
		rate := m.SuccessRate()
		if m.TotalCalls() > 0 {
			hasData = true
		}
		entries = append(entries, qualityEntry{name: n, rate: rate})
	}
	if !hasData {
		return r.pool.Route(ctx)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rate != entries[j].rate {
			return entries[i].rate > entries[j].rate // 降序：高成功率优先
		}
		return entries[i].name < entries[j].name
	})

	for _, e := range entries {
		provider, err := r.pool.RouteByName(e.name)
		if err == nil && provider != nil {
			return provider, nil
		}
	}
	return r.pool.Route(ctx)
}

// ExecuteSelect 便捷方法：按策略选 Provider → 调用 fn → 记录指标。
// fn 收到选中的 Provider 后执行实际请求，返回 (result, error)。
// ExecuteSelect 会自动通过 RecordResult 记录成功/失败与耗时到 ModelPool，
// 供 RoutingLatency / RoutingQuality 策略参考。
//
// 若选择阶段失败（如所有 Provider 都不可用），直接返回错误，不调用 fn。
// 若 fn 返回错误，ExecuteSelect 仍会将该次失败记录到指标，并将错误透传给调用方。
func (r *Router) ExecuteSelect(
	ctx context.Context,
	fn func(provider ModelRequester) error,
) error {
	provider, err := r.Select(ctx)
	if err != nil {
		return fmt.Errorf("router select: %w", err)
	}
	name := provider.Name()
	startNS := nowNanoseconds()
	err = fn(provider)
	durationNS := nowNanoseconds() - startNS
	r.pool.RecordResult(name, err == nil, durationNS)
	return err
}

// nowNanoseconds 返回当前时间的纳秒值。拆分为独立变量便于测试中替换。
var nowNanoseconds = func() int64 {
	return time.Now().UnixNano()
}
