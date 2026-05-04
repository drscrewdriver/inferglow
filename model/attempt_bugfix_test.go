package model

import (
	"errors"
	"fmt"
	"testing"
)

// M-HIGH-5: ClassifyError categorizes errors by HTTP status code embedded in
// the error message so the retry controller can pick the right policy.
//
//   - 401, 403 -> ErrorClassFatal (no retry)
//   - 429      -> ErrorClassBackoffRetry (retry with backoff)
//   - 5xx      -> ErrorClassRetry (retry)
//   - other    -> ErrorClassRetryOnce
func TestClassifyError_401(t *testing.T) {
	err := fmt.Errorf("API error (status 401): unauthorized")
	if got := ClassifyError(err); got != ErrorClassFatal {
		t.Errorf("ClassifyError(401) = %v, want ErrorClassFatal", got)
	}
}

func TestClassifyError_403(t *testing.T) {
	err := fmt.Errorf("API error (status 403): forbidden")
	if got := ClassifyError(err); got != ErrorClassFatal {
		t.Errorf("ClassifyError(403) = %v, want ErrorClassFatal", got)
	}
}

func TestClassifyError_429(t *testing.T) {
	err := fmt.Errorf("API error (status 429): rate limited")
	if got := ClassifyError(err); got != ErrorClassBackoffRetry {
		t.Errorf("ClassifyError(429) = %v, want ErrorClassBackoffRetry", got)
	}
}

func TestClassifyError_500(t *testing.T) {
	err := fmt.Errorf("API error (status 500): internal server error")
	if got := ClassifyError(err); got != ErrorClassRetry {
		t.Errorf("ClassifyError(500) = %v, want ErrorClassRetry", got)
	}
}

func TestClassifyError_503(t *testing.T) {
	err := fmt.Errorf("API error (status 503): service unavailable")
	if got := ClassifyError(err); got != ErrorClassRetry {
		t.Errorf("ClassifyError(503) = %v, want ErrorClassRetry", got)
	}
}

func TestClassifyError_Other(t *testing.T) {
	// Non-API error (no status code pattern) -> ErrorClassRetryOnce
	err := errors.New("connection reset by peer")
	if got := ClassifyError(err); got != ErrorClassRetryOnce {
		t.Errorf("ClassifyError(other) = %v, want ErrorClassRetryOnce", got)
	}
}

func TestClassifyError_NilError(t *testing.T) {
	if got := ClassifyError(nil); got != ErrorClassRetryOnce {
		t.Errorf("ClassifyError(nil) = %v, want ErrorClassRetryOnce", got)
	}
}

// ErrorClass values must be ordered iota so they can be compared.
func TestErrorClassConstants(t *testing.T) {
	if ErrorClassFatal != ErrorClass(0) {
		t.Errorf("ErrorClassFatal = %v, want 0", ErrorClassFatal)
	}
	if ErrorClassBackoffRetry != ErrorClass(1) {
		t.Errorf("ErrorClassBackoffRetry = %v, want 1", ErrorClassBackoffRetry)
	}
	if ErrorClassRetry != ErrorClass(2) {
		t.Errorf("ErrorClassRetry = %v, want 2", ErrorClassRetry)
	}
	if ErrorClassRetryOnce != ErrorClass(3) {
		t.Errorf("ErrorClassRetryOnce = %v, want 3", ErrorClassRetryOnce)
	}
}
