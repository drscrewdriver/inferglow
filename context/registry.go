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

package contextmgr

import (
	"fmt"
	"sync"
)

// Factory creates a ContextManager for a given mode.
type Factory func(cfg Config, store StepStoreLike) (ContextManager, error)

// StepStoreLike is the subset of store.StepStore needed by the manager.
// This avoids an import cycle; the real store.StepStore satisfies this.
type StepStoreLike interface {
	AppendStep(step StepRecord) error
	GetStep(stepID int) (*StepRecord, error)
	RangeSteps(from, to int) ([]StepRecord, error)
	UpsertRef(ref RefRecord) error
	GetRef(stepID int) (*RefRecord, error)
	AllActiveStepIDs() ([]int, error)
	RemoveRef(stepID int) error
	AppendL1(rec L1Record) error
	GetL1(stepID int) (*L1Record, error)
	AppendL2(rec L2Record) error
	GetL2(stepID int) (*L2Record, error)
	HotFacts(minRefCount int, minStrength float64) ([]L2Record, error)
	AppendL3(rec L3Record) error
	GetL3(stepID int) (*L3Record, error)
	UpsertLongMem(mem LongMemRecord) error
	GetLongMem(memID string) (*LongMemRecord, error)
	SearchLongMem(query string, category string, limit int) ([]LongMemRecord, error)
	RemoveLongMem(memID string) error
	// AppendAudit appends an append-only audit log entry.
	AppendAudit(rec AuditRecord) error
	Close() error
}

// Registry manages mode factories and supports hot-switching.
type Registry struct {
	mu        sync.RWMutex
	factories map[Mode]Factory
	current   ContextManager
}

// NewRegistry creates an empty mode registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[Mode]Factory),
	}
}

// Register adds a factory for a mode.
func (r *Registry) Register(mode Mode, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[mode] = f
}

// SwitchMode creates or switches to a new ContextManager for the target mode.
// The StepStore is shared across switches — only the strategy engine changes.
func (r *Registry) SwitchMode(from, to Mode, cfg Config, store StepStoreLike) (ContextManager, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factories[to]
	if !ok {
		return nil, fmt.Errorf("contextmgr: no factory registered for mode %q", to)
	}

	mgr, err := f(cfg, store)
	if err != nil {
		return nil, fmt.Errorf("contextmgr: create %s manager: %w", to, err)
	}

	if r.current != nil && r.current.Mode() == from {
		// Best-effort close old manager (ignore error)
		_ = r.current.Close()
	}

	r.current = mgr
	return mgr, nil
}

// Current returns the active ContextManager, or nil if none.
func (r *Registry) Current() ContextManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}
