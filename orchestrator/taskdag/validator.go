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

package taskdag

import "fmt"

// TopoSort returns a topological ordering of the DAG's nodes.
// Returns ErrCycleDetected if the graph contains a cycle.
func TopoSort(dag *TaskDAG) ([]TaskNode, error) {
	nodeMap := make(map[string]*TaskNode, len(dag.Tasks))
	for i := range dag.Tasks {
		nodeMap[dag.Tasks[i].ID] = &dag.Tasks[i]
	}

	// Kahn's algorithm.
	inDegree := make(map[string]int, len(dag.Tasks))
	for _, t := range dag.Tasks {
		if _, ok := inDegree[t.ID]; !ok {
			inDegree[t.ID] = 0
		}
		for _, dep := range t.DependsOn {
			_ = dep
			inDegree[t.ID]++
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var sorted []TaskNode
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, *nodeMap[id])

		// Find nodes that depend on this one.
		for _, t := range dag.Tasks {
			for _, dep := range t.DependsOn {
				if dep == id {
					inDegree[t.ID]--
					if inDegree[t.ID] == 0 {
						queue = append(queue, t.ID)
					}
				}
			}
		}
	}

	if len(sorted) != len(dag.Tasks) {
		return nil, ErrCycleDetected
	}
	return sorted, nil
}

// Validate checks the DAG for cycles and missing dependencies.
func Validate(dag *TaskDAG) error {
	nodeIDs := make(map[string]bool, len(dag.Tasks))
	for _, t := range dag.Tasks {
		if nodeIDs[t.ID] {
			return fmt.Errorf("%w: %s", ErrDuplicateNode, t.ID)
		}
		nodeIDs[t.ID] = true
	}
	for _, t := range dag.Tasks {
		for _, dep := range t.DependsOn {
			if !nodeIDs[dep] {
				return fmt.Errorf("%w: %s depends on %s", ErrNodeNotFound, t.ID, dep)
			}
		}
	}
	_, err := TopoSort(dag)
	return err
}
