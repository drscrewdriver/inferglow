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
