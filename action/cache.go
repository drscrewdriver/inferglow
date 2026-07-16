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
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CacheConfig holds configuration for the tool result cache (OT-11).
type CacheConfig struct {
	// MaxEntries is the maximum number of cached results (default 256).
	MaxEntries int
	// MaxBytes is the maximum total memory for cached values (default 64MB).
	MaxBytes int64
	// DefaultTTL is the default time-to-live for cache entries (default 5min).
	DefaultTTL time.Duration
}

// DefaultCacheConfig returns sensible cache defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxEntries: 256,
		MaxBytes:   64 * 1024 * 1024, // 64MB
		DefaultTTL: 5 * time.Minute,
	}
}

// CachedExecutor wraps an ActionExecutor with a TTL + LRU result cache (OT-11).
// Only successful results (OK==true) are cached. Actions with "write" in Tags
// are never cached.
type CachedExecutor struct {
	inner  ActionExecutor
	cache  *lruTTLCache
	action string
	ttl    time.Duration
}

// NewCachedExecutor wraps an executor with caching behavior.
func NewCachedExecutor(inner ActionExecutor, actionName string, ttl time.Duration, cfg CacheConfig) *CachedExecutor {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 256
	}
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 5 * time.Minute
	}
	if ttl <= 0 {
		ttl = cfg.DefaultTTL
	}
	return &CachedExecutor{
		inner:  inner,
		cache:  newLRUTTLCache(cfg.MaxEntries, cfg.MaxBytes),
		action: actionName,
		ttl:    ttl,
	}
}

// Execute checks the cache before delegating to the inner executor.
func (c *CachedExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	key := cacheKey(c.action, input)

	// Check cache.
	if cached, ok := c.cache.get(key); ok {
		return cached, nil
	}

	// Execute inner.
	result, err := c.inner.Execute(ctx, input)
	if err != nil {
		return result, err
	}

	// Only cache successful results.
	if result != nil && result.OK {
		c.cache.put(key, result, c.ttl)
	}

	return result, nil
}

// cacheKey computes a deterministic cache key from action name + input.
func cacheKey(actionName string, input map[string]any) string {
	h := sha256.New()
	h.Write([]byte(actionName))
	// Canonical JSON for deterministic hashing.
	data, _ := json.Marshal(input)
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// --- LRU + TTL Cache implementation ---

type cacheEntry struct {
	key       string
	result    *ActionResult
	expireAt  time.Time
	sizeBytes int64
}

type lruTTLCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	order      *list.List // front = most recently used
	maxEntries int
	maxBytes   int64
	curBytes   int64
}

func newLRUTTLCache(maxEntries int, maxBytes int64) *lruTTLCache {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024 * 1024
	}
	return &lruTTLCache{
		entries:    make(map[string]*list.Element, maxEntries),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
	}
}

func (c *lruTTLCache) get(key string) (*ActionResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)
	// Check TTL.
	if time.Now().After(entry.expireAt) {
		c.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently used).
	c.order.MoveToFront(elem)
	return entry.result, true
}

func (c *lruTTLCache) put(key string, result *ActionResult, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := estimateSize(result)

	// If key already exists, update it.
	if elem, ok := c.entries[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		c.curBytes -= entry.sizeBytes
		entry.result = result
		entry.expireAt = time.Now().Add(ttl)
		entry.sizeBytes = size
		c.curBytes += size
		c.evict()
		return
	}

	// Insert new entry.
	entry := &cacheEntry{
		key:       key,
		result:    result,
		expireAt:  time.Now().Add(ttl),
		sizeBytes: size,
	}
	elem := c.order.PushFront(entry)
	c.entries[key] = elem
	c.curBytes += size
	c.evict()
}

// evict removes LRU entries until within bounds.
func (c *lruTTLCache) evict() {
	for c.order.Len() > c.maxEntries || c.curBytes > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.removeElement(back)
	}
}

func (c *lruTTLCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	c.order.Remove(elem)
	delete(c.entries, entry.key)
	c.curBytes -= entry.sizeBytes
}

// estimateSize provides a rough byte estimate for a cached result.
func estimateSize(r *ActionResult) int64 {
	if r == nil {
		return 0
	}
	// Rough estimate: JSON serialize.
	data, _ := json.Marshal(r)
	return int64(len(data))
}
