// Package stage defines the registry for stage functions.
//
// This file adds the port/metadata model (Meta) that declares a stage's
// explicit input/output port schema and metadata. The model coexists with the
// existing Func registration: stages may carry a Meta alongside their
// runtime function without changing the semantics of Register/Get/List.
//
// The registration style below (name-keyed, replace-on-register, concurrency-safe)
// is intentionally mirrored after the ContextManager registration mechanism so the
// two remain semantically interoperable, but it does NOT import or modify the
// context package (kept decoupled to avoid conflict with parallel branch A).
package stage

// PortType enumerates the accepted data kinds for a named input/output port.
type PortType string

// Port types.
const (
	PortString PortType = "string"
	PortInt    PortType = "int"
	PortFloat  PortType = "float"
	PortBool   PortType = "bool"
	PortJSON   PortType = "json"
	PortFile   PortType = "file"
	PortCode   PortType = "code"
	PortModel  PortType = "model"
	PortAny    PortType = "any"
)

// PortDef declares the schema of a single named input/output port.
type PortDef struct {
	Name        string
	Type        PortType
	Required    bool
	Default     any
	Description string
	Enum        []string
	Min, Max    *float64
	Children    map[string]*PortDef // nested ports (PortJSON/PortFile/...)
}

// CompatibleWith implements schema-level compatibility: a port whose type is
// PortAny is compatible with every other type; otherwise the types must match
// exactly. This mirrors the flow port resolver rule (PortAny degrades gracefully).
func (t PortType) CompatibleWith(o PortType) bool {
	return t == PortAny || o == PortAny || t == o
}

// Meta declares a stage's explicit input/output port schema and metadata.
//
// Name is auto-filled by RegisterMeta/RegisterWithMeta when left empty.
// Meta holds arbitrary key/value metadata (e.g. Kind, Version, Owner) that
// downstream tooling (ContextManager-style registries) can consume by name.
type Meta struct {
	Name        string
	Description string
	InputPorts  []PortDef
	OutputPorts []PortDef
	Meta        map[string]string // arbitrary key/value metadata
}

// StageMeta is kept for backward compatibility.
//
//nolint:revive
type StageMeta = Meta

// InputPort returns the input port definition with the given name. ok is false
// when no such input port is declared. Lookup is linear over this stage's own
// ports only (k is typically small); it never scans the global registry.
func (m Meta) InputPort(name string) (PortDef, bool) {
	return FindPort(m.InputPorts, name)
}

// OutputPort returns the output port definition with the given name. ok is false
// when no such output port is declared.
func (m Meta) OutputPort(name string) (PortDef, bool) {
	return FindPort(m.OutputPorts, name)
}

// FindPort returns the port definition with the given name from defs, or ok=false
// when absent. It is a reusable helper for wp-b2 (FlowDef portization) and keeps
// field-level lookups scoped to a single stage's ports instead of a global scan.
func FindPort(defs []PortDef, name string) (PortDef, bool) {
	for _, d := range defs {
		if d.Name == name {
			return d, true
		}
	}
	return PortDef{}, false
}
