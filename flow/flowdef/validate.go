package flowdef

import (
	"fmt"
	"sort"
)

// knownOperators is the set of operator names recognised by the adapter in
// Phase 1. Stage is fully implemented; the others are accepted by Validate
// and get pass-through stubs in the Adapter.
var knownOperators = map[string]bool{
	"stage":           true,
	"parallel_fanout": true,
	"match_case":      true,
	"signal_gate":     true,
	"chunk":           true,
	"for_each":        true,
}

// Validate checks a FlowDef for structural correctness:
//   - metadata.name is non-empty
//   - at least one step exists
//   - each step has a name and a known operator
//   - step names are unique
//   - every depends_on entry references an existing step
//   - the dependency graph has no cycles
func Validate(def *FlowDef) error {
	if def == nil {
		return fmt.Errorf("flowdef: nil definition")
	}
	if def.Metadata.Name == "" {
		return fmt.Errorf("flowdef: metadata.name is empty")
	}
	if len(def.Spec.Steps) == 0 {
		return fmt.Errorf("flowdef: flow %q has no steps", def.Metadata.Name)
	}

	// Collect step names; check uniqueness, name presence, operator presence.
	names := make(map[string]bool, len(def.Spec.Steps))
	for i, s := range def.Spec.Steps {
		if s.Name == "" {
			return fmt.Errorf("flowdef: step at index %d has empty name", i)
		}
		if names[s.Name] {
			return fmt.Errorf("flowdef: duplicate step name %q", s.Name)
		}
		names[s.Name] = true
		if s.Operator == "" {
			return fmt.Errorf("flowdef: step %q has empty operator", s.Name)
		}
		if !knownOperators[s.Operator] {
			return fmt.Errorf("flowdef: step %q has unknown operator %q", s.Name, s.Operator)
		}
	}

	// Verify depends_on references and build adjacency: dep -> dependents.
	adj := make(map[string][]string)
	indeg := make(map[string]int)
	for _, name := range keysOf(names) {
		indeg[name] = 0
	}
	for _, s := range def.Spec.Steps {
		for _, dep := range s.DependsOn {
			if !names[dep] {
				return fmt.Errorf("flowdef: step %q depends_on unknown step %q", s.Name, dep)
			}
			adj[dep] = append(adj[dep], s.Name)
			indeg[s.Name]++
		}
	}

	// Cycle detection via DFS three-colour. White=unvisited, Gray=on stack,
	// Black=done.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(names))
	for name := range names {
		color[name] = white
	}
	// Visit in sorted order for deterministic error messages.
	roots := keysOf(names)
	sort.Strings(roots)
	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for _, next := range adj[node] {
			if color[next] == gray {
				return true // back edge -> cycle
			}
			if color[next] == white && dfs(next) {
				return true
			}
		}
		color[node] = black
		return false
	}
	for _, root := range roots {
		if color[root] == white && dfs(root) {
			return fmt.Errorf("flowdef: cycle detected in depends_on graph")
		}
	}
	return nil
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
