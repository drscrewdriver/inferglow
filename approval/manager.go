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
	"time"
)

// Sentinel errors.
var (
	ErrHandlerNotFound  = errors.New("approval handler not found")
	ErrHandlerExists    = errors.New("approval handler already registered")
	ErrNoDefaultHandler = errors.New("no default approval handler set")
	ErrRecordNotFound   = errors.New("approval record not found")
	ErrAlreadyResolved  = errors.New("approval already resolved")
)

// PolicyApprovalManager is the central approval authority. It maintains
// a registry of ApprovalHandler implementations and routes requests
// through the configured default or per-request handler.
//
// In addition to the synchronous Resolve flow, the manager supports a
// record-based flow via Submit: requests that cannot be resolved
// immediately (policy undecided and no handler, or handler returns
// pending) are stored as pending records for later manual resolution
// via ResolveRecord.
type PolicyApprovalManager struct {
	mu             sync.RWMutex
	handlers       map[string]ApprovalHandler
	defaultHandler string
	policy         *AccessPolicy
	records        map[string]*Record
	seq            int64
}

// NewPolicyApprovalManager creates a manager with no handlers registered.
func NewPolicyApprovalManager() *PolicyApprovalManager {
	return &PolicyApprovalManager{
		handlers: make(map[string]ApprovalHandler),
		records:  make(map[string]*Record),
	}
}

// SetPolicy configures the global access policy applied to every
// request in addition to any per-request policy. Pass nil to clear.
func (m *PolicyApprovalManager) SetPolicy(p *AccessPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policy = p
}

// effectivePolicy returns the merged global + per-request policy, or
// nil if neither is set.
func (m *PolicyApprovalManager) effectivePolicy(reqPolicy *AccessPolicy) *AccessPolicy {
	m.mu.RLock()
	global := m.policy
	m.mu.RUnlock()
	if global == nil && reqPolicy == nil {
		return nil
	}
	if global == nil {
		return reqPolicy
	}
	if reqPolicy == nil {
		return global
	}
	return MergePolicies(global, reqPolicy)
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

	// Apply access policy pre-checks (global + per-request).
	effective := m.effectivePolicy(req.Policy)
	if effective != nil {
		reqCopy := *req
		reqCopy.Policy = effective
		if d := evaluatePolicy(&reqCopy); d != nil {
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

// Submit creates a new approval request record, evaluates it against
// the access policy (global + per-request) and the default handler,
// and returns the resulting Record.
//
//   - If the policy or handler resolves the request immediately
//     (approved/denied/allowed), the record is returned without being
//     stored.
//   - If the request remains pending (no handler, or handler returns
//     pending), the record is stored for later manual resolution via
//     ResolveRecord.
func (m *PolicyApprovalManager) Submit(req *Request) (*Record, error) {
	now := time.Now()

	// Apply access policy pre-checks (global + per-request).
	effective := m.effectivePolicy(req.Policy)
	if effective != nil {
		reqCopy := *req
		reqCopy.Policy = effective
		if d := evaluatePolicy(&reqCopy); d != nil {
			return &Record{
				Request:   req,
				Status:    d.Status,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		}
	}

	// Try the default handler for a synchronous decision.
	m.mu.RLock()
	name := m.defaultHandler
	h, hasDefault := m.handlers[name]
	m.mu.RUnlock()
	if hasDefault {
		d, err := h.Resolve(req)
		if err != nil {
			return nil, err
		}
		if d.Status != DecisionPending {
			return &Record{
				Request:   req,
				Status:    d.Status,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		}
	}

	// Store pending record for manual resolution.
	m.mu.Lock()
	m.seq++
	id := fmt.Sprintf("approval_%d", m.seq)
	record := &Record{
		ID:        id,
		Request:   req,
		Status:    DecisionPending,
		CreatedAt: now,
	}
	m.records[id] = record
	m.mu.Unlock()
	return record, nil
}

// ResolveRecord manually resolves a pending approval record. If approved
// is true the record status becomes DecisionApproved; otherwise
// DecisionDenied. The approver string identifies who resolved the record.
func (m *PolicyApprovalManager) ResolveRecord(recordID string, approved bool, approver string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[recordID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRecordNotFound, recordID)
	}
	if record.Status != DecisionPending {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyResolved, record.Status)
	}
	if approved {
		record.Status = DecisionApproved
	} else {
		record.Status = DecisionDenied
	}
	record.Approver = approver
	record.UpdatedAt = time.Now()
	return record, nil
}

// GetRecord returns a stored approval record by ID.
func (m *PolicyApprovalManager) GetRecord(recordID string) (*Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.records[recordID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRecordNotFound, recordID)
	}
	return record, nil
}

// ListRecords returns all stored approval records.
func (m *PolicyApprovalManager) ListRecords() []*Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Record, 0, len(m.records))
	for _, r := range m.records {
		result = append(result, r)
	}
	return result
}


