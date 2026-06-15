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

// Package redis implements the Redis caching layer for contextmgr (§7.2).
//
// Redis is used as a discardable/rebuildable cache layer for:
//   - Semantic vector search (VSS)
//   - Keyword search (FT)
//   - Recency-based retrieval (ZSet)
//   - Rendered block caching
//
// All data in Redis can be rebuilt from the JSONL/SQLite/PostgreSQL store.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/inferglow/session/contextmgr"
)

// Client is the Redis client interface (abstraction over go-redis).
type Client interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl int) error
	Del(ctx context.Context, keys ...string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
}

// CacheStore implements a Redis-backed cache layer for contextmgr.
// It is discardable and rebuildable from the persistent store.
type CacheStore struct {
	mu        sync.RWMutex
	client    Client
	sessionID string
}

// NewCacheStore creates a new Redis cache store.
func NewCacheStore(client Client, sessionID string) *CacheStore {
	return &CacheStore{
		client:    client,
		sessionID: sessionID,
	}
}

func (c *CacheStore) key(suffix string) string {
	return fmt.Sprintf("ctx:%s:%s", c.sessionID, suffix)
}

// CacheStep caches a step record.
func (c *CacheStore) CacheStep(ctx context.Context, step contextmgr.StepRecord) error {
	data, err := json.Marshal(step)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(fmt.Sprintf("step:%d", step.StepID)), string(data), 0)
}

// GetCachedStep retrieves a cached step record.
func (c *CacheStore) GetCachedStep(ctx context.Context, stepID int) (*contextmgr.StepRecord, error) {
	data, err := c.client.Get(ctx, c.key(fmt.Sprintf("step:%d", stepID)))
	if err != nil {
		return nil, err
	}
	var rec contextmgr.StepRecord
	if err := json.Unmarshal([]byte(data), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// CacheRenderedBlock caches a rendered block.
func (c *CacheStore) CacheRenderedBlock(ctx context.Context, stepID int, level int, block string) error {
	entry := struct {
		StepID int    `json:"step_id"`
		Level  int    `json:"level"`
		Block  string `json:"block"`
	}{stepID, level, block}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(fmt.Sprintf("render:%d", stepID)), string(data), 0)
}

// GetCachedRenderedBlock retrieves a cached rendered block.
func (c *CacheStore) GetCachedRenderedBlock(ctx context.Context, stepID int, currentLevel int) (string, error) {
	data, err := c.client.Get(ctx, c.key(fmt.Sprintf("render:%d", stepID)))
	if err != nil {
		return "", err
	}
	var entry struct {
		StepID int    `json:"step_id"`
		Level  int    `json:"level"`
		Block  string `json:"block"`
	}
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return "", err
	}
	if entry.Level != currentLevel {
		return "", fmt.Errorf("cache stale: level mismatch")
	}
	return entry.Block, nil
}

// Flush clears all cached data for this session.
func (c *CacheStore) Flush(ctx context.Context) error {
	keys, err := c.client.Keys(ctx, fmt.Sprintf("ctx:%s:*", c.sessionID))
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...)
	}
	return nil
}

// RebuildFromStore rebuilds the Redis cache from a persistent store.
func (c *CacheStore) RebuildFromStore(ctx context.Context, store contextmgr.StepStoreLike) error {
	if err := c.Flush(ctx); err != nil {
		return err
	}

	ids, err := store.AllActiveStepIDs()
	if err != nil {
		return err
	}

	for _, id := range ids {
		step, err := store.GetStep(id)
		if err != nil {
			continue
		}
		_ = c.CacheStep(ctx, *step)
	}

	return nil
}
