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

// Package skill provides skill library management for the inferglow
// orchestrator. It is the Go equivalent of Agently's
// core/application/SkillLibrary: install, version, and bind reusable
// skill packages to agents.
package skill

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Sentinel errors.
var (
	ErrSkillNotFound    = errors.New("skill package not found")
	ErrSkillExists      = errors.New("skill package already installed")
	ErrRevisionNotFound = errors.New("skill revision not found")
	ErrInvalidSource    = errors.New("invalid skill source")
)

// SkillMode controls how skills are selected for an agent.
type SkillMode string

const (
	// ModelDecision lets the model decide which skills to invoke.
	ModelDecision SkillMode = "model_decision"
	// Required forces all bound skills to be invoked.
	Required SkillMode = "required"
)

// SkillRef references a specific skill package and optional revision.
type SkillRef struct {
	// Source is the skill package identifier (e.g. path, git URL).
	Source string `json:"source"`
	// Revision is the specific version; empty means latest.
	Revision string `json:"revision,omitempty"`
}

// SkillRevision is a specific installed version of a skill package.
type SkillRevision struct {
	// Source is the package identifier.
	Source string `json:"source"`
	// Revision is the version string.
	Revision string `json:"revision"`
	// InstalledAt is when this revision was installed.
	InstalledAt time.Time `json:"installed_at"`
	// Scope is the installation scope (e.g. "global", "agent", "session").
	Scope string `json:"scope"`
	// Metadata carries optional key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}


// SkillLibrary manages installed skill packages.
type SkillLibrary struct {
	mu        sync.RWMutex
	root      string
	packages  map[string][]*SkillRevision // source → revisions
}

// NewSkillLibrary creates a library rooted at the given path.
func NewSkillLibrary(root string) *SkillLibrary {
	return &SkillLibrary{
		root:     root,
		packages: make(map[string][]*SkillRevision),
	}
}

// Install registers a new skill revision. If the revision already
// exists for the source, it is appended as a new version.
func (lib *SkillLibrary) Install(source, scope string) (*SkillRevision, error) {
	if source == "" {
		return nil, ErrInvalidSource
	}
	lib.mu.Lock()
	defer lib.mu.Unlock()

	rev := &SkillRevision{
		Source:      source,
		Revision:    fmt.Sprintf("r%d", len(lib.packages[source])+1),
		InstalledAt: time.Now(),
		Scope:       scope,
	}
	lib.packages[source] = append(lib.packages[source], rev)
	return rev, nil
}

// GetRevision retrieves a specific revision. Empty revision means latest.
func (lib *SkillLibrary) GetRevision(source, revision string) (*SkillRevision, error) {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	revs, ok := lib.packages[source]
	if !ok || len(revs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, source)
	}
	if revision == "" {
		return revs[len(revs)-1], nil
	}
	for _, r := range revs {
		if r.Revision == revision {
			return r, nil
		}
	}
	return nil, fmt.Errorf("%w: %s@%s", ErrRevisionNotFound, source, revision)
}

// ListInstalled returns all installed skill revisions.
func (lib *SkillLibrary) ListInstalled() []*SkillRevision {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	var all []*SkillRevision
	for _, revs := range lib.packages {
		all = append(all, revs...)
	}
	return all
}

// Uninstall removes all revisions of a skill package.
func (lib *SkillLibrary) Uninstall(source string) bool {
	lib.mu.Lock()
	defer lib.mu.Unlock()
	_, ok := lib.packages[source]
	delete(lib.packages, source)
	return ok
}
