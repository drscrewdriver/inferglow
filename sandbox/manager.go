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

// autoFallbackOrder is the order of preference for ModeAuto selection.
// gvisor → docker → local → trusted_local
var autoFallbackOrder = []SandboxMode{ModeGVisor, ModeDocker, ModeLocal, ModeTrustedLocal}

// SelectSandbox picks a Provider for the given mode.
//
// For ModeAuto, it walks the fallback chain (gvisor → docker → local →
// trusted_local), calling InspectAvailability on each registered provider,
// returning the first available one. Returns ErrNoAvailableSandbox if none
// are available.
func (m *Manager) SelectSandbox(mode SandboxMode) (Provider, error) {
    if mode == ModeAuto {
        m.mu.RLock()
        defer m.mu.RUnlock()
        for _, candidate := range autoFallbackOrder {
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
