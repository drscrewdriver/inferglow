// Package flowdef — simple_types.go defines the simplified workflow format
// used by bug_fix_workflow.yaml and similar files. The format uses a flat
// stages list with next/rollback_to pointers (linked-list style) rather
// than the structured FlowDef's steps/depends_on DAG style.
//
// SimpleWorkflow is converted to FlowDef by ConvertSimpleToFlowDef in
// simple_converter.go. The Loader auto-detects the format in LoadFile.
package flowdef

// SimpleWorkflow is the top-level structure of the simplified workflow YAML.
//
// Example:
//
//	name: bug-fix-workflow
//	description: ...
//	start_stage: triage
//	end_stage: review
//	stages:
//	  - name: triage
//	    label: 问题理解
//	    next: branch
//	    rollback_to: null
//	  - name: branch
//	    label: 切分支
//	    next: locate
//	    rollback_to: triage
type SimpleWorkflow struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	StartStage  string        `yaml:"start_stage"`
	EndStage    string        `yaml:"end_stage"`
	Stages      []SimpleStage `yaml:"stages"`
}

// SimpleStage describes a single stage in the simplified workflow format.
type SimpleStage struct {
	Name        string  `yaml:"name"`
	Label       string  `yaml:"label"`
	Description string  `yaml:"description"`
	Next        *string `yaml:"next"`        // nil = terminal (no successor)
	RollbackTo  *string `yaml:"rollback_to"` // nil = no rollback target
}
