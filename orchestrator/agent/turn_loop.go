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

import (
	"github.com/inferglow/orchestrator/agent/internal/turnloop"
)

// This file re-exports the turn-loop state machine (now living in the
// internal/turnloop subpackage) so the public agent API remains stable. The
// type aliases make agent.TurnLoop and turnloop.TurnLoop the identical type,
// so existing call sites (including Engine and the cancel manager) compile
// unchanged.

// TurnPhase represents the current state of an agent's turn loop. See
// internal/turnloop for the state-machine documentation.
type TurnPhase = turnloop.TurnPhase

const (
	// TurnPhaseIdle means the agent is idle, waiting for input.
	TurnPhaseIdle = turnloop.TurnPhaseIdle
	// TurnPhasePlanning means an LLM call is in progress.
	TurnPhasePlanning = turnloop.TurnPhasePlanning
	// TurnPhaseActive means tool execution is in progress.
	TurnPhaseActive = turnloop.TurnPhaseActive
)

// ErrCannotPreemptIdle is returned by TurnLoop.Preempt when the loop is in the
// idle phase and there is no in-flight turn to preempt.
var ErrCannotPreemptIdle = turnloop.ErrCannotPreemptIdle

// TurnLoop is a thread-safe three-state state machine for the agent's
// PLAN → EXECUTE turn loop, supporting preemption of the current turn.
type TurnLoop = turnloop.TurnLoop

// NewTurnLoop creates a TurnLoop starting in the TurnPhaseIdle phase.
var NewTurnLoop = turnloop.NewTurnLoop

// TurnState is a point-in-time snapshot of the TurnLoop metadata.
type TurnState = turnloop.TurnState
