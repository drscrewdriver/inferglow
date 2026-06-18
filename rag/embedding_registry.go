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

package rag

import (
	"fmt"
	"sort"
	"sync"
)

// embeddingRegistry is the global registry of EmbeddingModel factories.
var embeddingRegistry = struct {
	mu      sync.RWMutex
	factory map[string]func() EmbeddingModel
}{
	factory: make(map[string]func() EmbeddingModel),
}

// RegisterEmbeddingModel registers an EmbeddingModel factory under the given name.
// It panics if the name is already registered.
func RegisterEmbeddingModel(name string, factory func() EmbeddingModel) {
	embeddingRegistry.mu.Lock()
	defer embeddingRegistry.mu.Unlock()
	if _, exists := embeddingRegistry.factory[name]; exists {
		panic(fmt.Sprintf("rag: embedding model %q already registered", name))
	}
	embeddingRegistry.factory[name] = factory
}

// GetEmbeddingModel returns the EmbeddingModel registered under the given name.
func GetEmbeddingModel(name string) (EmbeddingModel, error) {
	embeddingRegistry.mu.RLock()
	defer embeddingRegistry.mu.RUnlock()
	factory, ok := embeddingRegistry.factory[name]
	if !ok {
		return nil, fmt.Errorf("rag: unknown embedding model %q", name)
	}
	return factory(), nil
}

// ListEmbeddingModels returns the names of all registered embedding models.
func ListEmbeddingModels() []string {
	embeddingRegistry.mu.RLock()
	defer embeddingRegistry.mu.RUnlock()
	names := make([]string, 0, len(embeddingRegistry.factory))
	for name := range embeddingRegistry.factory {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
