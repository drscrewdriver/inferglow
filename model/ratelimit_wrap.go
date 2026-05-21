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

import "context"

// RateLimitAcquirer 是速率限制策略的抽象接口。
// github.com/inferglow/security/ratelimit 包中的 *Policy 类型
// 通过结构化类型系统自动满足此接口，无需显式声明 implements，
// 从而避免 model 包直接依赖 security 模块。
//
// 接口定义在 model 包（使用方），实现定义在 security/ratelimit 包
// （提供方），符合 Go "accept interfaces, return structs" 的惯例。
type RateLimitAcquirer interface {
	Acquire(ctx context.Context, providerName string, estimatedTokens int) error
}

// rateLimitedRequester 包装一个 ModelRequester，在 RequestModel 调用前
// 执行速率限制检查。其余方法直接委托给内层 requester。
type rateLimitedRequester struct {
	inner    ModelRequester
	acquirer RateLimitAcquirer
}

// WrapWithRateLimit 返回一个包装了速率限制的 ModelRequester。
// 每次 RequestModel 调用前会先调用 acquirer.Acquire；当 Acquire
// 返回错误（速率超限）时，RequestModel 直接返回该错误，不发起
// 实际请求。其余 ModelRequester 方法（Name、GenerateRequestData、
// BroadcastResponse）直接委托，不经过速率限制。
//
// requester 或 acquirer 为 nil 时直接返回 requester（不做包装），
// 保持向后兼容。
func WrapWithRateLimit(requester ModelRequester, acquirer RateLimitAcquirer) ModelRequester {
	if requester == nil || acquirer == nil {
		return requester
	}
	return &rateLimitedRequester{inner: requester, acquirer: acquirer}
}

func (w *rateLimitedRequester) Name() string {
	return w.inner.Name()
}

func (w *rateLimitedRequester) GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error) {
	return w.inner.GenerateRequestData(ctx, req)
}

func (w *rateLimitedRequester) RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error) {
	if err := w.acquirer.Acquire(ctx, w.inner.Name(), 0); err != nil {
		return nil, err
	}
	return w.inner.RequestModel(ctx, data)
}

func (w *rateLimitedRequester) BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error) {
	return w.inner.BroadcastResponse(ctx, stream)
}
