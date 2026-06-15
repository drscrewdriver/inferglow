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
	"crypto/sha256"
	"fmt"
	"sync"
)

// RenderedCache caches rendered blocks to avoid re-reading .lN.jsonl for unchanged steps (§4.6.1).
type RenderedCache struct {
	mu    sync.RWMutex
	items map[int]*RenderedCacheEntry
}

// RenderedCacheEntry is a single cached rendered block.
type RenderedCacheEntry struct {
	StepID int    `json:"step_id"`
	Level  int    `json:"level"`    // level at time of rendering
	Block  string `json:"block"`    // fully rendered text (with ⟨§N·type·Lx⟩ marker)
	Hash   string `json:"hash"`     // content hash for validation
}

// NewRenderedCache creates a new rendered cache.
func NewRenderedCache() *RenderedCache {
	return &RenderedCache{
		items: make(map[int]*RenderedCacheEntry),
	}
}

// Get retrieves a cached rendered block if the level matches.
func (c *RenderedCache) Get(stepID int, currentLevel int) *RenderedCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[stepID]
	if !ok {
		return nil
	}
	if entry.Level != currentLevel {
		return nil // level changed, cache invalid
	}
	return entry
}

// Set stores a rendered block in the cache.
func (c *RenderedCache) Set(stepID int, level int, block string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := computeHash(block)
	c.items[stepID] = &RenderedCacheEntry{
		StepID: stepID,
		Level:  level,
		Block:  block,
		Hash:   hash,
	}
}

// InvalidateAll clears the entire cache (e.g., on head_buffer version change).
func (c *RenderedCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[int]*RenderedCacheEntry)
}

// Invalidate removes a single step from the cache.
func (c *RenderedCache) Invalidate(stepID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, stepID)
}

// Len returns the number of cached entries.
func (c *RenderedCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func computeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// RenderStepWithCache renders a step using the cache for fast-path (§4.6.1).
func RenderStepWithCache(stepID int, ref RefRecord, cache *RenderedCache, store StepStoreLike) (RenderedBlock, error) {
	// Fast path: cache hit
	if cached := cache.Get(stepID, ref.Level); cached != nil {
		return RenderedBlock{
			StepID:  stepID,
			Level:   ref.Level,
			Content: cached.Block,
		}, nil
	}

	// Slow path: read from store and render
	content, err := renderFromStore(stepID, ref.Level, store)
	if err != nil {
		return RenderedBlock{}, err
	}

	step, _ := store.GetStep(stepID)
	typ := "reasoning"
	if step != nil {
		typ = step.Type
	}

	block := renderBlock(stepID, ref.Level, content, typ)

	// Update cache
	cache.Set(stepID, ref.Level, block.Content)

	return block, nil
}

// renderFromStore reads content at the appropriate level from the store.
func renderFromStore(stepID, level int, store StepStoreLike) (string, error) {
	switch level {
	case 0:
		step, err := store.GetStep(stepID)
		if err != nil {
			return "", err
		}
		return step.Content, nil
	case 1:
		rec, err := store.GetL1(stepID)
		if err != nil {
			step, err2 := store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Content, nil
	case 2:
		rec, err := store.GetL2(stepID)
		if err != nil {
			step, err2 := store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		result := ""
		for i, f := range rec.Facts {
			if i > 0 {
				result += "\n"
			}
			result += "• " + f
		}
		return result, nil
	case 3:
		rec, err := store.GetL3(stepID)
		if err != nil {
			step, err2 := store.GetStep(stepID)
			if err2 != nil {
				return "", err
			}
			return step.Content, nil
		}
		return rec.Mask, nil
	default:
		return "", fmt.Errorf("unknown level %d", level)
	}
}
