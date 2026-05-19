package agent

import "context"

// RateLimitHook 在 executeLoop 调用 RequestModel 前执行速率限制检查。
// 实现者返回非 nil error 时，请求被拒绝，executeLoop 直接返回该错误。
//
// github.com/inferglow/security/ratelimit 包中的 *Policy 类型通过
// 结构化类型系统自动满足此接口，可直接传入 WithRateLimitHook。
type RateLimitHook interface {
	Acquire(ctx context.Context, providerName string, estimatedTokens int) error
}

// WithRateLimitHook 安装速率限制钩子。传入 nil 可禁用已有的钩子。
// *ratelimit.Policy 满足 RateLimitHook 接口，可直接传入。
func WithRateLimitHook(hook RateLimitHook) RunOption {
	return func(c *runConfig) {
		c.rateLimitHook = hook
	}
}
