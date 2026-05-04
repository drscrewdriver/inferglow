package model

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// ErrorClass categorizes errors so the retry controller can pick the right
// policy. M-HIGH-5: previously every error was treated the same, leading to
// retry storms on 401/403 (no point retrying) and inadequate backoff on 429.
type ErrorClass int

const (
	// ErrorClassFatal: 401/403 — auth/permission issues, retrying won't help.
	ErrorClassFatal ErrorClass = iota
	// ErrorClassBackoffRetry: 429 — rate limited, retry with backoff.
	ErrorClassBackoffRetry
	// ErrorClassRetry: 5xx — server errors, retry.
	ErrorClassRetry
	// ErrorClassRetryOnce: unknown — try once more just in case.
	ErrorClassRetryOnce
)

// ClassifyError inspects an error returned by RequestModel (which formats HTTP
// errors as "API error (status NNN): ...") and returns the matching
// ErrorClass. nil errors and unrecognized formats fall back to
// ErrorClassRetryOnce.
//
// M-HIGH-5: error classification mechanism.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassRetryOnce
	}
	msg := err.Error()
	// Look for "status NNN" pattern produced by RequestModel error formatting.
	status := extractStatusCode(msg)
	switch {
	case status == 401 || status == 403:
		return ErrorClassFatal
	case status == 429:
		return ErrorClassBackoffRetry
	case status >= 500 && status <= 599:
		return ErrorClassRetry
	default:
		return ErrorClassRetryOnce
	}
}

// extractStatusCode scans msg for "status NNN" and returns NNN, or 0 if not
// found / not a valid 3-digit status.
func extractStatusCode(msg string) int {
	idx := strings.Index(msg, "status")
	if idx < 0 {
		return 0
	}
	rest := msg[idx+len("status"):]
	// Skip whitespace / parens.
	rest = strings.TrimLeft(rest, " (")
	// Read up to 3 digits.
	digits := make([]byte, 0, 3)
	for i := 0; i < len(rest) && len(digits) < 3; i++ {
		c := rest[i]
		if c < '0' || c > '9' {
			break
		}
		digits = append(digits, c)
	}
	if len(digits) == 0 {
		return 0
	}
	n, err := strconv.Atoi(string(digits))
	if err != nil {
		return 0
	}
	return n
}

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
//
// M-HIGH-6: stream 中途错误且 OutputStarted=false 时触发重试。
// 当 fn 返回 stream 成功后，Run 会先 peek 第一个 chunk 来判断是否需要
// 重试：
//   - stream 关闭未产生任何 chunk → 重试
//   - 首个 chunk 是错误 chunk (Meta["error"]) 且 OutputStarted=false → 重试
//   - 首个 chunk 是 content (Delta/Reasoning/Tools) → 标记 OutputStarted=true
//
// M-HIGH-5: 基于 ClassifyError 分类决策：
//   - ErrorClassFatal (401/403) → 不重试，立即返回错误
//   - ErrorClassRetry (5xx) / ErrorClassBackoffRetry (429) → 重试
func (ar *AttemptRunner) Run(ctx context.Context, fn func(ctx context.Context) (<-chan *StreamChunk, error)) (<-chan *StreamChunk, error) {
	var lastErr error
	for ar.AttemptIndex < ar.MaxAttempts {
		ar.AttemptIndex++

		stream, err := fn(ctx)
		if err != nil {
			lastErr = err
			// M-HIGH-5: fatal errors (401/403) — don't retry.
			if ClassifyError(err) == ErrorClassFatal {
				return nil, err
			}
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
			continue
		}

		// M-HIGH-6: peek the first chunk to detect early failure before any
		// output is produced. If retry is needed, drain the inner stream and
		// continue the retry loop. Otherwise, wrap the stream so the peeked
		// chunk is forwarded to the consumer and OutputStarted is updated as
		// content chunks flow through.
		peeked, retry, peekErr := ar.peekForEarlyFailure(stream)
		if retry {
			if peekErr != nil {
				lastErr = peekErr
			}
			// If output already started, do not retry (caller must handle).
			if ar.OutputStarted && !ar.AllowAfterOutputStarted {
				if peekErr != nil {
					return nil, fmt.Errorf("output started, aborting retry: %w", peekErr)
				}
				return nil, fmt.Errorf("output started, aborting retry: stream closed without output")
			}
			if ar.AttemptIndex >= ar.MaxAttempts {
				if peekErr != nil {
					return nil, fmt.Errorf("max attempts (%d) reached, last error: %w", ar.MaxAttempts, peekErr)
				}
				return nil, fmt.Errorf("max attempts (%d) reached, stream closed without output", ar.MaxAttempts)
			}
			// backoff and retry
			backoff := ar.calculateBackoff()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		// No retry — wrap the stream and return to caller.
		return ar.wrapStream(peeked, stream), nil
	}

	return nil, fmt.Errorf("unexpected end of retry loop: %w", lastErr)
}

// peekForEarlyFailure reads the first chunk from stream to decide whether Run
// should retry. It returns:
//   - peeked: the chunks already pulled from stream (to be re-emitted by the
//     wrapper).
//   - retry: true if Run should retry (no content produced, or error chunk
//     before any content).
//   - retryErr: the error captured from an error chunk, if any.
//
// On content chunks (Delta/Reasoning/Tools), OutputStarted is set true so
// subsequent retries are blocked.
func (ar *AttemptRunner) peekForEarlyFailure(stream <-chan *StreamChunk) (peeked []*StreamChunk, retry bool, retryErr error) {
	firstChunk, ok := <-stream
	if !ok {
		// Stream closed without producing any chunk → retry.
		return nil, true, nil
	}
	// Error chunk before any content → retry.
	if firstChunk.Meta != nil {
		if _, hasErr := firstChunk.Meta["error"]; hasErr && !ar.OutputStarted {
			// Drain remaining chunks to avoid leaking the producer goroutine.
			for range stream {
			}
			if errStr, ok := firstChunk.Meta["error"].(string); ok {
				return nil, true, fmt.Errorf("stream error before output: %s", errStr)
			}
			return nil, true, fmt.Errorf("stream error before output: %v", firstChunk.Meta["error"])
		}
	}
	// Mark OutputStarted if this chunk carries content.
	if firstChunk.Delta != "" || firstChunk.Reasoning != "" || len(firstChunk.Tools) > 0 {
		ar.OutputStarted = true
	}
	return []*StreamChunk{firstChunk}, false, nil
}

// wrapStream returns a new channel that first emits the peeked chunks, then
// forwards all remaining chunks from the inner stream. As content chunks
// flow through, OutputStarted is updated so subsequent retries are blocked.
func (ar *AttemptRunner) wrapStream(peeked []*StreamChunk, stream <-chan *StreamChunk) <-chan *StreamChunk {
	out := make(chan *StreamChunk, 64)
	go func() {
		defer close(out)
		for _, c := range peeked {
			out <- c
		}
		for chunk := range stream {
			if chunk.Delta != "" || chunk.Reasoning != "" || len(chunk.Tools) > 0 {
				ar.OutputStarted = true
			}
			out <- chunk
		}
	}()
	return out
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
