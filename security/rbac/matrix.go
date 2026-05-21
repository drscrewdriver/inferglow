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

package rbac

import (
	"sort"
	"sync"
)

// PermissionMatrix is the central registry that maps an Action name to
// the set of roles allowed to invoke it. The zero value is not usable;
// use NewPermissionMatrix. All methods are safe for concurrent use.
//
// The matrix implements a default-deny policy: an Action that has
// never been registered via Register returns false from Allow for
// every role, including RoleAdmin. Callers must explicitly register
// every Action they want to gate.
type PermissionMatrix struct {
	mu          sync.RWMutex
	permissions map[string]map[Role]bool
}

// NewPermissionMatrix returns an empty permission matrix with no
// Actions registered.
func NewPermissionMatrix() *PermissionMatrix {
	return &PermissionMatrix{
		permissions: make(map[string]map[Role]bool),
	}
}

// Register records that the given roles may invoke actionName. Calling
// Register twice for the same actionName replaces the previous entry.
// A nil or empty roles slice effectively denies the Action for every
// role (the action becomes registered-but-empty, which still returns
// false from Allow).
func (m *PermissionMatrix) Register(actionName string, roles []Role) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roleSet := make(map[Role]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}
	m.permissions[actionName] = roleSet
}

// RegisterPolicy is a convenience that registers actionName from an
// ActionPolicy's AllowedRoles.
func (m *PermissionMatrix) RegisterPolicy(actionName string, policy ActionPolicy) {
	m.Register(actionName, policy.AllowedRoles)
}

// Allow reports whether role may invoke actionName. It returns false
// for any actionName that has not been registered (default-deny) and
// false for any role not in the registered set.
func (m *PermissionMatrix) Allow(role Role, actionName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	roleSet, ok := m.permissions[actionName]
	if !ok {
		return false
	}
	return roleSet[role]
}

// ActionsForRole returns the names of every registered Action that role
// is allowed to invoke, sorted alphabetically for deterministic output.
func (m *PermissionMatrix) ActionsForRole(role Role) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for name, roleSet := range m.permissions {
		if roleSet[role] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// RegisteredActions returns the names of all Actions that have been
// registered with the matrix, sorted alphabetically.
func (m *PermissionMatrix) RegisteredActions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.permissions))
	for name := range m.permissions {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
