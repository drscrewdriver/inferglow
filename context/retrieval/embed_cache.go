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

package retrieval

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
)

// EmbeddingCacheBackend is the minimal interface for caching text→embedding.
// A Redis Hash implementation is typical; in-memory is provided as default.
type EmbeddingCacheBackend interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttlSeconds int) error
}

// EmbeddingCache wraps an Embedder with a cache layer to avoid redundant
// embedding API calls. Uses a Hash-style key-value store (Redis or in-memory).
type EmbeddingCache struct {
	backend EmbeddingCacheBackend
	embedder Embedder
	prefix  string
	ttl     int // seconds
	mu      sync.RWMutex
}

// NewEmbeddingCache creates a caching wrapper around an Embedder.
// prefix is the key prefix (e.g. "embed:"). ttl is cache TTL in seconds.
func NewEmbeddingCache(embedder Embedder, backend EmbeddingCacheBackend, prefix string, ttl int) *EmbeddingCache {
	return &EmbeddingCache{
		backend:  backend,
		embedder: embedder,
		prefix:   prefix,
		ttl:      ttl,
	}
}

// Embed returns the vector embedding for text, using cache if available.
func (c *EmbeddingCache) Embed(ctx context.Context, text string) ([]float32, error) {
	key := c.prefix + text

	// Try cache first.
	if cached, err := c.backend.Get(ctx, key); err == nil && cached != "" {
		var vec []float32
		if err := json.Unmarshal([]byte(cached), &vec); err == nil {
			return vec, nil
		}
	}

	// Cache miss: call the underlying embedder.
	vec, err := c.embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Store in cache.
	if data, err := json.Marshal(vec); err == nil {
		_ = c.backend.Set(ctx, key, string(data), c.ttl)
	}

	return vec, nil
}

// Dim returns the embedding dimensionality.
func (c *EmbeddingCache) Dim() int {
	return c.embedder.Dim()
}

// InMemoryEmbeddingCache is a simple in-memory implementation of
// EmbeddingCacheBackend for testing and single-process deployments.
type InMemoryEmbeddingCache struct {
	mu    sync.RWMutex
	items map[string]string
}

// NewInMemoryEmbeddingCache creates a new in-memory embedding cache backend.
func NewInMemoryEmbeddingCache() *InMemoryEmbeddingCache {
	return &InMemoryEmbeddingCache{items: make(map[string]string)}
}

// Get retrieves a cached value by key.
func (c *InMemoryEmbeddingCache) Get(_ context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

// Set stores a value with the given key. TTL is ignored for in-memory cache.
func (c *InMemoryEmbeddingCache) Set(_ context.Context, key, value string, _ int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
	return nil
}

// RedisEmbeddingCacheBackend implements EmbeddingCacheBackend using Redis Hash.
// It stores text→embedding mappings in a Redis Hash for O(1) lookup.
type RedisEmbeddingCacheBackend struct {
	client RedisClient
	hashKey string
}

// RedisClient is the minimal Redis interface needed for embedding cache.
type RedisClient interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key, field, value string) error
	Expire(ctx context.Context, key string, seconds int) error
}

// NewRedisEmbeddingCacheBackend creates a Redis-backed embedding cache.
// hashKey is the Redis Hash key (e.g. "embeddings").
func NewRedisEmbeddingCacheBackend(client RedisClient, hashKey string) *RedisEmbeddingCacheBackend {
	return &RedisEmbeddingCacheBackend{client: client, hashKey: hashKey}
}

// Get retrieves an embedding from the Redis Hash.
func (b *RedisEmbeddingCacheBackend) Get(ctx context.Context, key string) (string, error) {
	return b.client.HGet(ctx, b.hashKey, key)
}

// Set stores an embedding in the Redis Hash.
func (b *RedisEmbeddingCacheBackend) Set(ctx context.Context, key, value string, ttl int) error {
	if err := b.client.HSet(ctx, b.hashKey, key, value); err != nil {
		return err
	}
	if ttl > 0 {
		return b.client.Expire(ctx, b.hashKey, ttl)
	}
	return nil
}

// BatchEmbed embeds multiple texts, using cache where available.
// Returns a map from input text to its embedding vector.
func (c *EmbeddingCache) BatchEmbed(ctx context.Context, texts []string) (map[string][]float32, error) {
	results := make(map[string][]float32, len(texts))
	var uncached []string

	// Check cache for all texts.
	for _, text := range texts {
		key := c.prefix + text
		if cached, err := c.backend.Get(ctx, key); err == nil && cached != "" {
			var vec []float32
			if err := json.Unmarshal([]byte(cached), &vec); err == nil {
				results[text] = vec
				continue
			}
		}
		uncached = append(uncached, text)
	}

	// Embed uncached texts.
	for _, text := range uncached {
		vec, err := c.embedder.Embed(ctx, text)
		if err != nil {
			return results, err
		}
		results[text] = vec
		if data, err := json.Marshal(vec); err == nil {
			_ = c.backend.Set(ctx, c.prefix+text, string(data), c.ttl)
		}
	}

	return results, nil
}

// Stats returns cache statistics. Currently returns placeholder data.
func (c *EmbeddingCache) Stats() map[string]any {
	return map[string]any{
		"prefix":    c.prefix,
		"ttl":       strconv.Itoa(c.ttl),
		"dim":       c.embedder.Dim(),
		"cache_type": "embedding_cache",
	}
}
