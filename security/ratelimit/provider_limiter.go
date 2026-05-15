package ratelimit

import (
	"context"
	"sync"
)

// ProviderLimit 描述单个 Provider 的限额配置。零值表示不限制。
// SetProviderLimit 会根据 Max* 字段自动初始化对应的 TokenBucket。
type ProviderLimit struct {
	MaxRequestsPerMinute int
	MaxTokensPerMinute   int
	MaxTokensPerDay      int

	requestBucket    *TokenBucket
	tokenBucket      *TokenBucket
	dailyTokenBucket *TokenBucket
}

// ProviderLimiter 管理多个 Provider 的独立速率限制。
// 每个 Provider 拥有独立的 RPM/TPM/日限额桶；未配置的 Provider
// 使用 fallback 默认限额。所有方法线程安全。
type ProviderLimiter struct {
	mu        sync.RWMutex
	providers map[string]*ProviderLimit
	fallback  *ProviderLimit
}

// NewProviderLimiter 创建空的 ProviderLimiter。可通过 SetProviderLimit
// 逐个配置 Provider 限额，通过 SetFallback 配置默认限额。
func NewProviderLimiter() *ProviderLimiter {
	return &ProviderLimiter{
		providers: make(map[string]*ProviderLimit),
	}
}

// SetProviderLimit 为指定 Provider 配置限额。会根据 Max* 字段自动
// 初始化令牌桶。重复调用会覆盖之前的配置。
func (pl *ProviderLimiter) SetProviderLimit(name string, limit ProviderLimit) {
	pl.initBuckets(&limit)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.providers[name] = &limit
}

// SetFallback 配置默认限额，用于未通过 SetProviderLimit 注册的 Provider。
func (pl *ProviderLimiter) SetFallback(limit ProviderLimit) {
	pl.initBuckets(&limit)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.fallback = &limit
}

// initBuckets 根据 Max* 字段初始化对应的 TokenBucket。
// RPM 桶：容量 = MaxRPM，补充速率 = MaxRPM/min（1 分钟恢复满）。
// TPM 桶：容量 = MaxTPM，补充速率 = MaxTPM/min。
// 日限额桶：容量 = MaxTPD，补充速率 = MaxTPD/1440/min（24 小时恢复满）。
func (pl *ProviderLimiter) initBuckets(limit *ProviderLimit) {
	if limit.MaxRequestsPerMinute > 0 {
		limit.requestBucket = NewTokenBucket(limit.MaxRequestsPerMinute, limit.MaxRequestsPerMinute)
	}
	if limit.MaxTokensPerMinute > 0 {
		limit.tokenBucket = NewTokenBucket(limit.MaxTokensPerMinute, limit.MaxTokensPerMinute)
	}
	if limit.MaxTokensPerDay > 0 {
		dailyRefill := limit.MaxTokensPerDay / 1440
		if dailyRefill == 0 {
			dailyRefill = 1
		}
		limit.dailyTokenBucket = NewTokenBucket(limit.MaxTokensPerDay, dailyRefill)
	}
}

// lookup 返回指定 Provider 的限额配置。未注册时返回 fallback。
func (pl *ProviderLimiter) lookup(providerName string) *ProviderLimit {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	if l, ok := pl.providers[providerName]; ok {
		return l
	}
	return pl.fallback
}

// CheckRequest 检查指定 Provider 的 RPM。成功扣减 1 个请求令牌
// 并返回 nil；超出返回 ErrRateLimited。未配置 RPM 限制时直接放行。
func (pl *ProviderLimiter) CheckRequest(providerName string) error {
	limit := pl.lookup(providerName)
	if limit == nil || limit.requestBucket == nil {
		return nil
	}
	if !limit.requestBucket.Take(1) {
		return ErrRateLimited
	}
	return nil
}

// CheckTokens 检查指定 Provider 的 TPM 和日限额。成功扣减 tokens
// 个令牌并返回 nil；任一桶不足返回 ErrRateLimited。tokens <= 0
// 时直接放行。
func (pl *ProviderLimiter) CheckTokens(providerName string, tokens int) error {
	limit := pl.lookup(providerName)
	if limit == nil || tokens <= 0 {
		return nil
	}
	if limit.tokenBucket != nil && !limit.tokenBucket.Take(tokens) {
		return ErrRateLimited
	}
	if limit.dailyTokenBucket != nil && !limit.dailyTokenBucket.Take(tokens) {
		return ErrRateLimited
	}
	return nil
}

// RecordUsage 记录指定 Provider 的实际 token 使用量。从 TPM 桶和
// 日限额桶中扣减 tokens 个令牌。用于在请求完成后记录实际消耗。
func (pl *ProviderLimiter) RecordUsage(providerName string, tokens int) {
	limit := pl.lookup(providerName)
	if limit == nil || tokens <= 0 {
		return
	}
	if limit.tokenBucket != nil {
		limit.tokenBucket.Take(tokens)
	}
	if limit.dailyTokenBucket != nil {
		limit.dailyTokenBucket.Take(tokens)
	}
}

// AcquireRequestBlock 是 LimitSoft 模式下的 RPM 检查：阻塞直到
// 取到 1 个请求令牌或 ctx 取消。
func (pl *ProviderLimiter) AcquireRequestBlock(ctx context.Context, providerName string) error {
	limit := pl.lookup(providerName)
	if limit == nil || limit.requestBucket == nil {
		return nil
	}
	return limit.requestBucket.TakeBlock(ctx, 1)
}

// AcquireTokensBlock 是 LimitSoft 模式下的 TPM/日限额检查：阻塞直到
// 取到 tokens 个令牌或 ctx 取消。
func (pl *ProviderLimiter) AcquireTokensBlock(ctx context.Context, providerName string, tokens int) error {
	limit := pl.lookup(providerName)
	if limit == nil || tokens <= 0 {
		return nil
	}
	if limit.tokenBucket != nil {
		if err := limit.tokenBucket.TakeBlock(ctx, tokens); err != nil {
			return err
		}
	}
	if limit.dailyTokenBucket != nil {
		if err := limit.dailyTokenBucket.TakeBlock(ctx, tokens); err != nil {
			return err
		}
	}
	return nil
}
