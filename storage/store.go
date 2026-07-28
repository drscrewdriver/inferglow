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

// Package storage provides a generic, concurrency-safe key-value storage
// primitive (read/write/query/iterate) shared by the server's in-memory stores.
package storage

import "sync"

// Store is a generic KV storage interface (read / write / query / iterate).
type Store[K comparable, V any] interface {
	Get(key K) (V, bool) // read: value and whether the key exists
	Set(key K, val V)    // write: insert or update
	Delete(key K)        // delete: removing a missing key is a no-op
	Len() int            // query: number of entries
	Keys() []K           // iterate: all keys
	Values() []V         // iterate: all values
}

// Map is a concurrency-safe generic KV primitive backed by a Go map and a
// sync.RWMutex. The zero value is not usable; construct with NewMap.
type Map[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewMap creates an empty concurrency-safe Map.
func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{m: make(map[K]V)}
}

// Get returns the value stored under key and whether the key is present.
func (m *Map[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.m[key]
	return v, ok
}

// Set inserts or updates the value stored under key.
func (m *Map[K, V]) Set(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = val
}

// Delete removes the entry under key. Deleting a missing key is a no-op.
func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

// Len returns the number of entries.
func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.m)
}

// Keys returns all keys in the map (unordered).
func (m *Map[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]K, 0, len(m.m))
	for k := range m.m {
		out = append(out, k)
	}
	return out
}

// Values returns all values in the map (unordered).
func (m *Map[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]V, 0, len(m.m))
	for _, v := range m.m {
		out = append(out, v)
	}
	return out
}

// Compile-time assertion: *Map implements the Store interface.
var _ Store[string, any] = (*Map[string, any])(nil)