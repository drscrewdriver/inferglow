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

package taskdag

import (
	"context"
	"fmt"
	"sync"
)

// Handler executes a task node.
type Handler interface {
	Execute(ctx context.Context, tctx *TaskDAGContext) (any, error)
}

// HandlerResolver resolves a task node to its handler.
type HandlerResolver interface {
	Resolve(node *TaskNode) (Handler, error)
}

// StaticResolver maps task kinds to fixed handlers.
type StaticResolver struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewStaticResolver creates a resolver with no handlers.
func NewStaticResolver() *StaticResolver {
	return &StaticResolver{handlers: make(map[string]Handler)}
}

// Register adds a handler for a given kind.
func (r *StaticResolver) Register(kind string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = h
}

// Resolve looks up a handler by the node's Kind field.
func (r *StaticResolver) Resolve(node *TaskNode) (Handler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[node.Kind]
	if !ok {
		return nil, fmt.Errorf("%w: kind=%q", ErrHandlerNotFound, node.Kind)
	}
	return h, nil
}
