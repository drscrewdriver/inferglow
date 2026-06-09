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

// Package attempt contains the retry controller (AttemptRunner) and error
// classification helpers used by the model package.
//
// It is placed under model/internal/ so only code within the model module can
// import it. The package is generic over the chunk type T (constrained by the
// Chunk interface) to avoid a circular dependency on the model package, which
// defines the concrete *StreamChunk type and re-exports AttemptRunner via a
// type alias: model.AttemptRunner = attempt.AttemptRunner[*model.StreamChunk].
package attempt

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// Chunk is the minimal view of a stream chunk that AttemptRunner needs. The
// concrete chunk type (e.g. *model.StreamChunk) must implement this interface
// so AttemptRunner can peek at chunk fields without importing the model
// package. The methods must not collide with the chunk's exported field names;
// the model package adds them as accessor methods on *StreamChunk.
type Chunk interface {
	// ChunkDelta returns the content delta string (empty if none).
	ChunkDelta() string
	// ChunkReasoning returns the reasoning delta string (empty if none).
	ChunkReasoning() string
	// ChunkToolCount returns the number of tool calls carried by the chunk.
	ChunkToolCount() int
	// ChunkMeta returns the chunk's metadata map (may be nil).
	ChunkMeta() map[string]any
}

// ErrorClass categorizes errors so the retry controller can pick the right
// policy. M-HIGH-5: previously every error was treated the same, leading to
// retry storms on 401/403 (no point retrying) and inadequate backoff on 429.
type ErrorClass int

const (
	// ErrorClassFatal indicates a fatal error (401/403 — auth/permission issues) where retrying won't help.
	ErrorClassFatal ErrorClass = iota
	// ErrorClassBackoffRetry indicates a rate-limited (429) error; retry with backoff.
	ErrorClassBackoffRetry
	// ErrorClassRetry indicates a server error (5xx) that may succeed on retry.
	ErrorClassRetry
	// ErrorClassRetryOnce indicates an unknown error; try once more just in case.
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
	// AttemptRetry is the decision action indicating the request should be retried.
	AttemptRetry AttemptDecisionAction = "retry"
	// AttemptRaise is the decision action indicating the error should be raised to the caller.
	AttemptRaise AttemptDecisionAction = "raise"
	// AttemptYieldErr is the decision action indicating the error should be yielded as a stream event.
	AttemptYieldErr AttemptDecisionAction = "yield_error"
	// AttemptStop is the decision action indicating the retry loop should stop without further attempts.
	AttemptStop AttemptDecisionAction = "stop"
)

// AttemptDecision 表示一次尝试后的决策
type AttemptDecision struct {
	Action                  AttemptDecisionAction
	Reason                  string
	Error                   error
	AllowAfterOutputStarted bool
}

// AttemptRunner 带指数退避的重试控制器
//
// T is the stream chunk type, constrained by Chunk so the runner can inspect
// chunk fields (delta/reasoning/tools/meta) without importing the model
// package. The model package instantiates it as AttemptRunner[*StreamChunk].
type AttemptRunner[T Chunk] struct {
	MaxAttempts             int
	AttemptIndex            int
	OutputStarted           bool
	AllowAfterOutputStarted bool
	BackoffBase             time.Duration
	BackoffMax              time.Duration
	rng                     *rand.Rand
}

// NewAttemptRunner 创建新的 AttemptRunner
func NewAttemptRunner[T Chunk]() *AttemptRunner[T] {
	return &AttemptRunner[T]{
		MaxAttempts: 3,
		BackoffBase: time.Second,
		BackoffMax:  30 * time.Second,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
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
func (ar *AttemptRunner[T]) Run(ctx context.Context, fn func(ctx context.Context) (<-chan T, error)) (<-chan T, error) {
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
func (ar *AttemptRunner[T]) peekForEarlyFailure(stream <-chan T) (peeked []T, retry bool, retryErr error) {
	firstChunk, ok := <-stream
	if !ok {
		// Stream closed without producing any chunk → retry.
		return nil, true, nil
	}
	// Error chunk before any content → retry.
	if meta := firstChunk.ChunkMeta(); meta != nil {
		if _, hasErr := meta["error"]; hasErr && !ar.OutputStarted {
			// Drain remaining chunks to avoid leaking the producer goroutine.
			for range stream {
			}
			if errStr, ok := meta["error"].(string); ok {
				return nil, true, fmt.Errorf("stream error before output: %s", errStr)
			}
			return nil, true, fmt.Errorf("stream error before output: %v", meta["error"])
		}
	}
	// Mark OutputStarted if this chunk carries content.
	if firstChunk.ChunkDelta() != "" || firstChunk.ChunkReasoning() != "" || firstChunk.ChunkToolCount() > 0 {
		ar.OutputStarted = true
	}
	return []T{firstChunk}, false, nil
}

// wrapStream returns a new channel that first emits the peeked chunks, then
// forwards all remaining chunks from the inner stream. As content chunks
// flow through, OutputStarted is updated so subsequent retries are blocked.
func (ar *AttemptRunner[T]) wrapStream(peeked []T, stream <-chan T) <-chan T {
	out := make(chan T, 64)
	go func() {
		defer close(out)
		for _, c := range peeked {
			out <- c
		}
		for chunk := range stream {
			if chunk.ChunkDelta() != "" || chunk.ChunkReasoning() != "" || chunk.ChunkToolCount() > 0 {
				ar.OutputStarted = true
			}
			out <- chunk
		}
	}()
	return out
}

// MarkOutputStarted 标记输出已开始（停止重试）
func (ar *AttemptRunner[T]) MarkOutputStarted() {
	ar.OutputStarted = true
}

// calculateBackoff 计算指数退避时间
func (ar *AttemptRunner[T]) calculateBackoff() time.Duration {
	// 指数退避: base * 2^(attempt-1)
	exp := float64(ar.AttemptIndex - 1)
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
