package flowdef

import (
	"errors"
	"sync"

	"github.com/inferglow/flow"
)

// errNilFlowDef is returned when a nil definition is passed to Register.
var errNilFlowDef = errors.New("flowdef: nil definition")

// FlowRegistry holds named FlowDefs and is safe for concurrent use. It is
// keyed by Metadata.Name (the flow's canonical name), mirroring the
// name-keyed, replace-on-register semantics of the stage.Registry.
//
// A FlowRegistry is a build-time/authoring-time index: it stores the
// declarative FlowDef plus its already-compiled *flow.Flow. Compilation is
// performed lazily on first Register and cached, so a definition can be
// registered before its stages are present, and the compiled flow is
// recomputed only when the definition is replaced.
type FlowRegistry struct {
	mu    sync.RWMutex
	defs  map[string]*FlowDef
	flows map[string]*flow.Flow
}

// NewFlowRegistry returns an empty FlowRegistry.
func NewFlowRegistry() *FlowRegistry {
	return &FlowRegistry{
		defs:  make(map[string]*FlowDef),
		flows: make(map[string]*flow.Flow),
	}
}

// Register adds or replaces the flow registered under name. The name used is
// the argument passed in; if def.Metadata.Name is empty it is back-filled to
// name so the registry and the definition stay consistent. The flow is
// compiled immediately; a compile error is returned and the registry is left
// unchanged (so a bad definition never overwrites a good one).
func (r *FlowRegistry) Register(name string, def *FlowDef, adapter *Adapter) error {
	if def == nil {
		return errNilFlowDef
	}
	if def.Metadata.Name == "" {
		def.Metadata.Name = name
	}
	// Compile outside the lock to avoid holding it during template parsing.
	var compiled *flow.Flow
	if adapter != nil {
		f, err := adapter.ToFlow(def)
		if err != nil {
			return err
		}
		compiled = f
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs[name] = def
	if compiled != nil {
		r.flows[name] = compiled
	}
	return nil
}

// Get returns the FlowDef registered under name. ok is false when no flow is
// registered with that name.
func (r *FlowRegistry) Get(name string) (*FlowDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.defs[name]
	return def, ok
}

// GetCompiled returns the compiled *flow.Flow registered under name. ok is
// false when no flow is registered with that name, or when it was registered
// without an adapter (definitions-only registration).
func (r *FlowRegistry) GetCompiled(name string) (*flow.Flow, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.flows[name]
	return f, ok
}

// List returns the names of all registered flows. The order is unspecified.
func (r *FlowRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	return names
}

// Delete removes the flow registered under name. It is a no-op when the name
// is not present.
func (r *FlowRegistry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.defs, name)
	delete(r.flows, name)
}