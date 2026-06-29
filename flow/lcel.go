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

import (
	"context"
	"fmt"
	"sync"
)

// Chain is a lightweight LCEL-style (LangChain Expression Language) linear
// pipeline of StepFuncs. Each step's output becomes the next step's input.
//
// Chain is simpler than FlowBuilder (pure linear, no branches/conditions) and
// lighter than TriggerFlow (no generics/signals). It is ideal for quick
// prompt → model → parser pipelines.
//
// Use Build() to convert a Chain into a *Flow for execution via the standard
// engine, or call Invoke() directly for synchronous execution.
type Chain struct {
	steps []chainStep
}

type chainStep struct {
	name string
	fn   StepFunc
}

// LCEL creates a new Chain with a single initial step.
//
//	chain := LCEL("prompt", promptFn).Pipe("model", modelFn).Pipe("parser", parseFn)
func LCEL(name string, fn StepFunc) *Chain {
	return &Chain{
		steps: []chainStep{{name: name, fn: fn}},
	}
}

// Pipe appends a step to the chain and returns the chain for chaining.
func (c *Chain) Pipe(name string, fn StepFunc) *Chain {
	c.steps = append(c.steps, chainStep{name: name, fn: fn})
	return c
}

// Invoke executes the chain synchronously, threading the output of each step
// as the input to the next. Returns the final step's output.
func (c *Chain) Invoke(ctx context.Context, input any) (any, error) {
	current := input
	for _, s := range c.steps {
		out, err := s.fn(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", s.name, err)
		}
		current = out
	}
	return current, nil
}

// Build converts the Chain into a *Flow using FlowBuilder. The resulting Flow
// is a linear sequence of steps connected by edges.
func (c *Chain) Build() *Flow {
	if len(c.steps) == 0 {
		return NewFlow().Build()
	}
	fb := NewFlow()
	first := &Step{Name: c.steps[0].name, Func: c.steps[0].fn}
	fb.AddStep(first)
	for _, s := range c.steps[1:] {
		fb.To(&Step{Name: s.name, Func: s.fn})
	}
	return fb.Build()
}

// Len returns the number of steps in the chain.
func (c *Chain) Len() int { return len(c.steps) }

// Names returns the step names in order.
func (c *Chain) Names() []string {
	out := make([]string, len(c.steps))
	for i, s := range c.steps {
		out[i] = s.name
	}
	return out
}

// --- Combinators ---

// MapChain returns a StepFunc that applies fn to each element of a slice input.
// Input must be []any; output is []any with fn applied element-wise.
// Errors from any element stop processing and are returned immediately.
func MapChain(name string, fn StepFunc) StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		items, ok := input.([]any)
		if !ok {
			return nil, fmt.Errorf("MapChain %q: input must be []any, got %T", name, input)
		}
		out := make([]any, len(items))
		for i, item := range items {
			res, err := fn(ctx, item)
			if err != nil {
				return nil, fmt.Errorf("MapChain %q[%d]: %w", name, i, err)
			}
			out[i] = res
		}
		return out, nil
	}
}

// BranchChain returns a StepFunc that evaluates cond and delegates to either
// trueFn or falseFn based on the result.
func BranchChain(cond func(any) bool, trueFn, falseFn StepFunc) StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		if cond(input) {
			return trueFn(ctx, input)
		}
		return falseFn(ctx, input)
	}
}

// ParallelChain executes multiple chains concurrently with the same input
// and returns a map[string]any keyed by chain name. All chains run in parallel;
// the first error terminates the context and is returned.
func ParallelChain(chains ...*Chain) StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		type result struct {
			name string
			val  any
			err  error
		}
		ch := make(chan result, len(chains))
		var wg sync.WaitGroup
		for _, c := range chains {
			wg.Add(1)
			go func(chain *Chain) {
				defer wg.Done()
				name := "chain"
				if len(chain.steps) > 0 {
					name = chain.steps[0].name
				}
				val, err := chain.Invoke(ctx, input)
				ch <- result{name: name, val: val, err: err}
			}(c)
		}
		wg.Wait()
		close(ch)

		out := make(map[string]any, len(chains))
		for r := range ch {
			if r.err != nil {
				return nil, fmt.Errorf("parallel %q: %w", r.name, r.err)
			}
			out[r.name] = r.val
		}
		return out, nil
	}
}

// FuncChain wraps a plain function as a StepFunc for use in chains.
func FuncChain(fn func(ctx context.Context, input any) (any, error)) StepFunc {
	return fn
}

// ConstChain returns a StepFunc that ignores input and always returns val.
func ConstChain(val any) StepFunc {
	return func(_ context.Context, _ any) (any, error) {
		return val, nil
	}
}
