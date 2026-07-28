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

package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/inferglow/action"
)

// SkillRecord is the JSON-safe projection of a skill exposed by the Skill Hub
// (spec C-10). The underlying action.Action carries a Go Executor, which
// cannot be serialized, so the HTTP layer only ever sees this metadata view.
type SkillRecord struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Executable  bool     `json:"executable"`
}

// SkillStore is the C-10 Skill Hub store, wrapping action.ActionRegistry.
//
// The action package is owned by another lane (G1) and is treated as read-only
// here: the registry exposes Register/Get/List/Execute but no unregister, so
// removal is implemented as a soft delete tracked in a shadow set. The backing
// registry is never mutated for deletions, keeping the dependency clean.
type SkillStore struct {
	reg     *action.ActionRegistry
	mu      sync.RWMutex
	removed map[string]bool
}

// NewSkillStore creates an empty SkillStore.
func NewSkillStore() *SkillStore {
	return &SkillStore{
		reg:     action.NewRegistry(),
		removed: make(map[string]bool),
	}
}

// Install registers a skill in the backing registry. Re-installing a name
// that was soft-deleted re-activates it (the shadow tombstone is cleared).
func (ss *SkillStore) Install(a *action.Action) error {
	if a == nil {
		return fmt.Errorf("skill is nil")
	}
	ss.mu.Lock()
	delete(ss.removed, a.Name)
	ss.mu.Unlock()
	return ss.reg.Register(a)
}

// List returns the names of installed (non-removed) skills, sorted.
func (ss *SkillStore) List() []string {
	ss.mu.RLock()
	removed := make(map[string]bool, len(ss.removed))
	for k := range ss.removed {
		removed[k] = true
	}
	ss.mu.RUnlock()

	names := ss.reg.List()
	out := names[:0]
	for _, n := range names {
		if !removed[n] {
			out = append(out, n)
		}
	}
	return out
}

// Get returns the underlying action for a name, or nil if not installed
// (including names that have been soft-deleted).
func (ss *SkillStore) Get(name string) (*action.Action, error) {
	ss.mu.RLock()
	removed := ss.removed[name]
	ss.mu.RUnlock()
	if removed {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return ss.reg.Get(name)
}

// Remove soft-deletes a skill by name. The backing registry is left intact.
func (ss *SkillStore) Remove(name string) error {
	if _, err := ss.reg.Get(name); err != nil {
		return err
	}
	ss.mu.Lock()
	ss.removed[name] = true
	ss.mu.Unlock()
	return nil
}

// Execute runs a skill by name with the given input.
func (ss *SkillStore) Execute(ctx context.Context, name string, input map[string]any) (*action.ActionResult, error) {
	ss.mu.RLock()
	removed := ss.removed[name]
	ss.mu.RUnlock()
	if removed {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return ss.reg.Execute(ctx, name, input)
}