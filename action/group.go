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

package action

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// GroupRegistry is a derived, read-mostly view over an ActionRegistry that
// groups Actions by their Tags. It does NOT copy action data and does NOT
// change the semantics of ActionRegistry — it is purely a convenience layer
// for organizing tools into named groups (e.g. "readonly", "plan", "admin").
//
// Membership uses the reserved tag convention `group:<name>`: an Action whose
// Tags contain every tag of a ToolGroup's Tags belongs to that group. This
// reuses the existing tag metadata instead of duplicating it in a new field.
type GroupRegistry struct {
	mu       sync.RWMutex
	groups   map[string]*ToolGroup
	registry *ActionRegistry
}

// ToolGroup declares a named group of Actions. Tags are the membership
// selector: an Action belongs to the group when its Tags contain all of
// the group's Tags. Policy optionally carries group-level permission
// semantics for future extension.
type ToolGroup struct {
	Name        string
	Description string
	Tags        []string
	Policy      *GroupPolicy
}

// GroupPolicy declares group-level permission semantics. It is reserved for
// future extension (e.g. wiring group-level read-only constraints into a
// request-time ToolFilter). It is optional and may be left nil.
type GroupPolicy struct {
	// ReadOnly indicates the group only exposes read-only tools.
	ReadOnly bool
	// MaxLevel caps the maximum allowed side effect level for the group.
	// Zero value means "no limit".
	MaxLevel SideEffectLevel
}

// Errors surfaced by the GroupRegistry.
var (
	ErrGroupAlreadyRegistered = errors.New("group already registered")
	ErrGroupNotFound          = errors.New("group not found")
)

// NewGroupRegistry returns a GroupRegistry derived from the given
// ActionRegistry. A nil registry yields an empty group view.
func NewGroupRegistry(r *ActionRegistry) *GroupRegistry {
	return &GroupRegistry{
		groups:   make(map[string]*ToolGroup),
		registry: r,
	}
}

// Register adds a ToolGroup. It rejects nil groups, empty names, and
// duplicate names with ErrGroupAlreadyRegistered.
func (g *GroupRegistry) Register(grp *ToolGroup) error {
	if grp == nil {
		return fmt.Errorf("%w: group is nil", ErrGroupAlreadyRegistered)
	}
	if grp.Name == "" {
		return fmt.Errorf("%w: group name cannot be empty", ErrGroupAlreadyRegistered)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.groups[grp.Name]; exists {
		return fmt.Errorf("%w: %q", ErrGroupAlreadyRegistered, grp.Name)
	}
	g.groups[grp.Name] = grp
	return nil
}

// Get retrieves a registered ToolGroup by name.
func (g *GroupRegistry) Get(name string) (*ToolGroup, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	grp, ok := g.groups[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, name)
	}
	return grp, nil
}

// List returns the sorted names of every registered ToolGroup.
func (g *GroupRegistry) List() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	names := make([]string, 0, len(g.groups))
	for name := range g.groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Unregister removes a ToolGroup by name. Returns true if found and removed.
func (g *GroupRegistry) Unregister(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.groups[name]; !ok {
		return false
	}
	delete(g.groups, name)
	return true
}

// ListActionNames returns the sorted names of Actions that belong to the
// named group, derived from the underlying ActionRegistry via the group's
// Tags. A missing group returns an error wrapping ErrGroupNotFound.
func (g *GroupRegistry) ListActionNames(group string) ([]string, error) {
	grp, err := g.Get(group)
	if err != nil {
		return nil, err
	}
	if g.registry == nil {
		return nil, nil
	}
	return g.registry.ListActionNames(grp.Tags), nil
}

// HasAction reports whether the named Action belongs to the named group.
// A missing group returns false.
func (g *GroupRegistry) HasAction(group, name string) bool {
	names, err := g.ListActionNames(group)
	if err != nil {
		return false
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}