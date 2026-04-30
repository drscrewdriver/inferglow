package model

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// AttemptDecisionAction 重试决策动作
type AttemptDecisionAction string

const (
	AttemptRetry      AttemptDecisionAction = "retry"
	AttemptRaise      AttemptDecisionAction = "raise"
	AttemptYieldErr   AttemptDecisionAction = "yield_error"
	AttemptStop       AttemptDecisionAction = "stop"
)

// AttemptDecision 表示一次尝试后的决策
type AttemptDecision struct {
	Action                  AttemptDecisionAction
	Reason                  string
	Error                   error
	AllowAfterOutputStarted bool
}

// AttemptRunner 带指数退避的重试控制器
type AttemptRunner struct {
	MaxAttempts           int
	AttemptIndex          int
	OutputStarted         bool
	AllowAfterOutputStarted bool
	BackoffBase           time.Duration
	BackoffMax            time.Duration
	rng                   *rand.Rand
}

// NewAttemptRunner 创建新的 AttemptRunner
func NewAttemptRunner() *AttemptRunner {
	return &AttemptRunner{
		MaxAttempts:   3,
		BackoffBase:   time.Second,
		BackoffMax:    30 * time.Second,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run 执行带重试的请求函数
func (ar *AttemptRunner) Run(ctx context.Context, fn func(ctx context.Context) (<-chan *StreamChunk, error)) (<-chan *StreamChunk, error) {
	var lastErr error
	for ar.AttemptIndex < ar.MaxAttempts {
		ar.AttemptIndex++

		stream, err := fn(ctx)
		if err == nil {
			return stream, nil
		}

		lastErr = err

		// 如果已经开始输出且不允许重试，直接返回错误
		if ar.OutputStarted && !ar.AllowAfterOutputStarted {
			return nil, fmt.Errorf("output started, aborting retry: %w", err)
		}

		// 达到最大次数
		if ar.AttemptIndex >= ar.MaxAttempts {
			return nil, fmt.Errorf("max attempts (%d) reached, last error: %w", ar.MaxAttempts, err)
		}

		// 指数退避
		backoff := ar.calculateBackoff()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			// 等待结束，继续重试
		}
	}

	return nil, fmt.Errorf("unexpected end of retry loop: %w", lastErr)
}

// MarkOutputStarted 标记输出已开始（停止重试）
func (ar *AttemptRunner) MarkOutputStarted() {
	ar.OutputStarted = true
}

// calculateBackoff 计算指数退避时间
func (ar *AttemptRunner) calculateBackoff() time.Duration {
	// 指数退避: base * 2^(attempt-1)
	exp := float64(ar.AttemptIndex-1)
	backoff := float64(ar.BackoffBase) * math.Pow(2, exp)

	// 添加抖动 (±10%)
	jitter := 1.0 + (ar.rng.Float64()*0.2 - 0.1)
	backoff *= jitter

	// 限制最大值
	if backoff > float64(ar.BackoffMax) {
		backoff = float64(ar.BackoffMax)
	}

	return time.Duration(backoff)
}
