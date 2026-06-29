package flowdef

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/stage"
)

// Adapter compiles a declarative FlowDef into an executable inferglow
// *flow.Flow. It looks up stage functions from the stage.Registry and wires
// step edges based on depends_on.
//
// Phase 1 supports linear flows (each step depends on at most one previous
// step) and the `stage` operator. Other operators are accepted as
// pass-through stubs (TODO: full operator implementation in Phase 1.5).
type Adapter struct {
	stages *stage.Registry
}

// NewAdapter returns an Adapter backed by the given stage.Registry.
func NewAdapter(stages *stage.Registry) *Adapter {
	return &Adapter{stages: stages}
}

// ToFlow converts a FlowDef into a *flow.Flow. The resulting flow, when
// executed, threads an accumulated data map (flow inputs + prior step
// outputs) through each step. Each step stores its output under its name so
// later steps can reference it via templates like {{.stepname.field}}.
func (a *Adapter) ToFlow(def *FlowDef) (*flow.Flow, error) {
	if err := Validate(def); err != nil {
		return nil, err
	}

	// Build step functions and Step objects, keyed by name.
	stepObjs := make(map[string]*flow.Step, len(def.Spec.Steps))
	for _, sd := range def.Spec.Steps {
		fn, err := a.toStepFunc(sd)
		if err != nil {
			return nil, fmt.Errorf("flowdef: step %q: %w", sd.Name, err)
		}
		stepObjs[sd.Name] = flow.NewStep(sd.Name, fn).Build()
	}

	// Wire edges in topological (linear) order.
	order, err := linearOrder(def.Spec.Steps)
	if err != nil {
		return nil, err
	}

	builder := flow.NewFlow()
	for i, name := range order {
		if i == 0 {
			builder.AddStep(stepObjs[name])
		} else {
			builder.To(stepObjs[name])
		}
	}
	return builder.Build(), nil
}

// toStepFunc builds a flow.StepFunc for the given StepDef. The function:
//   - Receives the accumulated data map (flow inputs + prior outputs) as input
//   - Evaluates the `when` expression; if false, skips the stage and returns
//     the data map unchanged
//   - For the `stage` operator: renders step.Inputs templates, calls the
//     registered StageFunc, and stores the output under step.Name
//   - For other operators: pass-through stub (TODO: Phase 1.5)
func (a *Adapter) toStepFunc(sd StepDef) (flow.StepFunc, error) {
	// Pre-resolve the stage function for stage operators so that missing
	// stages are reported at build time (ToFlow), not at execution time.
	var stageFn stage.StageFunc
	if sd.Operator == "stage" {
		if a.stages == nil {
			return nil, fmt.Errorf("stage registry is nil")
		}
		fn, ok := a.stages.Get(sd.Stage)
		if !ok {
			return nil, fmt.Errorf("stage %q not found in registry", sd.Stage)
		}
		stageFn = fn
	}

	return func(ctx context.Context, input any) (any, error) {
		data := toDataMap(input)

		// Evaluate `when` guard. A false result skips the step body and
		// passes the accumulated data through unchanged.
		if sd.When != "" {
			shouldRun, err := evalWhen(sd.When, data)
			if err != nil {
				return nil, fmt.Errorf("step %q when %q: %w", sd.Name, sd.When, err)
			}
			if !shouldRun {
				return data, nil
			}
		}

		// Non-stage operators: pass-through stub.
		// TODO: full operator implementation in Phase 1.5.
		if sd.Operator != "stage" {
			return data, nil
		}

		// Render step.Inputs templates against the accumulated data.
		stageInputs := renderInputs(sd.Inputs, data)

		// Inject system_prompt override if specified in the YAML.
		// The convention is that _system_prompt is a reserved key that
		// LLM-based stages can check to override their default prompt.
		if sd.SystemPrompt != "" {
			rendered := renderValue(sd.SystemPrompt, data)
			if s, ok := rendered.(string); ok {
				stageInputs["_system_prompt"] = s
			}
		}

		// Extract FlowContext (may be absent in unit tests).
		fctx, _ := flow.FlowContextFrom(ctx)

		outputs, err := stageFn(ctx, stageInputs, fctx)
		if err != nil {
			return nil, fmt.Errorf("step %q stage %q: %w", sd.Name, sd.Stage, err)
		}

		// Copy outputs into a plain map[string]any so that downstream
		// templates and type assertions work uniformly.
		outMap := make(map[string]any, len(outputs))
		for k, v := range outputs {
			outMap[k] = v
		}

		// Return a new accumulated map with this step's output merged in.
		// Copying avoids mutating the predecessor's data in place.
		// Step outputs are merged both under the step name (for namespaced
		// access) and flattened into the top level (so downstream stages
		// can access fields like "branch_name" directly).
		merged := make(map[string]any, len(data)+len(outMap)+1)
		for k, v := range data {
			merged[k] = v
		}
		merged[sd.Name] = outMap
		for k, v := range outMap {
			merged[k] = v
		}
		return merged, nil
	}, nil
}

