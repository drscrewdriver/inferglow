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

package model

import (
	"fmt"
	"sync"
)

// ModelRequesterRegistry 管理已注册的 LLM Provider
type ModelRequesterRegistry struct { //nolint:revive
	mu        sync.RWMutex
	providers map[string]ModelRequester
}

// NewModelRequesterRegistry 创建新的注册表
func NewModelRequesterRegistry() *ModelRequesterRegistry {
	return &ModelRequesterRegistry{
		providers: make(map[string]ModelRequester),
	}
}

// Register 注册一个 Provider
func (r *ModelRequesterRegistry) Register(provider ModelRequester) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}

	r.providers[name] = provider
	return nil
}

// Get 按名称获取已注册的 Provider
func (r *ModelRequesterRegistry) Get(name string) (ModelRequester, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return provider, nil
}

// List 列出所有已注册的 Provider 名称
func (r *ModelRequesterRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
