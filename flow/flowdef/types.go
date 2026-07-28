// Package flowdef converts YAML flow definitions into executable *flow.Flow
// instances. A FlowDef is the declarative description of a pipeline (inputs,
// steps, outputs); the Adapter compiles it into an executable Flow backed by
// the stage.Registry.
//
// This package was recycled from the inferflow project and adapted to live
// under the inferglow flow module.
package flowdef

import "github.com/inferglow/flow/stage"

// FlowDef is the top-level YAML structure of a flow definition file.
type FlowDef struct {
	APIVersion string   `yaml:"api_version"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata describes the flow's identity and ownership.
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Owner       string `yaml:"owner"`
}

// Spec contains the inputs, steps, and outputs of a flow.
//
// InputPorts/OutputPorts declare the portized interface of the flow as a
// whole (wp-b2). They are optional: when empty, the flow degrades to the
// legacy any→any behaviour and existing callers are unaffected.
type Spec struct {
	Inputs      []InputDef        `yaml:"inputs"`
	Steps       []StepDef         `yaml:"steps"`
	Outputs     map[string]string `yaml:"outputs"`
	InputPorts  []stage.PortDef   `yaml:"input_ports,omitempty"`  // optional: flow-level input ports
	OutputPorts []stage.PortDef   `yaml:"output_ports,omitempty"` // optional: flow-level output ports
}

// InputDef describes a single named flow input parameter.
type InputDef struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
	Default  string `yaml:"default"`
}

// StepDef describes a single step in the flow. Only the fields relevant to
// Phase 1 (stage operator, linear depends_on, when) are fully wired; the
// remaining fields are parsed and stored for later phases.
//
// InputPorts/OutputPorts declare the step's explicit port interface (wp-b2).
// They are optional and coexist with the legacy Inputs/Schema fields; the
// adapter treats empty ports as the legacy any→any path.
type StepDef struct {
	Name         string          `yaml:"name"`
	Operator     string          `yaml:"operator"`
	Stage        string          `yaml:"stage"`
	DependsOn    []string        `yaml:"depends_on"`
	When         string          `yaml:"when"`
	Inputs       map[string]any  `yaml:"inputs"`
	Schema       map[string]any  `yaml:"schema"`
	FanoutOver   string          `yaml:"fanout_over"`
	FanoutAs     string          `yaml:"fanout_as"`
	Cases        []CaseDef       `yaml:"cases"`
	OutputsTo    string          `yaml:"outputs_to"`
	SystemPrompt string          `yaml:"system_prompt,omitempty"` // optional: overrides the stage's default system prompt
	InputPorts   []stage.PortDef `yaml:"input_ports,omitempty"`   // optional: named input ports (wp-b2)
	OutputPorts  []stage.PortDef `yaml:"output_ports,omitempty"`  // optional: named output ports (wp-b2)
}

// CaseDef describes one branch of a match_case operator.
type CaseDef struct {
	When    string `yaml:"when"`
	Next    string `yaml:"next"`
	Default bool   `yaml:"default"`
}
