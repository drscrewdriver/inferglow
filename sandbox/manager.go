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

package sandbox

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrProviderAlreadyRegistered returned when registering a duplicate provider name.
var ErrProviderAlreadyRegistered = errors.New("provider already registered")

// Manager manages a registry of sandbox Providers and supports
// sandbox mode selection (including the "auto" fallback chain).
type Manager struct {
	mu          sync.RWMutex
	providers   map[string]Provider
	defaultMode SandboxMode
	// AllowTrustedFallback gates whether ModeAuto may fall back to the
	// trusted_local backend, which provides no isolation. It defaults to
	// false so that ModeAuto returns an error rather than silently executing
	// unisolated commands. Set to true only in explicitly trusted
	// environments.
	AllowTrustedFallback bool
}

// NewManager returns an empty Manager ready to register providers.
func NewManager() *Manager {
	return &Manager{
		providers:   make(map[string]Provider),
		defaultMode: ModeAuto,
	}
}

// Register adds a Provider to the registry.
// Returns ErrProviderAlreadyRegistered if name conflicts.
func (m *Manager) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("%w: provider is nil", ErrProviderAlreadyRegistered)
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("%w: provider name is empty", ErrProviderAlreadyRegistered)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("%w: %q", ErrProviderAlreadyRegistered, name)
	}
	m.providers[name] = p
	return nil
}

// Get retrieves a registered Provider by name.
func (m *Manager) Get(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	return p, nil
}

// List returns the sorted names of every registered Provider.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// autoFallbackOrder is the order of preference for ModeAuto selection among
// isolated backends: gvisor → docker → local. trusted_local is intentionally
// excluded because it provides no isolation; it is only appended to the chain
// when Manager.AllowTrustedFallback is true.
var autoFallbackOrder = []SandboxMode{ModeGVisor, ModeDocker, ModeLocal}

// SelectSandbox picks a Provider for the given mode.
//
// For ModeAuto, it walks the isolated fallback chain (gvisor → docker →
// local), calling InspectAvailability on each registered provider and
// returning the first available one. trusted_local is only considered when
// Manager.AllowTrustedFallback is true. Returns ErrNoAvailableSandbox if no
// suitable (isolated) backend is available — it never silently falls back to
// the no-isolation trusted_local backend unless that fallback is explicitly
// allowed.
func (m *Manager) SelectSandbox(mode SandboxMode) (Provider, error) {
	if mode == ModeAuto {
		m.mu.RLock()
		defer m.mu.RUnlock()
		chain := autoFallbackOrder
		if m.AllowTrustedFallback {
			chain = append(append([]SandboxMode{}, autoFallbackOrder...), ModeTrustedLocal)
		}
		for _, candidate := range chain {
			p, ok := m.providers[string(candidate)]
			if !ok {
				continue
			}
			avail, err := p.InspectAvailability()
			if err != nil {
				continue
			}
			if avail != nil && avail.Available {
				return p, nil
			}
		}
		return nil, fmt.Errorf("%w: no provider available in auto chain", ErrNoAvailableSandbox)
	}
	// Explicit mode
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[string(mode)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, mode)
	}
	return p, nil
}

// CreateHandle is a convenience that selects a Provider for the mode and
// then creates a Handle via the Provider.
func (m *Manager) CreateHandle(mode SandboxMode, cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	p, err := m.SelectSandbox(mode)
	if err != nil {
		return nil, err
	}
	return p.CreateHandle(cfg, policy)
}
