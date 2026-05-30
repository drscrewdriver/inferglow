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

package blocks

import (
	"context"

	"github.com/inferglow/flow"
)

// ReasonBlock performs LLM reasoning.
type ReasonBlock struct {
	// ModelName is the model to use for reasoning.
	ModelName string
}

// Name returns "reason".
func (b *ReasonBlock) Name() string { return "reason" }

// BuildOperators produces a result_sink operator for the reasoning step.
func (b *ReasonBlock) BuildOperators(_ context.Context, _ *BlockBlueprint) ([]*flow.Operator, error) {
	return []*flow.Operator{
		{
			ID:   "reason_op",
			Kind: flow.OpResultSink,
			Name: "reason",
			Options: map[string]any{
				"model": b.ModelName,
			},
		},
	}, nil
}

// Execute performs the reasoning step.
func (b *ReasonBlock) Execute(_ context.Context, input any) (any, error) {
	// Placeholder: in production this would call the model.
	return map[string]any{
		"block":  "reason",
		"model":  b.ModelName,
		"input":  input,
		"status": "completed",
	}, nil
}

// ActBlock performs action execution.
type ActBlock struct {
	// AllowedActions restricts which actions can be invoked.
	AllowedActions []string
}

// Name returns "act".
func (b *ActBlock) Name() string { return "act" }

// BuildOperators produces operators for action dispatch.
func (b *ActBlock) BuildOperators(_ context.Context, _ *BlockBlueprint) ([]*flow.Operator, error) {
	return []*flow.Operator{
		{
			ID:   "act_op",
			Kind: flow.OpResultSink,
			Name: "act",
			Options: map[string]any{
				"allowed_actions": b.AllowedActions,
			},
		},
	}, nil
}

// Execute performs the action step.
func (b *ActBlock) Execute(_ context.Context, input any) (any, error) {
	return map[string]any{
		"block":           "act",
		"allowed_actions": b.AllowedActions,
		"input":           input,
		"status":          "completed",
	}, nil
}

// IntentBlock performs intent classification.
type IntentBlock struct {
	// ModelName is the model to use for intent classification.
	ModelName string
}

// Name returns "intent".
func (b *IntentBlock) Name() string { return "intent" }

// BuildOperators produces operators for intent classification.
func (b *IntentBlock) BuildOperators(_ context.Context, _ *BlockBlueprint) ([]*flow.Operator, error) {
	return []*flow.Operator{
		{
			ID:   "intent_op",
			Kind: flow.OpMatchRoute,
			Name: "intent",
			Options: map[string]any{
				"model": b.ModelName,
			},
		},
	}, nil
}

// Execute performs the intent classification step.
func (b *IntentBlock) Execute(_ context.Context, input any) (any, error) {
	return map[string]any{
		"block":  "intent",
		"model":  b.ModelName,
		"input":  input,
		"status": "completed",
	}, nil
}

// Compile-time interface checks.
var (
	_ FlowBlock = (*ReasonBlock)(nil)
	_ FlowBlock = (*ActBlock)(nil)
	_ FlowBlock = (*IntentBlock)(nil)
)
