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

package flow

import "sync"

// Edge defines a connection between two steps
type Edge struct {
	From string
	To   string
}

// Flow represents an executable pipeline of Steps
//
// F-MEDIUM-12: 添加 mu sync.RWMutex 保护 steps/edges/branches/startStep 等
// map/slice 字段的并发访问。Execute/Resume 加读锁（允许多读并发），
// FlowBuilder 的 AddStep/To/If 加写锁（独占）。无锁时 Go runtime 会检测到
// concurrent map read and map write 并抛出 fatal error 终止进程。
type Flow struct {
	mu        sync.RWMutex
	steps     map[string]*Step
	edges     []Edge
	branches  []Branch
	startStep *Step

	// Checkpoint 持久化相关配置。通过 FlowOption（如 WithAutoCheckpoint /
	// WithCheckPointID / WithSerializer 等）在构建期设置。零值表示不启用
	// checkpoint。这些字段在构建后视为只读，运行期不会被并发修改。
	autoCheckpoint  bool
	checkpointStore CheckpointStore
	serializer      Serializer
	checkPointID    string
	writeToID       string
	forceNewRun     bool
	stateModifier   func(*ExecutionSnapshot) *ExecutionSnapshot
}

// FlowOption configures a Flow during construction via FlowBuilder.WithOptions.
type FlowOption func(*Flow) //nolint:revive

// Branch defines an optional conditional branch in a flow
type Branch struct {
	From      string
	Cond      func(any) bool
	TrueStep  *Step
	FalseStep *Step
}

// FlowBuilder builds Flow instances with chainable API
type FlowBuilder struct { //nolint:revive
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
//
// F-MEDIUM-12: 加写锁保护 flow.steps / flow.startStep 的并发修改。
func (fb *FlowBuilder) AddStep(step *Step) *FlowBuilder {
	fb.flow.mu.Lock()
	defer fb.flow.mu.Unlock()
	fb.flow.steps[step.Name] = step
	if fb.flow.startStep == nil {
		fb.flow.startStep = step
	}
	fb.lastStep = step
	return fb
}

// To connects the previous step (or last added step) to the given step
//
// F-MEDIUM-12: 加写锁保护 flow.edges / flow.steps 的并发修改。
func (fb *FlowBuilder) To(step *Step) *FlowBuilder {
	fb.flow.mu.Lock()
	defer fb.flow.mu.Unlock()
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
//
// F-MEDIUM-12: 加写锁保护 flow.steps / flow.branches 的并发修改。
func (fb *FlowBuilder) If(cond func(any) bool, trueStep *Step, falseStep *Step) *FlowBuilder {
	fb.flow.mu.Lock()
	defer fb.flow.mu.Unlock()
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

// WithOptions applies the given FlowOptions to the underlying Flow and returns
// the builder for chaining. Options are applied in order. Typical use:
//
//	NewFlow().AddStep(a).To(b).WithOptions(WithAutoCheckpoint(store)).Build()
//
// Options may be applied before or after AddStep/To/If since they only touch
// checkpoint-related fields, never the step graph.
func (fb *FlowBuilder) WithOptions(opts ...FlowOption) *FlowBuilder {
	for _, opt := range opts {
		opt(fb.flow)
	}
	return fb
}

// Build creates the Flow
func (fb *FlowBuilder) Build() *Flow {
	return fb.flow
}

// ApplyOptions applies the given FlowOptions to an already-built Flow in
// place. It acquires the write lock so it is safe to call after Build but
// must not run concurrently with Execute/Resume (which hold the read lock).
// Orchestrator layers use this to inject checkpoint options (e.g.
// WithAutoCheckpoint / WithStateModifier) into a Flow received from the
// caller at run time, without requiring the caller to rebuild the Flow.
func (f *Flow) ApplyOptions(opts ...FlowOption) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, opt := range opts {
		opt(f)
	}
}
