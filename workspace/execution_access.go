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

package workspace

import (
	"errors"
	"sync"
	"time"
)

// Sentinel errors for execution access.
var (
	ErrAccessDenied  = errors.New("execution access denied")
	ErrAccessExpired = errors.New("execution access grant expired")
	ErrGrantNotFound = errors.New("access grant not found")
)

// ExecutionAccessGrant defines the access permissions for a specific
// execution within a workspace.
type ExecutionAccessGrant struct {
	// ExecutionID is the unique identifier of the execution.
	ExecutionID string `json:"execution_id"`
	// WorkspaceID identifies the workspace this grant applies to.
	WorkspaceID string `json:"workspace_id"`
	// ReadPaths lists paths the execution is allowed to read.
	ReadPaths []string `json:"read_paths,omitempty"`
	// WritePaths lists paths the execution is allowed to write.
	WritePaths []string `json:"write_paths,omitempty"`
	// ExpiresAt is the time when this grant becomes invalid.
	// Zero value means no expiration.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// IsExpired returns true if the grant has expired.
func (g *ExecutionAccessGrant) IsExpired() bool {
	if g.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(g.ExpiresAt)
}

// CanRead returns true if the grant allows reading the given path.
func (g *ExecutionAccessGrant) CanRead(path string) bool {
	if g.IsExpired() {
		return false
	}
	if len(g.ReadPaths) == 0 {
		return false
	}
	for _, p := range g.ReadPaths {
		if p == "*" || p == path {
			return true
		}
	}
	return false
}

// CanWrite returns true if the grant allows writing the given path.
func (g *ExecutionAccessGrant) CanWrite(path string) bool {
	if g.IsExpired() {
		return false
	}
	if len(g.WritePaths) == 0 {
		return false
	}
	for _, p := range g.WritePaths {
		if p == "*" || p == path {
			return true
		}
	}
	return false
}

// ExecutionAccessStore manages execution access grants.
type ExecutionAccessStore struct {
	mu     sync.RWMutex
	grants map[string]*ExecutionAccessGrant // keyed by ExecutionID
}

// NewExecutionAccessStore creates an empty access store.
func NewExecutionAccessStore() *ExecutionAccessStore {
	return &ExecutionAccessStore{
		grants: make(map[string]*ExecutionAccessGrant),
	}
}

// Grant adds or replaces an access grant.
func (s *ExecutionAccessStore) Grant(g *ExecutionAccessGrant) error {
	if g.ExecutionID == "" {
		return errors.New("execution_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[g.ExecutionID] = g
	return nil
}

// Revoke removes an access grant.
func (s *ExecutionAccessStore) Revoke(executionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.grants[executionID]
	if ok {
		delete(s.grants, executionID)
	}
	return ok
}

// Get returns the grant for the given execution, or ErrGrantNotFound.
func (s *ExecutionAccessStore) Get(executionID string) (*ExecutionAccessGrant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.grants[executionID]
	if !ok {
		return nil, ErrGrantNotFound
	}
	return g, nil
}

// CheckRead verifies that the execution can read the given path.
func (s *ExecutionAccessStore) CheckRead(executionID, path string) error {
	g, err := s.Get(executionID)
	if err != nil {
		return err
	}
	if g.IsExpired() {
		return ErrAccessExpired
	}
	if !g.CanRead(path) {
		return ErrAccessDenied
	}
	return nil
}

// CheckWrite verifies that the execution can write the given path.
func (s *ExecutionAccessStore) CheckWrite(executionID, path string) error {
	g, err := s.Get(executionID)
	if err != nil {
		return err
	}
	if g.IsExpired() {
		return ErrAccessExpired
	}
	if !g.CanWrite(path) {
		return ErrAccessDenied
	}
	return nil
}
