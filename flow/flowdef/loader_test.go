package flowdef

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadFile_Valid loads a YAML file matching the SPEC.md schema and
// verifies every field round-trips correctly.
func TestLoadFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "code-review.yaml")
	yamlContent := `api_version: flowdef/v1
kind: Flow
metadata:
  name: code-review
  version: "1.0.0"
  description: "代码审查流水线"
  owner: backend-team
spec:
  inputs:
    - name: repo_url
      type: string
      required: true
    - name: branch
      type: string
      default: main
  steps:
    - name: triage
      operator: stage
      stage: triage
      inputs: { repo_url: "{{.repo_url}}", branch: "{{.branch}}" }
      schema:
        type: object
        properties:
          category: { type: string, enum: [bug, feature, refactor] }
          priority: { type: string }
    - name: coder
      operator: stage
      stage: coder
      depends_on: [triage]
      when: "{{.triage.priority != 'low'}}"
    - name: reviewer
      operator: parallel_fanout
      stage: reviewer
      depends_on: [coder]
      fanout_over: "{{.coder.files}}"
      fanout_as: file
    - name: quality_gate
      operator: match_case
      depends_on: [reviewer]
      cases:
        - when: "{{all .reviewer.passed}}"
          next: committer
        - default: true
          next: coder
    - name: committer
      operator: stage
      stage: committer
      depends_on: [quality_gate]
      outputs_to: final
  outputs:
    commit_sha: "{{.committer.sha}}"
    review_count: "{{len .reviewer}}"
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	def, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	// Metadata
	if def.APIVersion != "flowdef/v1" {
		t.Errorf("APIVersion = %q, want %q", def.APIVersion, "flowdef/v1")
	}
	if def.Kind != "Flow" {
		t.Errorf("Kind = %q, want %q", def.Kind, "Flow")
	}
	if def.Metadata.Name != "code-review" {
		t.Errorf("Metadata.Name = %q, want %q", def.Metadata.Name, "code-review")
	}
	if def.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q, want %q", def.Metadata.Version, "1.0.0")
	}
	if def.Metadata.Description != "代码审查流水线" {
		t.Errorf("Metadata.Description = %q, want %q", def.Metadata.Description, "代码审查流水线")
	}
	if def.Metadata.Owner != "backend-team" {
		t.Errorf("Metadata.Owner = %q, want %q", def.Metadata.Owner, "backend-team")
	}

	// Inputs
	if len(def.Spec.Inputs) != 2 {
		t.Fatalf("Spec.Inputs len = %d, want 2", len(def.Spec.Inputs))
	}
	if def.Spec.Inputs[0].Name != "repo_url" || !def.Spec.Inputs[0].Required {
		t.Errorf("Inputs[0] = %+v, want repo_url required", def.Spec.Inputs[0])
	}
	if def.Spec.Inputs[1].Name != "branch" || def.Spec.Inputs[1].Default != "main" {
		t.Errorf("Inputs[1] = %+v, want branch default=main", def.Spec.Inputs[1])
	}

	// Steps
	if len(def.Spec.Steps) != 5 {
		t.Fatalf("Spec.Steps len = %d, want 5", len(def.Spec.Steps))
	}
	triage := def.Spec.Steps[0]
	if triage.Name != "triage" || triage.Operator != "stage" || triage.Stage != "triage" {
		t.Errorf("triage step = %+v", triage)
	}
	if got := triage.Inputs["repo_url"]; got != "{{.repo_url}}" {
		t.Errorf("triage inputs repo_url = %v, want template string", got)
	}

	coder := def.Spec.Steps[1]
	if coder.When != "{{.triage.priority != 'low'}}" {
		t.Errorf("coder when = %q, want template", coder.When)
	}
	if len(coder.DependsOn) != 1 || coder.DependsOn[0] != "triage" {
		t.Errorf("coder depends_on = %v, want [triage]", coder.DependsOn)
	}

	reviewer := def.Spec.Steps[2]
	if reviewer.Operator != "parallel_fanout" || reviewer.FanoutOver != "{{.coder.files}}" || reviewer.FanoutAs != "file" {
		t.Errorf("reviewer = %+v", reviewer)
	}

	gate := def.Spec.Steps[3]
	if gate.Operator != "match_case" || len(gate.Cases) != 2 {
		t.Errorf("quality_gate = %+v", gate)
	}
	if gate.Cases[0].When != "{{all .reviewer.passed}}" || gate.Cases[0].Next != "committer" {
		t.Errorf("case[0] = %+v", gate.Cases[0])
	}
	if !gate.Cases[1].Default || gate.Cases[1].Next != "coder" {
		t.Errorf("case[1] (default) = %+v", gate.Cases[1])
	}

	committer := def.Spec.Steps[4]
	if committer.OutputsTo != "final" {
		t.Errorf("committer outputs_to = %q, want final", committer.OutputsTo)
	}

	// Outputs
	if len(def.Spec.Outputs) != 2 {
		t.Fatalf("Spec.Outputs len = %d, want 2", len(def.Spec.Outputs))
	}
	if def.Spec.Outputs["commit_sha"] != "{{.committer.sha}}" {
		t.Errorf("outputs commit_sha = %q", def.Spec.Outputs["commit_sha"])
	}
}

// TestLoadFile_NotFound ensures a missing file produces an error.
func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadFile_InvalidYAML ensures malformed YAML produces an error.
func TestLoadFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("::: not yaml :::\n  - [\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// TestLoadDir_Multiple loads multiple YAML files from a directory and
// verifies they are keyed by metadata.name.
func TestLoadDir_Multiple(t *testing.T) {
	dir := t.TempDir()
	flowA := `api_version: flowdef/v1
kind: Flow
metadata:
  name: flow-a
  version: "1.0.0"
spec:
  steps:
    - name: s1
      operator: stage
      stage: triage
`
	flowB := `api_version: flowdef/v1
kind: Flow
metadata:
  name: flow-b
  version: "2.0.0"
spec:
  steps:
    - name: s1
      operator: stage
      stage: coder
`
	// Also include a .yml extension to verify both are picked up.
	flowC := `api_version: flowdef/v1
kind: Flow
metadata:
  name: flow-c
  version: "3.0.0"
spec:
  steps:
    - name: s1
      operator: stage
      stage: committer
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(flowA), 0o644); err != nil {
		t.Fatalf("write a.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yml"), []byte(flowB), 0o644); err != nil {
		t.Fatalf("write b.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(flowC), 0o644); err != nil {
		t.Fatalf("write c.yaml: %v", err)
	}
	// Non-yaml file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("LoadDir returned %d defs, want 3", len(defs))
	}
	for _, name := range []string{"flow-a", "flow-b", "flow-c"} {
		d, ok := defs[name]
		if !ok {
			t.Errorf("missing def %q in map keys %v", name, mapKeys(defs))
		} else if d.Metadata.Name != name {
			t.Errorf("def %q has Metadata.Name %q", name, d.Metadata.Name)
		}
	}
}

// TestLoadDir_Empty returns an empty map (not error) for a dir with no YAML.
func TestLoadDir_Empty(t *testing.T) {
	defs, err := LoadDir(t.TempDir())
	if err != nil {
		t.Fatalf("LoadDir empty dir error: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 defs, got %d", len(defs))
	}
}

func mapKeys(m map[string]*FlowDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
