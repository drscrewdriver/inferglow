package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrAllProvidersDown 表示 ModelPool 中所有 Provider（含降级链）均失败。
// 由 Route 返回，调用方可据此切换到降级模式或返回错误。
var ErrAllProvidersDown = errors.New("all providers down")

// ErrProviderNotFound 表示指定名称的 Provider 未在池中注册。
var ErrProviderNotFound = errors.New("provider not found in pool")

// ErrEmptyPool 表示 ModelPool 未注册任何 Provider。
var ErrEmptyPool = errors.New("model pool is empty")

// ModelPool 管理多个 Provider 实例，支持按名注册、降级链、API Key 轮询。
// 所有方法均线程安全，可在并发请求中复用。
//
// 设计要点：
//   - Register/Get/SetFallback/SetKeyRotation 写操作加互斥锁
//   - Route 读操作使用 RLock，与 Router 配合实现策略选择
//   - API Key 轮询通过 atomic 计数器实现无锁轮询
type ModelPool struct {
	mu      sync.RWMutex
	items   map[string]*poolEntry
	fallback []string // 降级顺序：主 → 次1 → 次2 ...
}

// poolEntry 包装一个 Provider 及其可选的 API Key 轮询列表。
type poolEntry struct {
	provider ModelRequester
	// keys 为空表示使用 provider 自带的 APIKey；
	// 非空时 nextKey 在这些 key 之间轮询，防止单 Key 触发限流。
	keys []string
	// keyCounter 用于无锁轮询 API Key。
	keyCounter atomic.Uint64
	// metrics 记录该 Provider 的延迟/成功率指标，供 Router 使用。
	metrics ProviderMetrics
}

// ProviderMetrics 记录 Provider 的运行时指标，供路由策略参考。
// 所有字段均可被并发读写；写入端通常使用 atomic，读取端使用 atomic.Load。
type ProviderMetrics struct {
	// SuccessCount 成功调用次数
	SuccessCount atomic.Int64
	// FailureCount 失败调用次数
	FailureCount atomic.Int64
	// LatencySumNS 累计延迟（纳秒），可结合调用次数算平均延迟
	LatencySumNS atomic.Int64
}

// MetricsSnapshot 是 ProviderMetrics 的值快照，使用 plain int64 而非 atomic 类型，
// 便于按值返回/比较而不触发 go vet 的 lock-copy 警告。
// 字段值是某一瞬间的快照，多 goroutine 间可能略有偏差，但单次读取是原子的。
type MetricsSnapshot struct {
	SuccessCount int64
	FailureCount int64
	LatencySumNS int64
}

// Snapshot 原子地读取 ProviderMetrics 当前值，返回 plain int64 快照。
// 该方法可安全地按值返回（无 lock 拷贝问题）。
func (m *ProviderMetrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		SuccessCount: m.SuccessCount.Load(),
		FailureCount: m.FailureCount.Load(),
		LatencySumNS: m.LatencySumNS.Load(),
	}
}

// AverageLatencyMS 返回快照的平均延迟（毫秒）。无调用记录时返回 0。
func (s MetricsSnapshot) AverageLatencyMS() float64 {
	count := s.SuccessCount + s.FailureCount
	if count == 0 {
		return 0
	}
	return float64(s.LatencySumNS) / float64(count) / 1e6
}

// SuccessRate 返回快照的成功率 [0, 1]。无调用记录时返回 0。
func (s MetricsSnapshot) SuccessRate() float64 {
	total := s.SuccessCount + s.FailureCount
	if total == 0 {
		return 0
	}
	return float64(s.SuccessCount) / float64(total)
}

// TotalCalls 返回快照的总调用次数（成功 + 失败）。
func (s MetricsSnapshot) TotalCalls() int64 {
	return s.SuccessCount + s.FailureCount
}

// AverageLatencyMS 返回该 Provider 的平均延迟（毫秒）。
// 无调用记录时返回 0。
func (m *ProviderMetrics) AverageLatencyMS() float64 {
	total := m.LatencySumNS.Load()
	count := m.SuccessCount.Load() + m.FailureCount.Load()
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count) / 1e6
}

// SuccessRate 返回成功率 [0, 1]。无调用记录时返回 0。
func (m *ProviderMetrics) SuccessRate() float64 {
	total := m.SuccessCount.Load() + m.FailureCount.Load()
	if total == 0 {
		return 0
	}
	return float64(m.SuccessCount.Load()) / float64(total)
}

// NewModelPool 创建空的 ModelPool。
func NewModelPool() *ModelPool {
	return &ModelPool{
		items: make(map[string]*poolEntry),
	}
}

// Register 注册一个 Provider。重复注册同名 Provider 返回错误。
// provider 不能为 nil，且 Name() 不能为空。
func (p *ModelPool) Register(provider ModelRequester) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.items[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}
	p.items[name] = &poolEntry{provider: provider}
	return nil
}

// Get 按名获取 Provider。未注册返回 ErrProviderNotFound。
func (p *ModelPool) Get(name string) (ModelRequester, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.items[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	return entry.provider, nil
}

// List 返回所有已注册 Provider 名称（无序）。
func (p *ModelPool) List() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.items))
	for name := range p.items {
		names = append(names, name)
	}
	return names
}

// SetFallback 设置降级链。Route 在主 Provider 失败时按此顺序依次尝试。
// chain 中的每个名称必须已通过 Register 注册，否则 Route 会跳过它。
// 传空切片等价于清除降级链。
func (p *ModelPool) SetFallback(chain []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 复制一份避免外部修改
	p.fallback = append([]string(nil), chain...)
}

