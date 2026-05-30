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

package blocks

import (
	"context"
	"fmt"
	"sync"
)

// BlockRegistry manages a collection of named blocks.
type BlockRegistry struct {
	mu     sync.RWMutex
	blocks map[string]FlowBlock
}

// NewBlockRegistry creates an empty registry.
func NewBlockRegistry() *BlockRegistry {
	return &BlockRegistry{
		blocks: make(map[string]FlowBlock),
	}
}

// Register adds a block to the registry.
func (r *BlockRegistry) Register(b FlowBlock, replace bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := b.Name()
	if _, exists := r.blocks[name]; exists && !replace {
		return fmt.Errorf("%w: %s", ErrBlockExists, name)
	}
	r.blocks[name] = b
	return nil
}

// Get retrieves a block by name.
func (r *BlockRegistry) Get(name string) (FlowBlock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.blocks[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrBlockNotFound, name)
	}
	return b, nil
}

// List returns all registered block names.
func (r *BlockRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.blocks))
	for name := range r.blocks {
		names = append(names, name)
	}
	return names
}

// ExecuteBlueprint runs a blueprint by executing each block in sequence.
func (r *BlockRegistry) ExecuteBlueprint(ctx context.Context, bp *BlockBlueprint, input any) (any, error) {
	current := input
	for _, ref := range bp.Blocks {
		b, err := r.Get(ref.BlockName)
		if err != nil {
			return nil, err
		}
		result, err := b.Execute(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("%w: block %s: %v", ErrBlockExecFail, ref.BlockName, err)
		}
		current = result
	}
	return current, nil
}
