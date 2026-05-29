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

package agent

import "errors"

// Sentinel errors for agent extensions.
var (
	ErrInvalidDAGInput = errors.New("agent: input is not a *taskdag.TaskDAG")
)

// AgentExtensions holds optional integrations that can be attached to an
// Agent. All fields are optional; nil means the integration is disabled.
//
// This struct is designed to be embedded or referenced from Agent so that
// zero-value means "no extensions" — fully backward compatible.
type AgentExtensions struct {
	// Strategy is the execution strategy. nil uses the default PLAN→EXECUTE loop.
	Strategy ExecutionStrategy
}

// DefaultExtensions returns an empty (zero) extensions struct.
func DefaultExtensions() *AgentExtensions {
	return &AgentExtensions{}
}
