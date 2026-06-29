package flowdef

import (
	"fmt"
	"log"
)

// ConvertSimpleToFlowDef transforms a SimpleWorkflow (stages/next linked-list
// format) into a FlowDef (steps/depends_on DAG format). The conversion is
// transparent to downstream consumers: Adapter.ToFlow and Validate work
// unchanged on the result.
//
// Conversion rules:
//   - name → metadata.name; description → metadata.description
//   - api_version = "flowdef/v1"; kind = "Flow"
//   - Each SimpleStage → StepDef{Operator: "stage", Stage: name}
//   - next pointers are inverted into depends_on arrays:
//     if A.Next == "B", then B.DependsOn = [..., "A"]
//   - rollback_to is stored in StepDef.Inputs["_rollback_to"] for future use
//   - label/description are stored in StepDef.Schema for metadata
func ConvertSimpleToFlowDef(sw *SimpleWorkflow) (*FlowDef, error) {
	if sw == nil {
		return nil, fmt.Errorf("flowdef: nil SimpleWorkflow")
	}
	if sw.Name == "" {
		return nil, fmt.Errorf("flowdef: SimpleWorkflow has empty name")
	}
	if len(sw.Stages) == 0 {
		return nil, fmt.Errorf("flowdef: SimpleWorkflow %q has no stages", sw.Name)
	}

	// Build name→index map and validate uniqueness.
	byName := make(map[string]int, len(sw.Stages))
	for i, s := range sw.Stages {
		if s.Name == "" {
			return nil, fmt.Errorf("flowdef: stage at index %d has empty name", i)
		}
		if _, dup := byName[s.Name]; dup {
			return nil, fmt.Errorf("flowdef: duplicate stage name %q", s.Name)
		}
		byName[s.Name] = i
	}

	// Validate start_stage exists (if specified).
	if sw.StartStage != "" {
		if _, ok := byName[sw.StartStage]; !ok {
			return nil, fmt.Errorf("flowdef: start_stage %q not found in stages", sw.StartStage)
		}
	}

	// Validate next pointers reference existing stages and build reverse
	// dependency map: for each stage, who points to it (i.e. depends_on).
	dependsOn := make(map[string][]string, len(sw.Stages))
	for _, s := range sw.Stages {
		if s.Next != nil && *s.Next != "" {
			target := *s.Next
			if _, ok := byName[target]; !ok {
				return nil, fmt.Errorf("flowdef: stage %q next %q not found", s.Name, target)
			}
			dependsOn[target] = append(dependsOn[target], s.Name)
		}
	}

	// Detect cycles via DFS three-colour.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(sw.Stages))
	for _, s := range sw.Stages {
		color[s.Name] = white
	}
	var dfs func(name string) bool
	dfs = func(name string) bool {
		color[name] = gray
		idx := byName[name]
		s := sw.Stages[idx]
		if s.Next != nil && *s.Next != "" {
			next := *s.Next
			if color[next] == gray {
				return true // back edge → cycle
			}
			if color[next] == white && dfs(next) {
				return true
			}
		}
		color[name] = black
		return false
	}
	for _, s := range sw.Stages {
		if color[s.Name] == white {
			if dfs(s.Name) {
				return nil, fmt.Errorf("flowdef: cycle detected in next pointers")
			}
		}
	}

	// Build StepDefs.
	steps := make([]StepDef, 0, len(sw.Stages))
	for _, s := range sw.Stages {
		sd := StepDef{
			Name:      s.Name,
			Operator:  "stage",
			Stage:     s.Name,
			DependsOn: dependsOn[s.Name], // may be nil/empty
		}

		// Store rollback_to as an input annotation for future runtime use.
		if s.RollbackTo != nil && *s.RollbackTo != "" {
			if _, ok := byName[*s.RollbackTo]; !ok {
				return nil, fmt.Errorf("flowdef: stage %q rollback_to %q not found", s.Name, *s.RollbackTo)
			}
			sd.Inputs = map[string]any{
				"_rollback_to": *s.RollbackTo,
			}
			log.Printf("flowdef: warning: rollback_to on stage %q is recorded but not enforced at runtime", s.Name)
		}

		// Store label/description as schema metadata.
		if s.Label != "" || s.Description != "" {
			if sd.Schema == nil {
				sd.Schema = make(map[string]any)
			}
			if s.Label != "" {
				sd.Schema["label"] = s.Label
			}
			if s.Description != "" {
				sd.Schema["description"] = s.Description
			}
		}

		steps = append(steps, sd)
	}

	def := &FlowDef{
		APIVersion: "flowdef/v1",
		Kind:       "Flow",
		Metadata: Metadata{
			Name:        sw.Name,
			Description: sw.Description,
		},
		Spec: Spec{
			Steps: steps,
		},
	}
	return def, nil
}
