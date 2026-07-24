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

package contextmgr

import (
	"context"
	"strings"
	"testing"
)

// TestBuildContextL2CacheNoBullet asserts that the unified render path produces
// L2 Zone 3 blocks without the stray "• " bullet. This validates wp-a4 (线A):
// the cache-rendered output now matches the main-path (renderStepContent) semantics.
func TestBuildContextL2CacheNoBullet(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "raw l0 payload"}
	store.refs[1] = RefRecord{StepID: 1, Level: 2, Strength: 1.0}
	store.l2[1] = L2Record{StepID: 1, Facts: []string{"fact one", "fact two"}}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0 // force step 1 into Zone 3 compressed history
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	blocks, err := h.BuildContext(context.Background(), 128000)
	if err != nil {
		t.Fatal(err)
	}

	var found *RenderedBlock
	for i := range blocks {
		if blocks[i].StepID == 1 {
			found = &blocks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected block for step 1 in BuildContext output")
	}
	if !strings.Contains(found.Content, "⟨§1·reasoning·L2⟩") {
		t.Errorf("expected L2 marker in block: %q", found.Content)
	}
	if strings.Contains(found.Content, "• ") {
		t.Errorf("L2 Zone 3 block must NOT contain bullet; got %q", found.Content)
	}
	if !strings.Contains(found.Content, "fact one") || !strings.Contains(found.Content, "fact two") {
		t.Errorf("expected facts joined with newline; got %q", found.Content)
	}
}

// TestBuildContextCacheLevelChangeInvalidates proves the level-key
// self-invalidation: rendering at L1 then raising the ref level to L2
// must re-render (cache serves a fresh entry for the new level).
func TestBuildContextCacheLevelChangeInvalidates(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "reasoning", Content: "raw"}
	store.refs[1] = RefRecord{StepID: 1, Level: 1, Strength: 1.0}
	store.l1[1] = L1Record{StepID: 1, Content: "l1 summary"}
	store.l2[1] = L2Record{StepID: 1, Facts: []string{"l2 fact"}}

	cfg := DefaultConfig()
	cfg.TailKeepSteps = 0
	mgr, err := NewHybridManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	h := mgr.(*HybridManager)

	// First render at L1.
	if _, err := h.BuildContext(context.Background(), 128000); err != nil {
		t.Fatal(err)
	}
	if cached := h.renderCache.Get(1, 1); cached == nil {
		t.Fatal("expected cached entry for step 1 at L1")
	}

	// Raise level to L2, then re-render.
	ref, _ := store.GetRef(1)
	ref.Level = 2
	_ = store.UpsertRef(*ref)

	if _, err := h.BuildContext(context.Background(), 128000); err != nil {
		t.Fatal(err)
	}

	// Old L1 entry must be gone / replaced by L2 (level-key invalidation).
	if cached := h.renderCache.Get(1, 1); cached != nil {
		t.Error("expected L1 cached entry to be invalidated after level change")
	}
	cached := h.renderCache.Get(1, 2)
	if cached == nil {
		t.Fatal("expected L2 entry to be present after re-render")
	}
	if !strings.Contains(cached.Block, "⟨§1·reasoning·L2⟩") {
		t.Errorf("expected L2 marker in re-rendered block; got %q", cached.Block)
	}
}

// TestRenderStepWithCacheHitMissSlow verifies the fast-path/slow-path behaviour:
// first render is a cache miss (slow path renders and Sets), a second render at
// the same level hits the cache via fast path without re-rendering.
func TestRenderStepWithCacheHitMissSlow(t *testing.T) {
	store := newFakeStore()
	store.steps[1] = StepRecord{StepID: 1, Type: "tool", Content: "raw tool"}
	store.refs[1] = RefRecord{StepID: 1, Level: 2, Strength: 1.0}
	store.l2[1] = L2Record{StepID: 1, Facts: []string{"tool result facts"}}

	cache := NewRenderedCache()
	ref, _ := store.GetRef(1)

	// Slow path: cache miss on first call.
	if cache.Len() != 0 {
		t.Fatalf("expected empty cache at start, got Len=%d", cache.Len())
	}
	b1, err := RenderStepWithCache(1, *ref, cache, store)
	if err != nil {
		t.Fatal(err)
	}
	if cache.Len() != 1 {
		t.Fatalf("slow path should populate cache; expected Len=1, got %d", cache.Len())
	}
	if !strings.Contains(b1.Content, "⟨§1·tool·L2⟩") {
		t.Errorf("expected tool L2 marker; got %q", b1.Content)
	}
	if strings.Contains(b1.Content, "• ") {
		t.Errorf("slow path L2 output must not contain bullet; got %q", b1.Content)
	}

	// Fast path: cache hit, identical output, cache size unchanged.
	b2, err := RenderStepWithCache(1, *ref, cache, store)
	if err != nil {
		t.Fatal(err)
	}
	if cache.Len() != 1 {
		t.Errorf("fast path must not grow cache; expected Len=1, got %d", cache.Len())
	}
	if b1.Content != b2.Content {
		t.Errorf("fast path returned different block:\n b1=%q\n b2=%q", b1.Content, b2.Content)
	}
}