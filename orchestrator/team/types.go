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

// Package team provides multi-agent coordination for InferGlow.
//
// The Coordinator orchestr multiple Agent instances, dispatching tasks through
// a private messageBus and collecting results. It uses the unified
// middleware.Handler signature from orchestrator/middleware/ for cross-cutting
// concerns (tracing, audit, rate limiting) shared with the agent package.
//
// Agent.Run has a variadic opts parameter that prevents direct satisfaction of
// the AgentRunner interface. Use AgentAdapter to wrap an *agent.Agent with
// fixed RunOptions.
package team

import (
	"context"

	"github.com/inferglow/orchestrator/agent"
)

// AgentRunner is the minimal interface a participant must satisfy.
// agent.Agent satisfies this via AgentAdapter.
type AgentRunner interface {
	Run(ctx context.Context, userMessage string) (string, error)
}

// AgentAdapter wraps an *agent.Agent with fixed RunOptions so it satisfies
// the AgentRunner interface.
type AgentAdapter struct {
	Agent *agent.Agent
	Opts  []agent.RunOption
}

// AdaptAgent creates an AgentAdapter for the given Agent.
func AdaptAgent(a *agent.Agent, opts ...agent.RunOption) *AgentAdapter {
	return &AgentAdapter{Agent: a, Opts: opts}
}

// Run delegates to the underlying Agent with the fixed options.
func (a *AgentAdapter) Run(ctx context.Context, userMessage string) (string, error) {
	return a.Agent.Run(ctx, userMessage, a.Opts...)
}

// Member is a single participant in a team coordination round.
type Member struct {
	// Agent is the runner that executes tasks for this member.
	Agent AgentRunner
	// Role identifies the member's function (e.g. "planner", "coder", "reviewer").
	Role string
	// Handoff lists the roles this member can delegate to. An empty list
	// means the member is a terminal node (no further handoff).
	Handoff []string
}

// Result is the outcome of a Coordinator.Round.
type Result struct {
	// FinalResponse is the consolidated response after all rounds.
	FinalResponse string
	// MemberOutputs maps each member's Role to its response.
	MemberOutputs map[string]string
	// Rounds is the number of coordination rounds executed.
	Rounds int
}

// RoundStep records a single member's contribution in a round.
type RoundStep struct {
	Role     string
	Response string
	Error    error
}
