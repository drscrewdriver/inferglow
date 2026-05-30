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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// Sentinel errors returned by ResourceManager operations.
var (
	ErrProviderNotFound  = errors.New("no provider registered for resource type")
	ErrProviderExists   = errors.New("provider already registered for resource type")
	ErrCapabilityMismatch = errors.New("provider does not satisfy required capabilities")
	ErrHandleNotFound   = errors.New("resource handle not found")
)

// ResourceProvider creates and manages resources of a specific type.
// Implementations wrap concrete runtime environments.
type ResourceProvider interface {
	// Type returns the resource type this provider creates (e.g. "bash").
	Type() string

	// Capabilities returns the capability set this provider supports.
	// The ResourceManager uses this to match against Requirement.Capabilities.
	Capabilities() []string

	// Create instantiates a new resource with the given configuration.
	Create(ctx context.Context, config ResourceConfig) (Resource, error)

	// Probe checks whether this provider can create resources in the
	// current environment (e.g. is the Python binary available?). Returns
	// nil if the provider is operational.
	Probe(ctx context.Context) error
}

// ResourceManager is the central registry for resource providers and
// handles. It selects providers by type + capability, reuses idle handles,
// and supports scope-based batch release.
type ResourceManager struct {
	mu        sync.Mutex
	providers map[string]ResourceProvider
	handles   map[string]*ResourceHandle
	// reuseKey maps "type:prop1=val1:prop2=val2" to an idle handle ID
	// for handle reuse on matching requirements.
	reuseKey  map[string]string
}

// NewResourceManager creates an empty manager with no providers.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		providers: make(map[string]ResourceProvider),
		handles:   make(map[string]*ResourceHandle),
		reuseKey:  make(map[string]string),
	}
}

// RegisterProvider adds a provider for its resource type. If replace is
// false and a provider for the same type already exists, returns
// ErrProviderExists.
func (m *ResourceManager) RegisterProvider(p ResourceProvider, replace bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := p.Type()
	if _, exists := m.providers[t]; exists && !replace {
		return fmt.Errorf("%w: %s", ErrProviderExists, t)
	}
	m.providers[t] = p
	return nil
}

// Declare allocates a ResourceHandle for the given requirement without
// necessarily creating the underlying resource immediately. If a matching
// idle handle exists it is reused.
func (m *ResourceManager) Declare(req Requirement) (*ResourceHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Try reuse first.
	if key := m.reuseKeyFor(req); key != "" {
		if hid, ok := m.reuseKey[key]; ok {
			if h, found := m.handles[hid]; found && h.State() == StateIdle {
				return h, nil
			}
			// Stale entry — clean up.
			delete(m.reuseKey, key)
		}
	}

	// No reusable handle — create a placeholder. The actual resource
	// will be created by Ensure.
	id := generateID()
	h := newHandle(id, nil, req.Scope)
	m.handles[id] = h
	return h, nil
}

// Ensure creates the underlying resource if the handle does not yet have
// one, or returns the existing handle if it is still healthy.
func (m *ResourceManager) Ensure(ctx context.Context, req Requirement) (*ResourceHandle, error) {
	m.mu.Lock()
	provider, ok := m.providers[req.Type]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, req.Type)
	}

	// Capability check.
	if !satisfiesCapabilities(provider, req.Capabilities) {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: type=%s", ErrCapabilityMismatch, req.Type)
	}

	// Try reuse.
	if key := m.reuseKeyFor(req); key != "" {
		if hid, found := m.reuseKey[key]; found {
			if h, exists := m.handles[hid]; exists {
				state := h.State()
				if (state == StateIdle || state == StateReady) && h.Resource() != nil {
					m.mu.Unlock()
					return h, nil
				}
				delete(m.reuseKey, key)
			}
		}
	}
	m.mu.Unlock()

	// Create new resource.
	config := ResourceConfig{
		Type:       req.Type,
		Properties: req.Properties,
	}
	res, err := provider.Create(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("resource create failed: %w", err)
	}

	id := generateID()
	h := newHandle(id, res, req.Scope)
	h.MarkReady()

	m.mu.Lock()
	m.handles[id] = h
	if key := m.reuseKeyFor(req); key != "" {
		m.reuseKey[key] = id
	}
	m.mu.Unlock()

	return h, nil
}

// Release closes and removes a single handle.
func (m *ResourceManager) Release(handle *ResourceHandle) error {
	if handle == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releaseLocked(handle.ID())
}

// ReleaseScope closes and removes all handles with the given scope.
func (m *ResourceManager) ReleaseScope(scope string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for id, h := range m.handles {
		if h.Scope() == scope {
			ids = append(ids, id)
		}
	}
	var firstErr error
	for _, id := range ids {
		if err := m.releaseLocked(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// releaseLocked closes and removes a handle. Caller must hold m.mu.
func (m *ResourceManager) releaseLocked(id string) error {
	h, ok := m.handles[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrHandleNotFound, id)
	}
	delete(m.handles, id)
	// Clean reuse map.
	for k, v := range m.reuseKey {
		if v == id {
			delete(m.reuseKey, k)
		}
	}
	return h.Close()
}

// Inspect returns the current status of a handle.
func (m *ResourceManager) Inspect(handle *ResourceHandle) ResourceStatus {
	if handle == nil {
		return ResourceStatus{}
	}
	return handle.Status()
}

// List returns a snapshot of all active handles.
func (m *ResourceManager) List() []*ResourceHandle {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*ResourceHandle, 0, len(m.handles))
	for _, h := range m.handles {
		out = append(out, h)
	}
	return out
}

// CloseAll releases all handles and clears the provider registry.
func (m *ResourceManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for id := range m.handles {
		if err := m.releaseLocked(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// reuseKeyFor builds a map key for handle reuse matching. Returns ""
// when the requirement has no properties (reuse disabled).
func (m *ResourceManager) reuseKeyFor(req Requirement) string {
	if len(req.Properties) == 0 {
		return req.Type
	}
	key := req.Type
	for k, v := range req.Properties {
		key += ":" + k + "=" + v
	}
	return key
}

// satisfiesCapabilities checks that the provider's capability set is a
// superset of the required capabilities.
func satisfiesCapabilities(p ResourceProvider, required []string) bool {
	if len(required) == 0 {
		return true
	}
	have := make(map[string]bool, len(p.Capabilities()))
	for _, c := range p.Capabilities() {
		have[c] = true
	}
	for _, c := range required {
		if !have[c] {
			return false
		}
	}
	return true
}

// generateID produces a random 16-byte hex identifier.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
