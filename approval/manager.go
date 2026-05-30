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

package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Sentinel errors.
var (
	ErrHandlerNotFound = errors.New("approval handler not found")
	ErrHandlerExists   = errors.New("approval handler already registered")
	ErrNoDefaultHandler = errors.New("no default approval handler set")
)

// PolicyApprovalManager is the central approval authority. It maintains
// a registry of ApprovalHandler implementations and routes requests
// through the configured default or per-request handler.
type PolicyApprovalManager struct {
	mu             sync.RWMutex
	handlers       map[string]ApprovalHandler
	defaultHandler string
}

// NewPolicyApprovalManager creates a manager with no handlers registered.
func NewPolicyApprovalManager() *PolicyApprovalManager {
	return &PolicyApprovalManager{
		handlers: make(map[string]ApprovalHandler),
	}
}

// RegisterHandler adds a handler to the registry. If replace is false
// and a handler with the same name exists, returns ErrHandlerExists.
func (m *PolicyApprovalManager) RegisterHandler(h ApprovalHandler, replace bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := h.Name()
	if _, exists := m.handlers[name]; exists && !replace {
		return fmt.Errorf("%w: %s", ErrHandlerExists, name)
	}
	m.handlers[name] = h
	return nil
}

// UnregisterHandler removes a handler by name. Returns true if the
// handler was found and removed.
func (m *PolicyApprovalManager) UnregisterHandler(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[name]; !ok {
		return false
	}
	delete(m.handlers, name)
	if m.defaultHandler == name {
		m.defaultHandler = ""
	}
	return true
}

// SetDefaultHandler sets the handler used when Resolve is called without
// an explicit handler name. The handler must already be registered.
func (m *PolicyApprovalManager) SetDefaultHandler(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[name]; !ok {
		return fmt.Errorf("%w: %s", ErrHandlerNotFound, name)
	}
	m.defaultHandler = name
	return nil
}

// DefaultHandler returns the name of the current default handler, or
// empty string if none is set.
func (m *PolicyApprovalManager) DefaultHandler() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultHandler
}

// Resolve evaluates an approval request using the specified handler. If
// handlerName is empty, the default handler is used. The ctx parameter
// is reserved for future async handlers (e.g. interactive approval).
func (m *PolicyApprovalManager) Resolve(_ context.Context, req *Request, handlerName string) (*Decision, error) {
	m.mu.RLock()
	name := handlerName
	if name == "" {
		name = m.defaultHandler
	}
	if name == "" {
		m.mu.RUnlock()
		return nil, ErrNoDefaultHandler
	}
	h, ok := m.handlers[name]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrHandlerNotFound, name)
	}

	// Apply access policy pre-checks.
	if req.Policy != nil {
		if d := evaluatePolicy(req); d != nil {
			d.Handler = name
			return d, nil
		}
	}

	return h.Resolve(req)
}

// ListHandlers returns the names of all registered handlers.
func (m *PolicyApprovalManager) ListHandlers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.handlers))
	for name := range m.handlers {
		names = append(names, name)
	}
	return names
}


