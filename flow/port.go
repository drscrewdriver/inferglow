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

package flow

// PortType enumerates the accepted data kinds for a named input/output port.
//
// This is the flow-graph-level port model (spec B-1). It is intentionally
// independent from the stage package's port model: stage imports flow (not the
// other way round), so a flow-level declaration cannot reference stage types
// without a cycle. The two models are kept semantically identical (same type
// names, same PortAny compatibility rule) so a port declared on a StageMeta can
// be mirrored onto a flow.Step without loss.
type PortType string

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

// CompatibleWith implements schema-level compatibility: a port whose type is
// PortAny is compatible with every other type; otherwise the types must match
// exactly. This mirrors the stage.PortType rule and the flow port resolver
// rule (PortAny degrades gracefully).
func (t PortType) CompatibleWith(o PortType) bool {
	return t == PortAny || o == PortAny || t == o
}

// PortDef declares the schema of a single named input/output port. It is the
// flow-graph-level counterpart of the stage package's PortDef (spec B-1/B-3).
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

// EdgePort declares a single port-level connection between two steps (spec
// B-1/B-4). An empty PortMappings on an Edge degrades to the legacy any→any
// pass-through, so existing flows are unaffected.
type EdgePort struct {
	FromStep string
	FromPort string
	ToStep   string
	ToPort   string
}

// FindPort returns the port definition with the given name from defs, or
// ok=false when absent. It mirrors the stage package's FindPort helper.
func FindPort(defs []PortDef, name string) (PortDef, bool) {
	for _, d := range defs {
		if d.Name == name {
			return d, true
		}
	}
	return PortDef{}, false
}