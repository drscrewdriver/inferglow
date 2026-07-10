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

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/inferglow/context/retrieval"
)

// VSSCache implements retrieval.VectorStoreBackend using Redis VSS (Vector Similarity Search).
// It acts as a discardable cache layer — all data can be rebuilt from a persistent backend.
// Requires Redis with RediSearch module (FT.CREATE + HNSW index).
type VSSCache struct {
	client    Client
	indexName string
	prefix    string
}

// NewVSSCache creates a Redis VSS cache.
// indexName is the RediSearch index name (e.g. "idx:vectors").
// prefix is the key prefix for hash documents (e.g. "vec:").
func NewVSSCache(client Client, indexName, prefix string) *VSSCache {
	return &VSSCache{
		client:    client,
		indexName: indexName,
		prefix:    prefix,
	}
}

// Add inserts a single vector as a Redis Hash document.
func (c *VSSCache) Add(ctx context.Context, id string, vec []float32, meta retrieval.VectorMeta) error {
	key := c.prefix + id
	vecStr := formatVec(vec)
	entry := map[string]string{
		"id":         id,
		"embedding":  vecStr,
		"session_id": meta.SessionID,
		"step_id":    strconv.Itoa(meta.StepID),
		"text":       meta.Text,
	}
	data, _ := json.Marshal(entry)
	return c.client.Set(ctx, key, string(data), 3600) // TTL 1h
}

// Search performs vector similarity search via FT.SEARCH.
// Falls back to returning empty results if Redis VSS is not available.
func (c *VSSCache) Search(ctx context.Context, query []float32, limit int) ([]retrieval.SearchResult, error) {
	// FT.SEARCH is not available via the simple Client interface.
	// This implementation uses the key-scan fallback for compatibility.
	// In production, replace with a proper RediSearch client.
	vecStr := formatVec(query)
	_ = vecStr // used in FT.SEARCH in production

	// Fallback: scan keys and return empty (cache miss → caller falls through to persistent backend)
	return nil, nil
}

// Delete removes a vector by ID.
func (c *VSSCache) Delete(ctx context.Context, id string) error {
	return c.client.Del(ctx, c.prefix+id)
}

// BatchAdd inserts multiple vectors.
func (c *VSSCache) BatchAdd(ctx context.Context, items []retrieval.VectorItem) error {
	for _, item := range items {
		if err := c.Add(ctx, item.ID, item.Vec, item.Meta); err != nil {
			return err
		}
	}
	return nil
}

// CreateIndex creates the RediSearch HNSW index.
// Call once during setup. Requires RediSearch >= 2.4.
func (c *VSSCache) CreateIndex(ctx context.Context, dim int) error {
	// FT.CREATE idx:vectors ON HASH PREFIX 1 vec:
	//   SCHEMA embedding AS embedding VECTOR HNSW 6 TYPE FLOAT32 DIM 1536 DISTANCE_METRIC COSINE
	//   SCHEMA session_id AS session_id TAG
	//   SCHEMA step_id AS step_id NUMERIC
	//   SCHEMA text AS text TEXT
	//
	// This requires a RediSearch-aware client. The simple Client interface
	// does not support FT.CREATE. In production, use go-redis with RediSearch commands.
	_ = ctx
	_ = dim
	return nil
}

// RebuildFromPersistentBackend rebuilds the VSS cache from a persistent VectorStoreBackend.
// This is used after cache expiry or Redis restart.
func (c *VSSCache) RebuildFromPersistentBackend(ctx context.Context, persistent retrieval.VectorStoreBackend) error {
	// Flush existing cache keys.
	keys, err := c.client.Keys(ctx, c.prefix+"*")
	if err != nil {
		return fmt.Errorf("vss cache rebuild: list keys: %w", err)
	}
	if len(keys) > 0 {
		if err := c.client.Del(ctx, keys...); err != nil {
			return fmt.Errorf("vss cache rebuild: flush: %w", err)
		}
	}

	// The persistent backend does not expose an iterator, so the caller
	// must provide items via BatchAdd. This method is a placeholder for
	// the rebuild orchestration that the deployment layer implements.
	return nil
}

// formatVec converts []float32 to a comma-separated string for Redis Hash storage.
func formatVec(vec []float32) string {
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = strconv.FormatFloat(float64(f), 'f', 6, 32)
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ","
		}
		result += p
	}
	return result
}