// FallbackChain 返回当前降级链的副本。
func (p *ModelPool) FallbackChain() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.fallback...)
}

// SetKeyRotation 为指定 Provider 配置 API Key 轮询列表。
// 配置后，Route 通过 nextKey 在这些 Key 之间轮询，防止单 Key 触发限流。
// provider 必须是 *OpenAICompatibleProvider，否则配置无意义（Key 轮询不生效）。
// keys 为空等价于清除该 Provider 的 Key 轮询配置。
func (p *ModelPool) SetKeyRotation(provider string, keys []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.items[provider]
	if !ok {
		return fmt.Errorf("%w: %q", ErrProviderNotFound, provider)
	}
	entry.keys = append([]string(nil), keys...)
	// 重置计数器
	entry.keyCounter.Store(0)
	return nil
}

// nextKey 返回指定 Provider 的下一个 API Key（轮询）。
// 若未配置 Key 轮询，返回空字符串（调用方应回退到 provider 自带的 Key）。
func (p *ModelPool) nextKey(name string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.items[name]
	if !ok || len(entry.keys) == 0 {
		return ""
	}
	idx := entry.keyCounter.Add(1) - 1
	return entry.keys[idx%uint64(len(entry.keys))]
}

// applyKeyToProvider 将轮询 Key 应用到 Provider 实例。
// 仅对 *OpenAICompatibleProvider 生效（复制一份实例以避免污染池中原始对象）。
func (p *ModelPool) applyKeyToProvider(name string) (ModelRequester, error) {
	p.mu.RLock()
	entry, ok := p.items[name]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	if len(entry.keys) == 0 {
		// 无 Key 轮询配置，直接返回原 Provider
		return entry.provider, nil
	}
	key := p.nextKey(name)
	// 仅 *OpenAICompatibleProvider 支持 Key 轮询；其他类型直接返回原实例。
	if oai, ok := entry.provider.(*OpenAICompatibleProvider); ok {
		clone := *oai
		clone.APIKey = key
		return &clone, nil
	}
	return entry.provider, nil
}

// metricsFor 返回指定 Provider 的指标对象（指针，可原子更新）。
// 未注册时返回 nil。
func (p *ModelPool) metricsFor(name string) *ProviderMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.items[name]
	if !ok {
		return nil
	}
	return &entry.metrics
}

// Route 按降级链选择一个可用 Provider。
// 路由流程：
//  1. 若 fallback 链非空，按链顺序依次尝试：拿到 Provider → 调用 tryProvider
//     → 成功则返回，失败则继续下一个。
//  2. 若 fallback 为空，遍历所有已注册 Provider，首个可用即返回。
//  3. 全部失败返回 ErrAllProvidersDown。
//
// 注意：Route 本身只做选择，不执行实际请求；实际请求由调用方通过返回的
// ModelRequester 发起。tryProvider 仅做轻量探测（如检查 Provider 是否为 nil），
// 真正的成功/失败由 RecordResult 在请求完成后记录。
//
// 当 ctx 被取消时，Route 立即返回 ctx.Err()。
func (p *ModelPool) Route(ctx context.Context) (ModelRequester, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	if len(p.items) == 0 {
		p.mu.RUnlock()
		return nil, ErrEmptyPool
	}

	// 构造尝试顺序：优先 fallback 链，否则所有已注册 Provider。
	var tryOrder []string
	if len(p.fallback) > 0 {
		tryOrder = append(tryOrder, p.fallback...)
	} else {
		for name := range p.items {
			tryOrder = append(tryOrder, name)
		}
	}
	p.mu.RUnlock()

	var lastErr error
	for _, name := range tryOrder {
		provider, err := p.applyKeyToProvider(name)
		if err != nil {
			lastErr = err
			continue
		}
		if provider == nil {
			lastErr = fmt.Errorf("provider %q is nil", name)
			continue
		}
		// 简单存活检查：Provider 已注册且非 nil 即视为可路由。
		// 实际请求成功/失败由 RecordResult 在请求完成后更新指标。
		return provider, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: last error: %v", ErrAllProvidersDown, lastErr)
	}
	return nil, ErrAllProvidersDown
}

// RouteByName 按 name 选择 Provider，不使用降级链。
// 适用于调用方明确指定要使用某个 Provider 的场景。
func (p *ModelPool) RouteByName(name string) (ModelRequester, error) {
	return p.applyKeyToProvider(name)
}

// RecordResult 记录一次调用的成功/失败与延迟，用于 Router 的指标统计。
// name 必须已注册；duration 为本次调用的总耗时。
// 该方法线程安全，可在请求完成后异步调用。
func (p *ModelPool) RecordResult(name string, success bool, durationNS int64) {
	m := p.metricsFor(name)
	if m == nil {
		return
	}
	if success {
		m.SuccessCount.Add(1)
	} else {
		m.FailureCount.Add(1)
	}
	m.LatencySumNS.Add(durationNS)
}

// Metrics 返回指定 Provider 的指标快照（plain int64 值副本，可安全按值返回）。
// 未注册时返回零值 MetricsSnapshot。
func (p *ModelPool) Metrics(name string) MetricsSnapshot {
	m := p.metricsFor(name)
	if m == nil {
		return MetricsSnapshot{}
	}
	return m.Snapshot()
}