// toDataMap normalises the step input into a map[string]any. If the input
// is already such a map it is returned directly; otherwise the value is
// wrapped under the key "input".
func toDataMap(input any) map[string]any {
	if m, ok := input.(map[string]any); ok {
		return m
	}
	return map[string]any{"input": input}
}

// renderInputs renders each value in the step's inputs map as a text/template
// against the data map. Non-string values are passed through unchanged.
// When the step has no explicit inputs, the full data map is returned so the
// stage can pick the fields it needs.
// When explicit inputs are provided, they are merged ON TOP of the accumulated
// data map, so stages receive both the context from previous steps AND any
// step-specific overrides (like _agent: true).
func renderInputs(inputs map[string]any, data map[string]any) stage.Inputs {
	// Start with a copy of the accumulated data
	out := make(stage.Inputs, len(data)+len(inputs))
	for k, v := range data {
		out[k] = v
	}
	// Override/merge with explicit inputs
	for k, v := range inputs {
		out[k] = renderValue(v, data)
	}
	return out
}

// renderValue renders a single input value. String values containing "{{"
// are treated as templates; all other values pass through unchanged.
func renderValue(v any, data map[string]any) any {
	s, ok := v.(string)
	if !ok || !strings.Contains(s, "{{") {
		return v
	}
	tmpl, err := template.New("input").Parse(s)
	if err != nil {
		return s
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return s
	}
	return buf.String()
}

// linearOrder returns step names in execution order for a linear flow. It
// performs a topological sort (Kahn's algorithm) and expects each step to
// have at most one predecessor. Steps with no depends_on are roots; if
// multiple roots exist they are ordered alphabetically for determinism.
func linearOrder(steps []StepDef) ([]string, error) {
	byName := make(map[string]StepDef, len(steps))
	for _, s := range steps {
		byName[s.Name] = s
	}

	// in-degree and adjacency (dep -> dependents).
	indeg := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, s := range steps {
		indeg[s.Name] = len(s.DependsOn)
		for _, dep := range s.DependsOn {
			adj[dep] = append(adj[dep], s.Name)
		}
	}

	// Seed queue with zero-in-degree nodes, sorted for determinism.
	var queue []string
	for name, d := range indeg {
		if d == 0 {
			queue = append(queue, name)
		}
	}
	// Sort the seed queue.
	for i := 0; i < len(queue); i++ {
		for j := i + 1; j < len(queue); j++ {
			if queue[j] < queue[i] {
				queue[i], queue[j] = queue[j], queue[i]
			}
		}
	}

	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		// Sort dependents for deterministic ordering.
		deps := adj[cur]
		for i := 0; i < len(deps); i++ {
			for j := i + 1; j < len(deps); j++ {
				if deps[j] < deps[i] {
					deps[i], deps[j] = deps[j], deps[i]
				}
			}
		}
		for _, next := range deps {
			indeg[next]--
			if indeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(steps) {
		return nil, fmt.Errorf("flowdef: could not linearise steps (cycle or disconnected graph)")
	}
	return order, nil
}
