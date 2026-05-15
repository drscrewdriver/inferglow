package ratelimit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPolicy_LimitHard_AllowsUnderLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 5,
	})
	p := NewPolicy(LimitHard, pl)

	for i := 0; i < 5; i++ {
		if err := p.Acquire(context.Background(), "openai", 0); err != nil {
			t.Fatalf("Acquire #%d failed: %v", i, err)
		}
	}
}

func TestPolicy_LimitHard_RejectsOverLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 3,
	})
	p := NewPolicy(LimitHard, pl)

	for i := 0; i < 3; i++ {
		if err := p.Acquire(context.Background(), "openai", 0); err != nil {
			t.Fatalf("Acquire #%d failed: %v", i, err)
		}
	}
	err := p.Acquire(context.Background(), "openai", 0)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("Acquire over limit error = %v, want ErrRateLimited", err)
	}
}

func TestPolicy_LimitHard_CheckTokens(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 10,
		MaxTokensPerMinute:   100,
	})
	p := NewPolicy(LimitHard, pl)

	// 50 tokens should pass
	if err := p.Acquire(context.Background(), "openai", 50); err != nil {
		t.Fatalf("Acquire(50 tokens) failed: %v", err)
	}
	// Another 50 should pass
	if err := p.Acquire(context.Background(), "openai", 50); err != nil {
		t.Fatalf("Acquire(50 tokens) failed: %v", err)
	}
	// 1 more should fail (100 used out of 100)
	err := p.Acquire(context.Background(), "openai", 1)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("Acquire over TPM error = %v, want ErrRateLimited", err)
	}
}

func TestPolicy_LimitSoft_AllowsUnderLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 5,
	})
	p := NewPolicy(LimitSoft, pl)

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := p.Acquire(ctx, "openai", 0); err != nil {
			t.Fatalf("Acquire #%d failed: %v", i, err)
		}
		cancel()
	}
}

func TestPolicy_LimitSoft_BlocksUntilAvailable(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 1,
	})
	p := NewPolicy(LimitSoft, pl)

	// Take the only token
	if err := p.Acquire(context.Background(), "openai", 0); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// Second should block then succeed after refill
	// refillRate = 1/min → 1 token per minute → too slow for test
	// Use higher limit with fast refill instead
	pl2 := NewProviderLimiter()
	pl2.SetProviderLimit("fast", ProviderLimit{
		MaxRequestsPerMinute: 600, // 600/min = 10/sec
	})
	p2 := NewPolicy(LimitSoft, pl2)

	// Drain all 600 tokens
	for i := 0; i < 600; i++ {
		if err := p2.Acquire(context.Background(), "fast", 0); err != nil {
			t.Fatalf("drain Acquire #%d failed: %v", i, err)
		}
	}

	// Next Acquire should block then succeed after ~100ms
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err := p2.Acquire(ctx, "fast", 0)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Acquire after block failed: %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("Acquire returned too fast (%v), expected to block", elapsed)
	}
}

func TestPolicy_LimitSoft_ContextCancelled(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 1,
	})
	p := NewPolicy(LimitSoft, pl)

	// Drain the token
	p.Acquire(context.Background(), "openai", 0)

	// Second should block and fail with cancelled context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := p.Acquire(ctx, "openai", 0)
	if err == nil {
		t.Fatal("Acquire should fail with cancelled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Acquire error = %v, want context.DeadlineExceeded", err)
	}
}

func TestPolicy_NilPolicy(t *testing.T) {
	var p *Policy
	if err := p.Acquire(context.Background(), "openai", 0); err != nil {
		t.Errorf("nil Policy Acquire should be no-op, got: %v", err)
	}
}

func TestPolicy_NilLimiter(t *testing.T) {
	p := NewPolicy(LimitHard, nil)
	if err := p.Acquire(context.Background(), "openai", 0); err != nil {
		t.Errorf("Policy with nil Limiter should be no-op, got: %v", err)
	}
}

func TestPolicy_MultiProviderIndependent(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 2,
	})
	pl.SetProviderLimit("anthropic", ProviderLimit{
		MaxRequestsPerMinute: 3,
	})
	p := NewPolicy(LimitHard, pl)

	// Drain openai
	p.Acquire(context.Background(), "openai", 0)
	p.Acquire(context.Background(), "openai", 0)
	if err := p.Acquire(context.Background(), "openai", 0); err == nil {
		t.Error("openai should be rate limited")
	}

	// Anthropic unaffected
	for i := 0; i < 3; i++ {
		if err := p.Acquire(context.Background(), "anthropic", 0); err != nil {
			t.Errorf("anthropic Acquire #%d failed: %v", i, err)
		}
	}
	if err := p.Acquire(context.Background(), "anthropic", 0); err == nil {
		t.Error("anthropic should be rate limited after 3 requests")
	}
}

func TestPolicy_LimitHard_EstimatedTokens(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 10,
		MaxTokensPerMinute:   50,
	})
	p := NewPolicy(LimitHard, pl)

	// First request with estimated 30 tokens
	if err := p.Acquire(context.Background(), "openai", 30); err != nil {
		t.Fatalf("Acquire(30 tokens) failed: %v", err)
	}
	// Second request with estimated 30 tokens (30+30=60 > 50)
	err := p.Acquire(context.Background(), "openai", 30)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("Acquire over TPM error = %v, want ErrRateLimited", err)
	}
}

func TestPolicy_Concurrent(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 100,
	})
	p := NewPolicy(LimitHard, pl)

	var successCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Acquire(context.Background(), "openai", 0); err == nil {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successCount.Load(); got != 100 {
		t.Errorf("concurrent successes = %d, want 100", got)
	}
}

func TestPolicy_Fallback(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetFallback(ProviderLimit{
		MaxRequestsPerMinute: 2,
	})
	p := NewPolicy(LimitHard, pl)

	// Unknown provider uses fallback
	if err := p.Acquire(context.Background(), "unknown", 0); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	if err := p.Acquire(context.Background(), "unknown", 0); err != nil {
		t.Fatalf("second Acquire failed: %v", err)
	}
	if err := p.Acquire(context.Background(), "unknown", 0); err == nil {
		t.Error("third Acquire should fail (fallback RPM=2)")
	}
}
