package model

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// BUG-5: ValidateAndRetry does not re-fetch on validation failure, so a
// single invalid response is retried against the same stale data. The new
// ValidateAndRetryWithFetch accepts a ResponseFetcher callback and invokes
// it on every attempt so each retry gets a fresh response from the model.
func TestValidateAndRetryWithFetchRefetchesOnFailure(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})
	v.MaxRetries = 3
	v.BackoffBase = 0.001

	var fetchCount int32
	fetcher := func(ctx context.Context) (*ModelResponse, error) {
		n := atomic.AddInt32(&fetchCount, 1)
		if n < 3 {
			// First two attempts: empty content (fails validation).
			return &ModelResponse{}, nil
		}
		// Third attempt: valid content.
		return &ModelResponse{Content: "valid"}, nil
	}

	resp, err := v.ValidateAndRetryWithFetch(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp == nil || resp.Content != "valid" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if fetchCount != 3 {
		t.Errorf("expected 3 fetches, got %d", fetchCount)
	}
}

// BUG-5: ValidateAndRetryWithFetch should propagate fetcher errors
// immediately (not retry on transport-level failures; that's the
// AttemptRunner's job).
func TestValidateAndRetryWithFetchPropagatesFetcherError(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})
	v.MaxRetries = 3
	v.BackoffBase = 0.001

	fetchErr := errors.New("upstream transport failure")
	fetcher := func(ctx context.Context) (*ModelResponse, error) {
		return nil, fetchErr
	}

	_, err := v.ValidateAndRetryWithFetch(context.Background(), fetcher)
	if err == nil {
		t.Fatal("expected error to be propagated")
	}
	if !errors.Is(err, fetchErr) {
		t.Errorf("expected error to wrap fetcher error, got: %v", err)
	}
}

// BUG-5: When all retries fail validation, return the validation error.
func TestValidateAndRetryWithFetchAllAttemptsFail(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})
	v.MaxRetries = 2
	v.BackoffBase = 0.001

	var fetchCount int32
	fetcher := func(ctx context.Context) (*ModelResponse, error) {
		atomic.AddInt32(&fetchCount, 1)
		return &ModelResponse{}, nil // always fails
	}

	_, err := v.ValidateAndRetryWithFetch(context.Background(), fetcher)
	if err == nil {
		t.Fatal("expected error after all retries")
	}
	// MaxRetries=2 → MaxAttempts = MaxRetries+1 = 3 fetches.
	if fetchCount != 3 {
		t.Errorf("expected 3 fetches, got %d", fetchCount)
	}
}

// BUG-5: ValidateAndRetryWithFetch should respect ctx cancellation.
func TestValidateAndRetryWithFetchContextCancellation(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})
	v.MaxRetries = 5
	v.BackoffBase = 1.0 // long backoff so cancellation triggers during sleep

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	fetcher := func(ctx context.Context) (*ModelResponse, error) {
		return &ModelResponse{}, nil // fails validation
	}

	_, err := v.ValidateAndRetryWithFetch(ctx, fetcher)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

// BUG-5: nil fetcher should error out cleanly.
func TestValidateAndRetryWithFetchNilFetcher(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})

	_, err := v.ValidateAndRetryWithFetch(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil fetcher")
	}
}

// BUG-5: nil Schema should short-circuit and call fetcher exactly once.
func TestValidateAndRetryWithFetchNilSchema(t *testing.T) {
	v := &OutputValidator{Schema: nil, MaxRetries: 3}

	var fetchCount int32
	fetcher := func(ctx context.Context) (*ModelResponse, error) {
		atomic.AddInt32(&fetchCount, 1)
		return &ModelResponse{Content: "anything"}, nil
	}

	resp, err := v.ValidateAndRetryWithFetch(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("expected no error for nil schema, got: %v", err)
	}
	if resp == nil || resp.Content != "anything" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch for nil schema, got %d", fetchCount)
	}
}
