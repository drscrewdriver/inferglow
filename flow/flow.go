package flow

// Edge defines a connection between two steps
type Edge struct {
	From string
	To   string
}

// Flow represents an executable pipeline of Steps
type Flow struct {
	steps    map[string]*Step
	edges    []Edge
	branches []Branch
	startStep *Step
}

// Branch defines an optional conditional branch in a flow
type Branch struct {
	From      string
	Cond      func(any) bool
	TrueStep  *Step
	FalseStep *Step
}

// FlowBuilder builds Flow instances with chainable API
type FlowBuilder struct {
	flow     *Flow
	lastStep *Step
}

// NewFlow creates a new FlowBuilder
func NewFlow() *FlowBuilder {
	return &FlowBuilder{
		flow: &Flow{
			steps: make(map[string]*Step),
		},
	}
}

// AddStep adds a step to the flow and returns the builder for chaining
func (fb *FlowBuilder) AddStep(step *Step) *FlowBuilder {
	fb.flow.steps[step.Name] = step
	if fb.flow.startStep == nil {
		fb.flow.startStep = step
	}
	fb.lastStep = step
	return fb
}

// To connects the previous step (or last added step) to the given step
func (fb *FlowBuilder) To(step *Step) *FlowBuilder {
	if fb.lastStep != nil {
		fb.flow.edges = append(fb.flow.edges, Edge{
			From: fb.lastStep.Name,
			To:   step.Name,
		})
	}
	fb.flow.steps[step.Name] = step
	fb.lastStep = step
	return fb
}

// If adds a conditional branch: evaluates cond with the output of the last step.
// If true, executes trueStep; if false, executes falseStep.
// Both branches connect back to the flow so the caller can chain further steps.
func (fb *FlowBuilder) If(cond func(any) bool, trueStep *Step, falseStep *Step) *FlowBuilder {
	fb.flow.steps[trueStep.Name] = trueStep
	fb.flow.steps[falseStep.Name] = falseStep
	fb.flow.branches = append(fb.flow.branches, Branch{
		From:      fb.lastStep.Name,
		Cond:      cond,
		TrueStep:  trueStep,
		FalseStep: falseStep,
	})
	// Keep lastStep pointing to trueStep so subsequent .To() chains from there
	fb.lastStep = trueStep
	return fb
}

// Build creates the Flow
func (fb *FlowBuilder) Build() *Flow {
	return fb.flow
}
