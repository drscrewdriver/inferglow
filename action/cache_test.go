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

package action

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// cacheMockExecutor counts invocations and returns a fixed result.
type cacheMockExecutor struct {
	calls  atomic.Int32
	result *ActionResult
}

func (m *cacheMockExecutor) Execute(_ context.Context, _ map[string]any) (*ActionResult, error) {
	m.calls.Add(1)
	return m.result, nil
}

func TestCachedExecutor_CacheHit(t *testing.T) {
	inner := &cacheMockExecutor{result: &ActionResult{OK: true, Status: "success", Result: "hello"}}
	cfg := CacheConfig{MaxEntries: 10, DefaultTTL: time.Minute}
	cached := NewCachedExecutor(inner, "test_action", time.Minute, cfg)

	input := map[string]any{"key": "value"}
	ctx := context.Background()

	// First call: miss → executes inner.
	r1, err := cached.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r1.OK {
		t.Fatal("expected OK result")
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", inner.calls.Load())
	}

	// Second call: hit → does NOT execute inner.
	r2, err := cached.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r2.OK {
		t.Fatal("expected OK result on cache hit")
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("expected still 1 call (cache hit), got %d", inner.calls.Load())
	}
}

func TestCachedExecutor_TTLExpiry(t *testing.T) {
	inner := &cacheMockExecutor{result: &ActionResult{OK: true, Status: "success"}}
	cfg := CacheConfig{MaxEntries: 10, DefaultTTL: 50 * time.Millisecond}
	cached := NewCachedExecutor(inner, "ttl_action", 50*time.Millisecond, cfg)

	input := map[string]any{"q": "test"}
	ctx := context.Background()

	// First call.
	_, _ = cached.Execute(ctx, input)
	if inner.calls.Load() != 1 {
		t.Fatal("expected 1 call")
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	// Should miss cache and call inner again.
	_, _ = cached.Execute(ctx, input)
	if inner.calls.Load() != 2 {
		t.Fatalf("expected 2 calls after TTL expiry, got %d", inner.calls.Load())
	}
}

func TestCachedExecutor_LRUEviction(t *testing.T) {
	inner := &cacheMockExecutor{result: &ActionResult{OK: true, Status: "success"}}
	cfg := CacheConfig{MaxEntries: 2, DefaultTTL: time.Minute}
	cached := NewCachedExecutor(inner, "lru_action", time.Minute, cfg)

	ctx := context.Background()

	// Fill cache with 2 entries.
	_, _ = cached.Execute(ctx, map[string]any{"i": 1})
	_, _ = cached.Execute(ctx, map[string]any{"i": 2})
	if inner.calls.Load() != 2 {
		t.Fatal("expected 2 calls")
	}

	// Add a 3rd entry → evicts oldest (i=1).
	_, _ = cached.Execute(ctx, map[string]any{"i": 3})
	if inner.calls.Load() != 3 {
		t.Fatal("expected 3 calls")
	}

	// Access i=1 again → should be evicted, so inner is called.
	_, _ = cached.Execute(ctx, map[string]any{"i": 1})
	if inner.calls.Load() != 4 {
		t.Fatalf("expected 4 calls (evicted entry), got %d", inner.calls.Load())
	}

	// Access i=2 → should still be cached (was not evicted, i=1 was oldest).
	// Actually i=2 was evicted when i=3 was added (LRU: i=1 was accessed most recently
	// before i=3 was added... let's just verify i=3 is cached).
	_, _ = cached.Execute(ctx, map[string]any{"i": 3})
	if inner.calls.Load() != 4 {
		t.Fatalf("expected i=3 to be cached (still 4 calls), got %d", inner.calls.Load())
	}
}

func TestCachedExecutor_ErrorNotCached(t *testing.T) {
	inner := &cacheMockExecutor{result: &ActionResult{OK: false, Status: "error", Error: "fail"}}
	cfg := CacheConfig{MaxEntries: 10, DefaultTTL: time.Minute}
	cached := NewCachedExecutor(inner, "err_action", time.Minute, cfg)

	ctx := context.Background()
	input := map[string]any{"x": "y"}

	// First call: error result, should NOT be cached.
	_, _ = cached.Execute(ctx, input)
	_, _ = cached.Execute(ctx, input)
	if inner.calls.Load() != 2 {
		t.Fatalf("expected 2 calls (errors not cached), got %d", inner.calls.Load())
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	k1 := cacheKey("action", map[string]any{"a": 1, "b": 2})
	k2 := cacheKey("action", map[string]any{"a": 1, "b": 2})
	if k1 != k2 {
		t.Fatal("cache key should be deterministic")
	}

	k3 := cacheKey("action", map[string]any{"a": 1, "b": 3})
	if k1 == k3 {
		t.Fatal("different inputs should produce different keys")
	}

	k4 := cacheKey("other", map[string]any{"a": 1, "b": 2})
	if k1 == k4 {
		t.Fatal("different action names should produce different keys")
	}
}
