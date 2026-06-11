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

// Package turnloop implements the thread-safe three-state state machine used
// by the agent's PLAN → EXECUTE turn loop. It is an internal implementation
// detail of the agent package; the agent package re-exports the public types
// and helpers via type aliases so existing callers are unaffected.
package turnloop

import (
	"errors"
	"fmt"
	"sync"
)

// TurnPhase represents the current state of an agent's turn loop.
//
// State machine:
//
//	idle ──EnterPlanning──▶ planning ──EnterActive──▶ active ──EnterIdle──▶ idle
//	  ▲                       │                                          ▲
//	  └─────────Preempt────────┘────────────Preempt──────────────────────┘
//
// Preempt is valid from planning or active and always transitions back to idle.
type TurnPhase uint8

const (
	// TurnPhaseIdle means the agent is idle, waiting for input.
	TurnPhaseIdle TurnPhase = iota
	// TurnPhasePlanning means an LLM call is in progress.
	TurnPhasePlanning
	// TurnPhaseActive means tool execution is in progress.
	TurnPhaseActive
)

// String returns a human-readable representation of the phase: "idle",
// "planning", or "active".
func (p TurnPhase) String() string {
	switch p {
	case TurnPhaseIdle:
		return "idle"
	case TurnPhasePlanning:
		return "planning"
	case TurnPhaseActive:
		return "active"
	default:
		return fmt.Sprintf("unknown(%d)", p)
	}
}

// ErrCannotPreemptIdle is returned by TurnLoop.Preempt when the loop is in the
// idle phase and there is no in-flight turn to preempt.
var ErrCannotPreemptIdle = errors.New("agent: cannot preempt idle turn")

// TurnLoop is a thread-safe three-state state machine for the agent's
// PLAN → EXECUTE turn loop, supporting preemption of the current turn.
//
// The loop transitions through TurnPhaseIdle → TurnPhasePlanning →
// TurnPhaseActive → TurnPhaseIdle. At any point during planning or active,
// Preempt may be called to interrupt the in-flight turn: it closes the current
// preempt channel (unblocking any caller selecting on it) and records the
// reason. Callers obtain the preempt channel from EnterPlanning/EnterActive and
// select on it alongside their LLM call or tool execution to observe
// interruption.
type TurnLoop struct {
	mu            sync.Mutex
	phase         TurnPhase
	preemptCh     chan struct{} // closed when preempt is requested
	preempted     bool
	preemptReason string
}

// NewTurnLoop creates a TurnLoop starting in the TurnPhaseIdle phase.
func NewTurnLoop() *TurnLoop {
	return &TurnLoop{
		phase: TurnPhaseIdle,
	}
}

// Phase returns the current turn phase. Safe for concurrent use.
func (l *TurnLoop) Phase() TurnPhase {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.phase
}

// EnterPlanning transitions the loop to TurnPhasePlanning and returns a new
// preempt channel that callers should select on during the LLM call. Safe for
// concurrent use.
func (l *TurnLoop) EnterPlanning() chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.phase = TurnPhasePlanning
	l.preemptCh = make(chan struct{})
	return l.preemptCh
}

// EnterActive transitions the loop to TurnPhaseActive and returns a new preempt
// channel that callers should select on during tool execution. Safe for
// concurrent use.
func (l *TurnLoop) EnterActive() chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.phase = TurnPhaseActive
	l.preemptCh = make(chan struct{})
	return l.preemptCh
}

// EnterIdle transitions the loop to TurnPhaseIdle, closing any in-flight preempt
// channel so callers selecting on it unblock. It returns the preempt channel
// (nil after the transition). Safe for concurrent use.
func (l *TurnLoop) EnterIdle() chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.phase = TurnPhaseIdle
	if l.preemptCh != nil {
		close(l.preemptCh)
		l.preemptCh = nil
	}
	return l.preemptCh
}

// Preempt interrupts the current turn. If the phase is idle it returns
// ErrCannotPreemptIdle. Otherwise it closes the in-flight preempt channel to
// signal interruption, records the reason, sets the preempted flag, and
// transitions to TurnPhaseIdle. Safe for concurrent use.
func (l *TurnLoop) Preempt(reason string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase == TurnPhaseIdle {
		return ErrCannotPreemptIdle
	}
	if l.preemptCh != nil {
		close(l.preemptCh)
		l.preemptCh = nil
	}
	l.preempted = true
	l.preemptReason = reason
	l.phase = TurnPhaseIdle
	return nil
}

// IsPreempted reports whether Preempt has been called since the last Reset.
// Safe for concurrent use.
func (l *TurnLoop) IsPreempted() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.preempted
}

// PreemptReason returns the reason string from the last Preempt call, or the
// empty string if no preempt has occurred. Safe for concurrent use.
func (l *TurnLoop) PreemptReason() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.preemptReason
}

// Reset clears the preempted state and reason, closes any in-flight preempt
// channel, and transitions the loop back to TurnPhaseIdle. Safe for concurrent
// use.
func (l *TurnLoop) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.preempted = false
	l.preemptReason = ""
	l.phase = TurnPhaseIdle
	if l.preemptCh != nil {
		close(l.preemptCh)
		l.preemptCh = nil
	}
}
