package ratelimit

import (
	"context"
	"errors"
)

// ErrRateLimited 表示速率限制超出，请求被拒绝。
var ErrRateLimited = errors.New("ratelimit: rate limit exceeded")

// LimitMode 控制超出限额时的行为。
type LimitMode int

const (
	// LimitHard 超出直接拒绝，返回 ErrRateLimited。
	LimitHard LimitMode = iota
	// LimitSoft 超出排队等待，阻塞直到令牌可用或 ctx 取消。
	LimitSoft
)

// Policy 封装速率限制策略。根据 Mode 选择硬限制（拒绝）或软限制（排队）。
type Policy struct {
	Mode    LimitMode
	Limiter *ProviderLimiter
}

// NewPolicy 创建一个速率限制策略。mode 决定超出时的行为，
// limiter 提供 per-provider 限额查询。
func NewPolicy(mode LimitMode, limiter *ProviderLimiter) *Policy {
	return &Policy{Mode: mode, Limiter: limiter}
}

// Acquire 在发起请求前获取速率许可。
//
// LimitHard 模式：调用 CheckRequest + CheckTokens，超出返回 ErrRateLimited。
// LimitSoft 模式：调用 AcquireRequestBlock + AcquireTokensBlock 阻塞等待。
//
// estimatedTokens 为预估 token 数；传 0 表示仅检查 RPM。
func (p *Policy) Acquire(ctx context.Context, providerName string, estimatedTokens int) error {
	if p == nil || p.Limiter == nil {
		return nil
	}
	switch p.Mode {
	case LimitHard:
		if err := p.Limiter.CheckRequest(providerName); err != nil {
			return err
		}
		if err := p.Limiter.CheckTokens(providerName, estimatedTokens); err != nil {
			return err
		}
		return nil
	case LimitSoft:
		if err := p.Limiter.AcquireRequestBlock(ctx, providerName); err != nil {
			return err
		}
		if err := p.Limiter.AcquireTokensBlock(ctx, providerName, estimatedTokens); err != nil {
			return err
		}
		return nil
	}
	return nil
}
