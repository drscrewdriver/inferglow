package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderLimiter_CheckRequest_AllowUnderLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 5,
	})

	for i := 0; i < 5; i++ {
		if err := pl.CheckRequest("openai"); err != nil {
			t.Fatalf("CheckRequest #%d failed: %v", i, err)
		}
	}
}

func TestProviderLimiter_CheckRequest_RejectOverLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 3,
	})

	for i := 0; i < 3; i++ {
		if err := pl.CheckRequest("openai"); err != nil {
			t.Fatalf("CheckRequest #%d failed: %v", i, err)
		}
	}
	if err := pl.CheckRequest("openai"); err == nil {
		t.Error("CheckRequest should fail after exceeding RPM")
	}
}

func TestProviderLimiter_CheckRequest_NoLimit(t *testing.T) {
	pl := NewProviderLimiter()
	// No limit configured for this provider
	for i := 0; i < 100; i++ {
		if err := pl.CheckRequest("unknown"); err != nil {
			t.Fatalf("CheckRequest #%d should pass with no limit: %v", i, err)
		}
	}
}

func TestProviderLimiter_Fallback(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetFallback(ProviderLimit{
		MaxRequestsPerMinute: 2,
	})

	// "unknown" provider uses fallback
	if err := pl.CheckRequest("unknown"); err != nil {
		t.Fatalf("CheckRequest #1 failed: %v", err)
	}
	if err := pl.CheckRequest("unknown"); err != nil {
		t.Fatalf("CheckRequest #2 failed: %v", err)
	}
	if err := pl.CheckRequest("unknown"); err == nil {
		t.Error("CheckRequest #3 should fail (fallback RPM=2)")
	}
}

func TestProviderLimiter_FallbackOverriddenBySpecific(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetFallback(ProviderLimit{
		MaxRequestsPerMinute: 2,
	})
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 10,
	})

	// "openai" uses its own limit (10), not fallback (2)
	for i := 0; i < 10; i++ {
		if err := pl.CheckRequest("openai"); err != nil {
			t.Fatalf("CheckRequest #%d for openai failed: %v", i, err)
		}
	}
}

func TestProviderLimiter_CheckTokens_TPM(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxTokensPerMinute: 1000,
	})

	if err := pl.CheckTokens("openai", 500); err != nil {
		t.Fatalf("CheckTokens(500) failed: %v", err)
	}
	if err := pl.CheckTokens("openai", 500); err != nil {
		t.Fatalf("CheckTokens(500) failed: %v", err)
	}
	if err := pl.CheckTokens("openai", 1); err == nil {
		t.Error("CheckTokens(1) should fail after exceeding TPM")
	}
}

func TestProviderLimiter_CheckTokens_DailyLimit(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxTokensPerDay: 100,
	})

	if err := pl.CheckTokens("openai", 60); err != nil {
		t.Fatalf("CheckTokens(60) failed: %v", err)
	}
	if err := pl.CheckTokens("openai", 50); err == nil {
		t.Error("CheckTokens(50) should fail after exceeding daily limit (60+50>100)")
	}
}

func TestProviderLimiter_CheckTokens_ZeroOrNegative(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxTokensPerMinute: 100,
	})
	if err := pl.CheckTokens("openai", 0); err != nil {
		t.Errorf("CheckTokens(0) should always pass: %v", err)
	}
	if err := pl.CheckTokens("openai", -1); err != nil {
		t.Errorf("CheckTokens(-1) should always pass: %v", err)
	}
}

func TestProviderLimiter_RecordUsage(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxTokensPerMinute: 1000,
		MaxTokensPerDay:    10000,
	})

	pl.RecordUsage("openai", 300)

	// After recording 300 tokens, only 700 should remain
	if err := pl.CheckTokens("openai", 700); err != nil {
		t.Errorf("CheckTokens(700) after RecordUsage(300) failed: %v", err)
	}
	if err := pl.CheckTokens("openai", 1); err == nil {
		t.Error("CheckTokens should fail after exceeding remaining tokens")
	}
}

func TestProviderLimiter_MultiProviderIndependent(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 2,
	})
	pl.SetProviderLimit("anthropic", ProviderLimit{
		MaxRequestsPerMinute: 3,
	})

	// Drain openai
	pl.CheckRequest("openai")
	pl.CheckRequest("openai")
	if err := pl.CheckRequest("openai"); err == nil {
		t.Error("openai should be rate limited after 2 requests")
	}

	// Anthropic should still have tokens
	if err := pl.CheckRequest("anthropic"); err != nil {
		t.Errorf("anthropic should not be affected by openai usage: %v", err)
	}
	if err := pl.CheckRequest("anthropic"); err != nil {
		t.Errorf("anthropic CheckRequest #2 failed: %v", err)
	}
	if err := pl.CheckRequest("anthropic"); err != nil {
		t.Errorf("anthropic CheckRequest #3 failed: %v", err)
	}
	if err := pl.CheckRequest("anthropic"); err == nil {
		t.Error("anthropic should be rate limited after 3 requests")
	}
}

func TestProviderLimiter_Concurrent(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 100,
	})

	var successCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pl.CheckRequest("openai"); err == nil {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successCount.Load(); got != 100 {
		t.Errorf("concurrent successes = %d, want 100", got)
	}
}

func TestProviderLimiter_SetProviderLimit_Overwrite(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 2,
	})
	pl.CheckRequest("openai")
	pl.CheckRequest("openai")
	if err := pl.CheckRequest("openai"); err == nil {
		t.Error("should be rate limited after 2 requests")
	}

	// Overwrite with higher limit
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 10,
	})
	for i := 0; i < 10; i++ {
		if err := pl.CheckRequest("openai"); err != nil {
			t.Fatalf("CheckRequest #%d after overwrite failed: %v", i, err)
		}
	}
}

func TestProviderLimiter_AcquireRequestBlock(t *testing.T) {
	pl := NewProviderLimiter()
	pl.SetProviderLimit("openai", ProviderLimit{
		MaxRequestsPerMinute: 2,
	})

	// Take 2 (should succeed immediately)
	if err := pl.AcquireRequestBlock(context.Background(), "openai"); err != nil {
		t.Fatalf("AcquireRequestBlock #1 failed: %v", err)
	}
	if err := pl.AcquireRequestBlock(context.Background(), "openai"); err != nil {
		t.Fatalf("AcquireRequestBlock #2 failed: %v", err)
	}

	// Third should block — use short timeout to verify it blocks
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := pl.AcquireRequestBlock(ctx, "openai")
	if err == nil {
		t.Error("AcquireRequestBlock should block/fail when no tokens available")
	}
}
