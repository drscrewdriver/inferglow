package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTokenBucket_StartsFull(t *testing.T) {
	b := NewTokenBucket(10, 60)
	if got := b.Tokens(); got != 10 {
		t.Errorf("new bucket tokens = %d, want 10", got)
	}
}

func TestNewTokenBucket_NegativeValues(t *testing.T) {
	b := NewTokenBucket(-5, -10)
	if got := b.Tokens(); got != 0 {
		t.Errorf("negative capacity bucket tokens = %d, want 0", got)
	}
	if got := b.RefillRate(); got != 0 {
		t.Errorf("negative refillRate = %d, want 0", got)
	}
}

func TestTokenBucket_Take_Success(t *testing.T) {
	b := NewTokenBucket(5, 0)
	if !b.Take(3) {
		t.Error("Take(3) on bucket with 5 tokens should succeed")
	}
	if got := b.Tokens(); got != 2 {
		t.Errorf("after Take(3), tokens = %d, want 2", got)
	}
}

func TestTokenBucket_Take_Fail(t *testing.T) {
	b := NewTokenBucket(3, 0)
	if !b.Take(3) {
		t.Error("Take(3) on full bucket should succeed")
	}
	if b.Take(1) {
		t.Error("Take(1) on empty bucket should fail")
	}
	if got := b.Tokens(); got != 0 {
		t.Errorf("after failed Take, tokens = %d, want 0", got)
	}
}

func TestTokenBucket_TakeZeroOrNegative(t *testing.T) {
	b := NewTokenBucket(5, 0)
	if !b.Take(0) {
		t.Error("Take(0) should always succeed")
	}
	if !b.Take(-1) {
		t.Error("Take(-1) should always succeed")
	}
	if got := b.Tokens(); got != 5 {
		t.Errorf("after Take(0) and Take(-1), tokens = %d, want 5", got)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	b := NewTokenBucket(10, 600) // 600/min = 10/sec
	b.Take(10)
	if got := b.Tokens(); got != 0 {
		t.Errorf("after draining, tokens = %d, want 0", got)
	}
	time.Sleep(150 * time.Millisecond) // ~1.5 tokens refilled
	b.Refill()
	if got := b.Tokens(); got < 1 {
		t.Errorf("after refill, tokens = %d, want >= 1", got)
	}
}

func TestTokenBucket_Refill_CappedAtCapacity(t *testing.T) {
	b := NewTokenBucket(5, 6000)
	b.Take(3)
	time.Sleep(100 * time.Millisecond)
	b.Refill()
	if got := b.Tokens(); got > 5 {
		t.Errorf("tokens = %d, should not exceed capacity 5", got)
	}
}

func TestTokenBucket_RefillZeroRate(t *testing.T) {
	b := NewTokenBucket(5, 0)
	b.Take(5)
	time.Sleep(50 * time.Millisecond)
	b.Refill()
	if got := b.Tokens(); got != 0 {
		t.Errorf("with refillRate=0, tokens = %d, want 0", got)
	}
}

func TestTokenBucket_TakeBlock_Success(t *testing.T) {
	b := NewTokenBucket(5, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.TakeBlock(ctx, 3); err != nil {
		t.Errorf("TakeBlock(3) on bucket with 5 tokens failed: %v", err)
	}
	if got := b.Tokens(); got != 2 {
		t.Errorf("after TakeBlock(3), tokens = %d, want 2", got)
	}
}

func TestTokenBucket_TakeBlock_ZeroOrNegative(t *testing.T) {
	b := NewTokenBucket(5, 0)
	ctx := context.Background()
	if err := b.TakeBlock(ctx, 0); err != nil {
		t.Errorf("TakeBlock(0) should succeed: %v", err)
	}
	if err := b.TakeBlock(ctx, -1); err != nil {
		t.Errorf("TakeBlock(-1) should succeed: %v", err)
	}
}

func TestTokenBucket_TakeBlock_BlocksUntilRefill(t *testing.T) {
	b := NewTokenBucket(1, 600) // 600/min = 10/sec → 1 token per 100ms
	b.Take(1)                   // drain

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := b.TakeBlock(ctx, 1)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("TakeBlock failed: %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("TakeBlock returned too fast: %v, expected to block", elapsed)
	}
}

func TestTokenBucket_TakeBlock_ContextCancelled(t *testing.T) {
	b := NewTokenBucket(0, 0) // empty, no refill
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := b.TakeBlock(ctx, 1)
	if err == nil {
		t.Fatal("TakeBlock should fail with cancelled context")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("TakeBlock error = %v, want context.DeadlineExceeded", err)
	}
}

func TestTokenBucket_TakeBlock_PartialFail(t *testing.T) {
	b := NewTokenBucket(2, 0) // no refill
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Take 2 (should succeed), then try 3 (should block and fail)
	if err := b.TakeBlock(ctx, 2); err != nil {
		t.Fatalf("TakeBlock(2) should succeed: %v", err)
	}
	err := b.TakeBlock(ctx, 3)
	if err == nil {
		t.Fatal("TakeBlock(3) on bucket with 0 tokens should fail")
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	b := NewTokenBucket(1000, 0)
	var successCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Take(1) {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successCount.Load(); got != 100 {
		t.Errorf("concurrent successes = %d, want 100", got)
	}
	if got := b.Tokens(); got != 900 {
		t.Errorf("after concurrent takes, tokens = %d, want 900", got)
	}
}

func TestTokenBucket_Concurrent_MoreThanCapacity(t *testing.T) {
	b := NewTokenBucket(50, 0)
	var successCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Take(1) {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successCount.Load(); got != 50 {
		t.Errorf("concurrent successes = %d, want 50 (capacity)", got)
	}
}

func TestTokenBucket_Tokens_DoesNotMutate(t *testing.T) {
	b := NewTokenBucket(10, 0)
	if got := b.Tokens(); got != 10 {
		t.Fatalf("Tokens() = %d, want 10", got)
	}
	if got := b.Tokens(); got != 10 {
		t.Errorf("second Tokens() = %d, want 10 (should not mutate)", got)
	}
}

func TestTokenBucket_Capacity(t *testing.T) {
	b := NewTokenBucket(42, 0)
	if got := b.Capacity(); got != 42 {
		t.Errorf("Capacity() = %d, want 42", got)
	}
}
