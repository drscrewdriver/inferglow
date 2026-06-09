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

// This file re-exports the retry controller and error classification helpers
// that now live in model/internal/attempt. The implementation was moved under
// internal/ to reduce the size of the model God Package (Task 13 of the
// architecture audit). The attempt package is generic over the chunk type T
// (constrained by attempt.Chunk) so it does not need to import the model
// package — which would be a circular dependency. The aliases and wrappers
// below preserve the existing public API so callers (validator, providers,
// examples, tests) continue to compile unchanged.
//
// AttemptRunner is re-exported as a generic type alias instantiated with the
// concrete chunk type: model.AttemptRunner = attempt.AttemptRunner[*StreamChunk].

package model

import (
	"github.com/inferglow/model/internal/attempt"
)

// === StreamChunk accessors for the attempt.Chunk interface ===
//
// These four methods let *StreamChunk satisfy attempt.Chunk without the
// attempt package importing model. Method names are prefixed with "Chunk" to
// avoid colliding with the struct's exported field names (Delta, Reasoning,
// Tools, Meta) — Go forbids a method and a field sharing a name on the same
// type.

// ChunkDelta returns the content delta carried by this chunk (empty if none).
func (c *StreamChunk) ChunkDelta() string { return c.Delta }

// ChunkReasoning returns the reasoning delta carried by this chunk (empty if none).
func (c *StreamChunk) ChunkReasoning() string { return c.Reasoning }

// ChunkToolCount returns the number of tool calls carried by this chunk.
func (c *StreamChunk) ChunkToolCount() int { return len(c.Tools) }

// ChunkMeta returns the chunk's metadata map (may be nil).
func (c *StreamChunk) ChunkMeta() map[string]any { return c.Meta }

// === Re-exports from model/internal/attempt ===

// ErrorClass categorizes errors so the retry controller can pick the right
// policy. See attempt.ErrorClass for details.
type ErrorClass = attempt.ErrorClass

const (
	// ErrorClassFatal indicates a fatal error (401/403) where retrying won't help.
	ErrorClassFatal = attempt.ErrorClassFatal
	// ErrorClassBackoffRetry indicates a rate-limited (429) error; retry with backoff.
	ErrorClassBackoffRetry = attempt.ErrorClassBackoffRetry
	// ErrorClassRetry indicates a server error (5xx) that may succeed on retry.
	ErrorClassRetry = attempt.ErrorClassRetry
	// ErrorClassRetryOnce indicates an unknown error; try once more just in case.
	ErrorClassRetryOnce = attempt.ErrorClassRetryOnce
)

// ClassifyError inspects an error and returns the matching ErrorClass.
// It delegates to attempt.ClassifyError.
func ClassifyError(err error) ErrorClass {
	return attempt.ClassifyError(err)
}

// AttemptDecisionAction 重试决策动作
type AttemptDecisionAction = attempt.AttemptDecisionAction

const (
	// AttemptRetry is the decision action indicating the request should be retried.
	AttemptRetry = attempt.AttemptRetry
	// AttemptRaise is the decision action indicating the error should be raised to the caller.
	AttemptRaise = attempt.AttemptRaise
	// AttemptYieldErr is the decision action indicating the error should be yielded as a stream event.
	AttemptYieldErr = attempt.AttemptYieldErr
	// AttemptStop is the decision action indicating the retry loop should stop without further attempts.
	AttemptStop = attempt.AttemptStop
)

// AttemptDecision 表示一次尝试后的决策
type AttemptDecision = attempt.AttemptDecision

// AttemptRunner 带指数退避的重试控制器。
//
// It is a type alias for attempt.AttemptRunner[*StreamChunk], so all exported
// fields (MaxAttempts, AttemptIndex, OutputStarted, AllowAfterOutputStarted,
// BackoffBase, BackoffMax) and methods (Run, MarkOutputStarted) are available
// exactly as before.
type AttemptRunner = attempt.AttemptRunner[*StreamChunk]

// NewAttemptRunner 创建新的 AttemptRunner.
func NewAttemptRunner() *AttemptRunner {
	return attempt.NewAttemptRunner[*StreamChunk]()
}
