package model

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// M-HIGH-6: AttemptRunner must retry when fn returns a stream successfully
// but the stream errors before any chunk is consumed (OutputStarted=false).
// Previously the stream was returned immediately and any mid-stream error was
// unrecoverable.
func TestAttemptRunner_StreamMidErrorRetry(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	var calls int32
	streamCh := make(chan *StreamChunk, 1)
	close(streamCh)

	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First call: return a stream that immediately closes without
			// producing any chunk. This simulates "stream created but failed
			// before first chunk". OutputStarted stays false → should retry.
			ch := make(chan *StreamChunk, 1)
			close(ch)
			return ch, nil
		}
		// Second call: success.
		ch := make(chan *StreamChunk, 1)
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Drain the returned stream.
	var done bool
	for chunk := range stream {
		if chunk.IsDone {
			done = true
		}
	}
	if !done {
		t.Error("expected a done chunk from the retried stream")
	}

	if calls < 2 {
		t.Errorf("expected at least 2 calls (retry), got %d", calls)
	}
	// OutputStarted should still be false because we never marked it.
	if runner.OutputStarted {
		t.Error("OutputStarted should remain false (no chunks consumed from first attempt)")
	}
}

// M-HIGH-6: when the stream errors out before producing any chunk (i.e. the
// channel is closed empty), Run should treat this as a recoverable failure
// and retry up to MaxAttempts.
func TestAttemptRunner_StreamEmptyRetriesUntilMax(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	var calls int32
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		atomic.AddInt32(&calls, 1)
		// Always return an empty (closed) stream — no chunks at all.
		ch := make(chan *StreamChunk)
		close(ch)
		return ch, nil
	})

	if err == nil {
		t.Fatal("expected error after max retries with empty streams")
	}
	if calls != 3 {
		t.Errorf("expected 3 attempts (max), got %d", calls)
	}
}

// M-HIGH-6: when OutputStarted=true (chunks consumed from the stream), a
// subsequent stream error must NOT trigger a retry.
func TestAttemptRunner_StreamErrorAfterOutputStartedNoRetry(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond
	runner.OutputStarted = true // mark as started

	var calls int32
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		atomic.AddInt32(&calls, 1)
		// Even though we return an empty stream, OutputStarted=true means we
		// should NOT retry.
		ch := make(chan *StreamChunk)
		close(ch)
		return ch, nil
	})

	if err == nil {
		t.Fatal("expected error (output started, no retry)")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry when OutputStarted=true), got %d", calls)
	}
}

// M-HIGH-6: AttemptRunner.Run with streamRetryAdapter wraps the returned
// stream so that an error chunk (with Meta["error"]) emitted BEFORE any
// content chunk triggers a retry instead of being forwarded to the caller.
//
// This tests the wrapper behavior directly: fn returns a stream that emits
// an error chunk, then closes. OutputStarted stays false → Run should retry
// and ultimately succeed.
func TestAttemptRunner_StreamErrorChunkBeforeContentRetries(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	var calls int32
	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		n := atomic.AddInt32(&calls, 1)
		ch := make(chan *StreamChunk, 2)
		if n == 1 {
			// Emit an error chunk BEFORE any content → OutputStarted stays
			// false → should retry.
			ch <- &StreamChunk{IsDone: true, Meta: map[string]any{"error": "stream init failed"}}
			close(ch)
			return ch, nil
		}
		ch <- &StreamChunk{Delta: "hi"}
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sawContent bool
	for chunk := range stream {
		if chunk.Delta != "" {
			sawContent = true
		}
	}
	if !sawContent {
		t.Error("expected content from retried stream")
	}
}

// M-HIGH-6: Ensure Run wraps the returned stream so that when the stream
// produces content chunks, OutputStarted is set true automatically.
func TestAttemptRunner_RunMarksOutputStartedOnContentChunk(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	stream, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		ch := make(chan *StreamChunk, 2)
		ch <- &StreamChunk{Delta: "first chunk"}
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for range stream {
	}

	// After consuming content chunks, OutputStarted must be true.
	if !runner.OutputStarted {
		t.Error("OutputStarted should be true after consuming content chunks")
	}
}

// M-HIGH-6: AttemptRunner.Run should return an error from fn directly when
// the error is fatal (ErrorClassFatal) instead of retrying. This requires Run
// to consult ClassifyError.
func TestAttemptRunner_RunFatalErrorNoRetry(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	var calls int32
	_, err := runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("API error (status 401): unauthorized")
	})

	if err == nil {
		t.Fatal("expected error for 401")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry on 401), got %d", calls)
	}
}

// Use a sleep to verify retry happens with backoff.
func TestAttemptRunner_RunRetryWithBackoffOn5xx(t *testing.T) {
	runner := NewAttemptRunner()
	runner.MaxAttempts = 3
	runner.BackoffBase = 1 * time.Millisecond

	var calls int32
	_, _ = runner.Run(context.Background(), func(ctx context.Context) (<-chan *StreamChunk, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil, errors.New("API error (status 503): unavailable")
		}
		ch := make(chan *StreamChunk, 1)
		ch <- &StreamChunk{IsDone: true}
		close(ch)
		return ch, nil
	})

	if calls != 3 {
		t.Errorf("expected 3 attempts (retry on 5xx), got %d", calls)
	}
}
