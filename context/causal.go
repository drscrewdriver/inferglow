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

package contextmgr

import (
	"fmt"
	"sort"
)

// CausalChain represents a chain of causally related steps.
type CausalChain struct {
	Steps     []StepRecord // steps in causal order (chronological)
	RootStep  int          // chain starting step
	FocusStep int          // chain focus step
	Files     []string     // all files involved in the chain
}

// TraceChain traces the causal chain starting from a given step.
// It follows file dependencies: if step A modified file X and step B read file X,
// then B depends on A.
func TraceChain(store StepStoreLike, stepID int) (*CausalChain, error) {
	// Get the target step
	step, err := store.GetStep(stepID)
	if err != nil {
		return nil, fmt.Errorf("trace chain: get step %d: %w", stepID, err)
	}

	chain := &CausalChain{
		RootStep:  stepID,
		FocusStep: stepID,
	}

	// Collect all files involved in this step
	fileSet := make(map[string]bool)
	for _, f := range step.FilesRead {
		fileSet[f] = true
	}
	for _, f := range step.FilesModified {
		fileSet[f] = true
	}

	// If no files, return just this step
	if len(fileSet) == 0 {
		chain.Steps = []StepRecord{*step}
		return chain, nil
	}

	// Scan all steps to find related ones
	allSteps, err := scanAllSteps(store)
	if err != nil {
		return nil, fmt.Errorf("trace chain: scan steps: %w", err)
	}

	// Find all steps that touch the same files
	for _, s := range allSteps {
		touches := false
		for _, f := range s.FilesRead {
			if fileSet[f] {
				touches = true
				break
			}
		}
		if !touches {
			for _, f := range s.FilesModified {
				if fileSet[f] {
					touches = true
					break
				}
			}
		}
		if touches {
			chain.Steps = append(chain.Steps, s)
		}
	}

	// Sort by step ID (chronological order)
	sort.Slice(chain.Steps, func(i, j int) bool {
		return chain.Steps[i].StepID < chain.Steps[j].StepID
	})

	// Collect all files
	for f := range fileSet {
		chain.Files = append(chain.Files, f)
	}
	sort.Strings(chain.Files)

	return chain, nil
}

// TraceFiles returns all steps that involve the specified files.
func TraceFiles(store StepStoreLike, files []string, limit int) ([]StepRecord, error) {
	if len(files) == 0 {
		return nil, nil
	}

	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	allSteps, err := scanAllSteps(store)
	if err != nil {
		return nil, fmt.Errorf("trace files: scan steps: %w", err)
	}

	var result []StepRecord
	for _, s := range allSteps {
		touches := false
		for _, f := range s.FilesRead {
			if fileSet[f] {
				touches = true
				break
			}
		}
		if !touches {
			for _, f := range s.FilesModified {
				if fileSet[f] {
					touches = true
					break
				}
			}
		}
		if touches {
			result = append(result, s)
		}
	}

	// Sort by step ID
	sort.Slice(result, func(i, j int) bool {
		return result[i].StepID < result[j].StepID
	})

	// Apply limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// TraceTaskGroup returns all steps in the same task group.
func TraceTaskGroup(store StepStoreLike, group string) ([]StepRecord, error) {
	if group == "" {
		return nil, nil
	}

	allSteps, err := scanAllSteps(store)
	if err != nil {
		return nil, fmt.Errorf("trace task group: scan steps: %w", err)
	}

	var result []StepRecord
	for _, s := range allSteps {
		if s.TaskGroup == group {
			result = append(result, s)
		}
	}

	// Sort by step ID
	sort.Slice(result, func(i, j int) bool {
		return result[i].StepID < result[j].StepID
	})

	return result, nil
}

// scanAllSteps retrieves all steps from the store.
func scanAllSteps(store StepStoreLike) ([]StepRecord, error) {
	// Get all active step IDs
	ids, err := store.AllActiveStepIDs()
	if err != nil {
		return nil, err
	}

	var steps []StepRecord
	for _, id := range ids {
		step, err := store.GetStep(id)
		if err != nil {
			continue // skip missing steps
		}
		steps = append(steps, *step)
	}

	return steps, nil
}
