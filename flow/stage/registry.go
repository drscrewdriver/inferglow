// Package stage defines the registry for stage functions.
//
// A Stage is the unit of execution in a flow. Stages are registered by name
// and looked up at flow-build time. Each StageFunc receives the step inputs
// and the flow.FlowContext (for LLM/Action/Session access).
//
// This package was recycled from the inferflow project and adapted to live
// under the inferglow flow module.
package stage

import (
	"context"
	"sync"

	"github.com/inferglow/flow"
)

// Inputs is the set of named inputs passed into a stage.
type Inputs map[string]any

// Outputs is the set of named outputs produced by a stage.
type Outputs map[string]any

// StageFunc is the signature for a registered stage. The flow.FlowContext
// may be nil when the stage is invoked outside a flow (e.g. in unit tests).
//
// StageFunc is a specialised form of flow.StepFunc that receives typed
// Inputs/Outputs maps and direct access to FlowContext. Use Adapt to
// convert a StageFunc into a flow.StepFunc for use in LCEL chains or
// other StepFunc-based APIs.
type StageFunc func(ctx context.Context, in Inputs, fctx flow.FlowContext) (Outputs, error)

// Adapt converts a StageFunc into a flow.StepFunc so it can be used in
// LCEL chains, flow.Step, or any other API that accepts flow.StepFunc.
//
// The returned StepFunc extracts the FlowContext from ctx (via
// flow.FlowContextFrom), type-asserts the input to map[string]any
// (falling back to wrapping under key "input"), and converts the
// stage.Outputs result to a plain map[string]any.
func Adapt(fn StageFunc) flow.StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		data := toMap(input)
		fctx, _ := flow.FlowContextFrom(ctx)
		outs, err := fn(ctx, data, fctx)
		if err != nil {
			return nil, err
		}
		// Convert Outputs to plain map for downstream compatibility.
		out := make(map[string]any, len(outs))
		for k, v := range outs {
			out[k] = v
		}
		return out, nil
	}
}

// toMap normalises input into a map[string]any. If the input is already
// such a map it is returned directly; otherwise the value is wrapped
// under the key "input".
func toMap(input any) map[string]any {
	if m, ok := input.(map[string]any); ok {
		return m
	}
	return map[string]any{"input": input}
}

// Registry holds named StageFuncs (and their optional StageMeta declarations)
// and is safe for concurrent use.
//
// The StageFunc map (m) and the metadata map (meta) are kept fully decoupled:
// RegisterMeta does not depend on whether a StageFunc was registered first, and
// Get/GetMeta each read only their own map. This preserves the existing
// Register/Get/List semantics unchanged while adding a parallel metadata channel.
type Registry struct {
	mu   sync.RWMutex
	m    map[string]StageFunc
	meta map[string]StageMeta // added: separate declarative metadata side-channel
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		m:    make(map[string]StageFunc),
		meta: make(map[string]StageMeta),
	}
}

// Register adds or replaces the stage registered under name.
func (r *Registry) Register(name string, f StageFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = f
}

// Get returns the StageFunc registered under name. ok is false when no
// stage is registered with that name.
func (r *Registry) Get(name string) (StageFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.m[name]
	return f, ok
}

// List returns the names of all registered stages. The order is unspecified.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.m))
	for name := range r.m {
		names = append(names, name)
	}
	return names
}

// RegisterMeta attaches declarative metadata to a stage name. It is independent of
// StageFunc registration: it is harmless to register metadata before or without a
// StageFunc. A non-empty meta.Name is preserved; an empty Name is back-filled to
// the registration key, mirroring the name-keyed, replace-on-register semantics of
// the ContextManager registry (without importing or modifying context).
func (r *Registry) RegisterMeta(name string, meta StageMeta) {
	if meta.Name == "" {
		meta.Name = name
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta[name] = meta
}

// RegisterWithMeta atomically registers a StageFunc together with its metadata,
// writing both sides under a single lock. It is the recommended entry point for
// stages that want to declare their ports alongside their runtime function.
func (r *Registry) RegisterWithMeta(name string, f StageFunc, meta StageMeta) {
	if meta.Name == "" {
		meta.Name = name
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = f
	r.meta[name] = meta
}

// GetMeta returns the StageMeta registered under name. ok is false when no
// metadata is registered with that name. Lookup is O(1) on a map.
func (r *Registry) GetMeta(name string) (StageMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.meta[name]
	return meta, ok
}

// MetaNames returns the names that carry a StageMeta. The order is unspecified.
func (r *Registry) MetaNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.meta))
	for name := range r.meta {
		names = append(names, name)
	}
	return names
}
