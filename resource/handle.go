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

package resource

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrResourceClosed is returned when Execute or HealthCheck is called on
// a closed resource handle.
var ErrResourceClosed = errors.New("resource handle is closed")

// ErrResourceFailed is returned when the underlying resource has entered
// a failed state and can no longer serve requests.
var ErrResourceFailed = errors.New("resource handle is in failed state")

// ResourceHandle wraps a Resource with lifecycle tracking, reuse support,
// and health monitoring. It is the token returned by ResourceManager.Declare
// and ResourceManager.Ensure.
type ResourceHandle struct {
	mu         sync.Mutex
	id         string
	resource   Resource
	state      string
	scope      string
	createdAt  time.Time
	lastUsedAt time.Time
	useCount   int64
	healthy    bool
	healthErr  string
	closed     bool
}

// newHandle creates a ResourceHandle in the "creating" state.
func newHandle(id string, res Resource, scope string) *ResourceHandle {
	return &ResourceHandle{
		id:        id,
		resource:  res,
		state:     StateCreating,
		scope:     scope,
		createdAt: time.Now(),
		healthy:   true,
	}
}

// ID returns the handle's unique identifier.
func (h *ResourceHandle) ID() string { return h.id }

// Scope returns the handle's release scope.
func (h *ResourceHandle) Scope() string { return h.scope }

// State returns the current lifecycle state.
func (h *ResourceHandle) State() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// UseCount returns the number of times Execute has been called.
func (h *ResourceHandle) UseCount() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.useCount
}

// Execute delegates to the underlying resource after state validation.
// It transitions the handle to "busy" during execution and to "idle"
// afterward. UseCount is incremented on each call.
func (h *ResourceHandle) Execute(ctx context.Context, cmd any) (*ResourceResult, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrResourceClosed
	}
	if h.state == StateFailed {
		h.mu.Unlock()
		return nil, ErrResourceFailed
	}
	h.state = StateBusy
	h.mu.Unlock()

	result, err := h.resource.Execute(ctx, cmd)

	h.mu.Lock()
	h.useCount++
	h.lastUsedAt = time.Now()
	if err != nil {
		h.state = StateFailed
		h.healthy = false
		h.healthErr = err.Error()
	} else {
		h.state = StateIdle
	}
	h.mu.Unlock()

	return result, err
}

// HealthCheck delegates to the underlying resource and updates the
// handle's health status.
func (h *ResourceHandle) HealthCheck(ctx context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ErrResourceClosed
	}
	h.mu.Unlock()

	err := h.resource.HealthCheck(ctx)

	h.mu.Lock()
	if err != nil {
		h.healthy = false
		h.healthErr = err.Error()
		h.state = StateFailed
	} else {
		h.healthy = true
		h.healthErr = ""
	}
	h.mu.Unlock()

	return err
}

// Close releases the underlying resource. Subsequent Execute and
// HealthCheck calls return ErrResourceClosed.
func (h *ResourceHandle) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.state = StateClosed
	h.mu.Unlock()
	return h.resource.Close()
}

// Status returns a snapshot of the handle's current state.
func (h *ResourceHandle) Status() ResourceStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ResourceStatus{
		HandleID:     h.id,
		ResourceType: h.resource.Type(),
		State:        h.state,
		Scope:        h.scope,
		CreatedAt:    h.createdAt,
		LastUsedAt:   h.lastUsedAt,
		UseCount:     h.useCount,
		Healthy:      h.healthy,
		HealthError:  h.healthErr,
	}
}

// MarkReady transitions the handle from "creating" to "ready".
func (h *ResourceHandle) MarkReady() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == StateCreating {
		h.state = StateReady
	}
}

// Resource returns the underlying Resource. Use with caution — prefer
// Execute through the handle for proper lifecycle tracking.
func (h *ResourceHandle) Resource() Resource {
	return h.resource
}
