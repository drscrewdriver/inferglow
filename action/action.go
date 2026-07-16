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

// Package action defines the Action Runtime MVP for Inferglow.
//
// It exposes four core abstractions:
//
//   - Action            — a named, schema-described unit of work
//   - ActionExecutor    — the runtime contract for executing an Action
//   - ActionResult      — the structured outcome of an execution
//   - ActionRegistry    — a concurrency-safe catalog of Actions
//
// The package is intentionally independent of the rest of Inferglow so it
// can be embedded by any module that needs to expose callable Go functions
// as discoverable, schema-validated Actions.
package action

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/inferglow/model"
)

// Action is a named, schema-described unit of work bound to an Executor.
type Action struct {
	Name        string
	Description string
	Schema      map[string]any
	Executor    ActionExecutor
	Tags        []string
	// CacheTTL enables result caching for this action (OT-11).
	// Zero value means no caching. Only effective for non-write actions.
	CacheTTL time.Duration
}

// ActionExecutor is the runtime contract every Action must satisfy.
//
// Implementations are responsible for converting the loose map[string]any
// input into whatever strongly-typed form they require and for returning
// a structured ActionResult. Returning a non-nil error is reserved for
// infrastructure-level failures (e.g. registry lookup miss, panic
// recovery) and is wrapped into an error-shaped ActionResult by the
// registry's Execute helper.
type ActionExecutor interface { //nolint:revive
	Execute(ctx context.Context, input map[string]any) (*ActionResult, error)
}

// ActionResult is the structured outcome of an Action execution.
//
// Status is one of "success", "error", or "blocked". When OK is false,
// Error carries a human-readable message. Metadata is an optional
// side-channel for executor-specific extras that don't fit into the
// primary Result (e.g. MCP resource links collected during a
// tools/call response). Executors MAY leave Metadata nil.
type ActionResult struct { //nolint:revive
	OK       bool
	Status   string // "success" | "error" | "blocked"
	Result   any
	Error    string
	Metadata map[string]any
	// ContentBlocks carries multimodal output from the action
	// (e.g. generated images, audio, files). When non-empty,
	// callers can render these blocks in CLI/GUI.
	ContentBlocks []model.ContentBlock
}

// ActionRegistry is a concurrency-safe catalog of Actions keyed by Name.
type ActionRegistry struct { //nolint:revive
	mu      sync.RWMutex
	actions map[string]*Action
}

// Errors surfaced by the Action Runtime.
var (
	ErrActionAlreadyRegistered      = errors.New("action already registered")
	ErrActionNotFound               = errors.New("action not found")
	ErrUnsupportedFunctionSignature = errors.New("unsupported function signature")
)

// NewRegistry returns an empty ActionRegistry ready to accept registrations.
func NewRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]*Action),
	}
}

// Register validates and stores an Action.
//
// It rejects actions with an empty Name or nil Executor, and refuses
// duplicate registrations with ErrActionAlreadyRegistered.
func (r *ActionRegistry) Register(a *Action) error {
	if a == nil {
		return fmt.Errorf("%w: action is nil", ErrActionAlreadyRegistered)
	}
	if a.Name == "" {
		return fmt.Errorf("%w: action name cannot be empty", ErrActionAlreadyRegistered)
	}
	if a.Executor == nil {
		return fmt.Errorf("%w: action executor cannot be nil", ErrActionAlreadyRegistered)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.actions[a.Name]; exists {
		return fmt.Errorf("%w: %q", ErrActionAlreadyRegistered, a.Name)
	}
	r.actions[a.Name] = a
	return nil
}

// Get retrieves a registered Action by name.
//
// Missing names return an error wrapping ErrActionNotFound.
func (r *ActionRegistry) Get(name string) (*Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrActionNotFound, name)
	}
	return a, nil
}

// List returns the sorted names of every registered Action.
func (r *ActionRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.actions))
	for name := range r.actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Execute looks up an Action by name and dispatches the input to its Executor.
//
// A lookup miss returns ErrActionNotFound (wrapped). If the Executor
// returns a non-nil error, it is converted into an error-shaped
// ActionResult so callers always receive a structured result.
func (r *ActionRegistry) Execute(ctx context.Context, name string, input map[string]any) (*ActionResult, error) {
	a, err := r.Get(name)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	result, err := a.Executor.Execute(ctx, input)
	if err != nil {
		return &ActionResult{
			OK:     false,
			Status: "error",
			Error:  err.Error(),
		}, nil
	}
	return result, nil
}
