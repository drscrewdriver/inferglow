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

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Check: AttemptDecisionAction 常量定义完整
func TestAttemptDecisionActionConstants(t *testing.T) {
	expected := []AttemptDecisionAction{
		AttemptRetry, AttemptRaise, AttemptYieldErr, AttemptStop,
	}
	for _, action := range expected {
		if string(action) == "" {
			t.Errorf("empty action: %v", action)
		}
	}

	if AttemptRetry != "retry" {
		t.Errorf("AttemptRetry = %q, want %q", AttemptRetry, "retry")
	}
	if AttemptRaise != "raise" {
		t.Errorf("AttemptRaise = %q, want %q", AttemptRaise, "raise")
	}
	if AttemptYieldErr != "yield_error" {
		t.Errorf("AttemptYieldErr = %q, want %q", AttemptYieldErr, "yield_error")
	}
	if AttemptStop != "stop" {
		t.Errorf("AttemptStop = %q, want %q", AttemptStop, "stop")
	}
}

// Check: AttemptDecision 结构体完整
func TestAttemptDecisionFields(t *testing.T) {
	dec := AttemptDecision{
		Action:                  AttemptRetry,
		Reason:                  "timeout",
		Error:                   errors.New("timeout error"),
		AllowAfterOutputStarted: true,
	}

	if dec.Action != AttemptRetry {
		t.Errorf("Action = %v, want %v", dec.Action, AttemptRetry)
	}
	if dec.Reason != "timeout" {
		t.Errorf("Reason = %q, want %q", dec.Reason, "timeout")
	}
	if dec.Error == nil {
		t.Error("Error should not be nil")
	}
	if !dec.AllowAfterOutputStarted {
		t.Error("AllowAfterOutputStarted should be true")
	}
}

// Check: MarkOutputStarted 正确设置状态
func TestMarkOutputStarted(t *testing.T) {
	runner := NewAttemptRunner()
	if runner.OutputStarted {
		t.Error("OutputStarted should be false initially")
	}

	runner.MarkOutputStarted()
	if !runner.OutputStarted {
		t.Error("OutputStarted should be true after MarkOutputStarted")
	}
}

// Check: 尝试次数为0时不执行
func TestAttemptRunnerZeroAttempts(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 0

	callCount := 0
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		return nil, errors.New("error")
	})

	if err == nil {
		t.Fatal("expected error for zero attempts")
	}
	if callCount != 0 {
		t.Errorf("expected 0 calls, got %d", callCount)
	}
}

// Check: 首次调用就成功
func TestAttemptRunnerFirstCallSuccess(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond

	callCount := 0
	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		ch := make(chan *StreamChunk, 1)
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (first success), got %d", callCount)
	}
	_ = <-stream
}

// Check: 尝试次数用完后的错误信息
func TestAttemptRunnerErrorMessages(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 2
	runner.BackoffBase = 1 * time.Millisecond

	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		return nil, errors.New("persistent error")
	})

	if err == nil {
		t.Fatal("expected error")
	}
	// 错误信息应该非空
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// Check: context cancellation 在首次调用时发生
func TestAttemptRunnerContextCancelledFirstCall(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond
	runner.MaxAttempts = 3

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	callCount := 0
	_, err := runner.Run(ctx, func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		return nil, errors.New("error")
	})

	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	// 首次调用会执行，cancel 发生在 fn 内部
	if callCount < 1 {
		t.Errorf("expected at least 1 call, got %d", callCount)
	}
}

// Check: AllowAfterOutputStarted=true 时允许重试
func TestAttemptRunnerAllowAfterOutputStarted(t *testing.T) {
	runner := NewAttemptRunner()
	runner.BackoffBase = 1 * time.Millisecond
	runner.OutputStarted = true
	runner.AllowAfterOutputStarted = true

	callCount := 0
	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		callCount++
		if callCount < 2 {
			return nil, errors.New("error")
		}
		ch := make(chan *StreamChunk, 1)
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (retry allowed), got %d", callCount)
	}
	_ = <-stream
}
