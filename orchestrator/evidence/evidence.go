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

// Package evidence implements a declarative "evidence gate" policy engine.
//
// A gate declares the minimum set of evidence that must be satisfied before a
// consequential step may proceed (test passed, coverage threshold, lint clean,
// a verification artifact present, ...). Evaluate is deny-by-default: when any
// required evidence is missing the gate blocks the step and reports the unmet
// items, so a workflow never proceeds on insufficient proof.
package evidence

import "fmt"

// Key is a canonical evidence identifier. Known keys are listed below, but a
// gate may reference any provider-specific key.
type Key string

const (
	// KeyTestsPassed requires a green test run for the changed scope.
	KeyTestsPassed Key = "tests_passed"
	// KeyCoverageOK requires line coverage at or above a configured threshold.
	KeyCoverageOK Key = "coverage_ok"
	// KeyLintClean requires the changed scope to pass lint.
	KeyLintClean Key = "lint_clean"
	// KeyVerification requires a concrete verification artifact (e.g. a
	// browser screenshot, an exported report, an integration trace).
	KeyVerification Key = "verification"
)

// Requirement is a single evidence item a gate requires.
type Requirement struct {
	// Key is the evidence identifier.
	Key Key
	// Description is a human-readable reason shown when unmet.
	Description string
}

// Gate is a named, ordered set of requirements. All requirements must be
// satisfied for the gate to open.
type Gate struct {
	// Name identifies the gate (e.g. "TDD", "consequence").
	Name string
	// Requirements are the required evidence, in order.
	Requirements []Requirement
}

// PresetTDDGate returns a gate enforcing the TDD core loop: tests must pass,
// coverage must be met, and lint must be clean before a step is accepted.
func PresetTDDGate(requireLint bool) *Gate {
	reqs := []Requirement{
		{Key: KeyTestsPassed, Description: "a passing test for the changed scope"},
		{Key: KeyCoverageOK, Description: "line coverage above the project threshold"},
	}
	if requireLint {
		reqs = append(reqs, Requirement{Key: KeyLintClean, Description: "clean lint on the changed scope"})
	}
	return &Gate{Name: "TDD", Requirements: reqs}
}

// ConsequenceGate returns a gate that additionally demands a concrete
// verification artifact before a consequential (destructive/irreversible)
// step is allowed.
func ConsequenceGate() *Gate {
	return &Gate{
		Name: "consequence",
		Requirements: []Requirement{
			{Key: KeyTestsPassed, Description: "a passing test for the changed scope"},
			{Key: KeyVerification, Description: "a concrete verification artifact"},
		},
	}
}

// RequiresLint reports whether the gate requires KeyLintClean evidence.
func (g *Gate) RequiresLint() bool {
	if g == nil {
		return false
	}
	for _, r := range g.Requirements {
		if r.Key == KeyLintClean {
			return true
		}
	}
	return false
}

// Evaluate checks the supplied evidence against the gate. It returns allowed=
// true only when every requirement is satisfied. On deny it returns the
// descriptions of the unmet items (deny-by-default).
func (g *Gate) Evaluate(evidence map[Key]bool) (allowed bool, unmet []string, err error) {
	if g == nil {
		return false, nil, fmt.Errorf("evidence: nil gate")
	}
	for _, r := range g.Requirements {
		if !evidence[r.Key] {
			unmet = append(unmet, r.Description)
		}
	}
	return len(unmet) == 0, unmet, nil
}