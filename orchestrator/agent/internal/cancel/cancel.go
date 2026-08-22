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

// Package cancel implements cancel modes, handles, and the CancelManager that
// coordinates cancel requests with the agent executeLoop safe-points. It is an
// internal implementation detail of the agent package; the agent package
// re-exports the public types and helpers via type aliases so existing callers
// are unaffected.
package cancel

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/inferglow/orchestrator/agent/internal/turnloop"
)

// PreemptMode specifies how user input should interrupt a running agent.
type PreemptMode int

const (
	// PreemptQueue waits for the current turn to complete, then processes
	// the new input in the next turn. No cancel is issued.
	PreemptQueue PreemptMode = iota
	// PreemptSafePoint interrupts at the next planning-phase boundary,
	// preserving the current state.
	PreemptSafePoint
	// PreemptForce terminates the current execution immediately and
	// starts a new turn.
	PreemptForce
)

// String returns a human-readable representation of the preempt mode.
func (m PreemptMode) String() string {
	switch m {
	case PreemptQueue:
		return "queue"
	case PreemptSafePoint:
		return "safe_point"
	case PreemptForce:
		return "force"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// CancelMode specifies when an agent should be canceled. Modes can be
// combined with bitwise OR so that whichever safe-point is reached first
// triggers the cancel. CancelImmediate (0) cancels at any point without
// waiting for a safe-point.
type CancelMode int

const (
	// CancelImmediate cancels the agent as soon as the signal is received,
	// without waiting for a ChatModel or ToolCalls safe-point.
	CancelImmediate CancelMode = 0
	// CancelAfterChatModel cancels after the current LLM call completes.
	CancelAfterChatModel CancelMode = 1 << iota
	// CancelAfterToolCalls cancels after the current tool batch completes.
	CancelAfterToolCalls
)

// String returns a human-readable representation of the cancel mode.
// For combined modes the parts are joined with "|".
func (m CancelMode) String() string {
	if m == CancelImmediate {
		return "immediate"
	}
	var parts []string
	if m&CancelAfterChatModel != 0 {
		parts = append(parts, "after_chat_model")
	}
	if m&CancelAfterToolCalls != 0 {
		parts = append(parts, "after_tool_calls")
	}
	if len(parts) == 0 {
		return "immediate"
	}
	return strings.Join(parts, "|")
}

// ErrCancelTimeout is returned by CancelHandle.Wait when a safe-point cancel
// timed out and was escalated to CancelImmediate.
var ErrCancelTimeout = errors.New("cancel timed out, escalated to immediate")

// ErrTurnInterrupted reports that the current turn was interrupted by a
// user input (PreemptSafePoint or PreemptForce). It is not a failure;
// callers may check it with errors.Is to distinguish steering from errors.
var ErrTurnInterrupted = errors.New("agent: turn interrupted by user input")

// CancelHandle represents a cancel operation that can be waited on. The done
// channel is closed when the cancel is complete; err holds the outcome (nil on
// success, ErrCancelTimeout on escalation).
type CancelHandle struct {
	done chan struct{}
	err  error
	// req is non-nil when the handle was created via CancelManager.Cancel.
	// Wait reads the request's err so that escalation errors set after
	// handle creation are reflected. For standalone handles (constructed
	// directly in tests) req is nil and Wait reads done/err directly.
	req *cancelRequest
}

// Wait blocks until the cancel is complete and returns the outcome error.
// nil indicates the cancel was honored cleanly; ErrCancelTimeout indicates a
// safe-point cancel timed out and escalated to immediate.
func (h *CancelHandle) Wait() error {
	if h.req != nil {
		<-h.req.done
		return h.req.err
	}
	if h.done != nil {
		<-h.done
	}
	return h.err
}

// cancelRequest is the internal representation of a cancel request, created by
// CancelManager.Cancel and consumed by the executeLoop at safe-points.
type cancelRequest struct {
	mode        CancelMode
	preemptMode PreemptMode
	recursive   bool
	timeout     time.Duration
	reason      string
	done        chan struct{}
	err         error
}

// CancelManager coordinates cancel requests with the executeLoop. The
// executeLoop polls HasPendingCancel/CheckCancel at safe-points; Cancel sets
// the active request and (for CancelImmediate) preempts the TurnLoop.
type CancelManager struct {
	mu              sync.Mutex
	requestCh       chan *cancelRequest // buffered notification channel
	activeReq       *cancelRequest      // currently pending request
	turnLoop        *turnloop.TurnLoop
	timeoutDeadline *time.Time
}

// NewCancelManager creates a CancelManager associated with the given TurnLoop.
// The TurnLoop is preempted when an immediate cancel (or timeout escalation)
// fires; it may be nil in which case preemption is skipped.
func NewCancelManager(turnLoop *turnloop.TurnLoop) *CancelManager {
	return &CancelManager{
		requestCh: make(chan *cancelRequest, 1),
		turnLoop:  turnLoop,
	}
}

// CancelOption configures a cancel request.
type CancelOption func(*cancelConfig)

type cancelConfig struct {
	recursive bool
	timeout   time.Duration
	reason    string
}

// WithRecursive opts into propagating the cancel to child agents.
func WithRecursive() CancelOption {
	return func(c *cancelConfig) {
		c.recursive = true
	}
}

// WithCancelTimeout sets a grace period before a safe-point cancel escalates
// to CancelImmediate. A zero or negative duration disables escalation.
func WithCancelTimeout(d time.Duration) CancelOption {
	return func(c *cancelConfig) {
		c.timeout = d
	}
}

// WithReason attaches a human-readable reason string to a cancel request.
// The reason is stored on the cancelRequest and can be used for diagnostics.
func WithReason(reason string) CancelOption {
	return func(c *cancelConfig) {
		c.reason = reason
	}
}

// Cancel creates a cancel request with the given mode and options, registers
// it as the active request, notifies the executeLoop via requestCh, and
// returns a CancelHandle. For CancelImmediate the TurnLoop is preempted
// immediately so any in-flight stream select unblocks right away.
//
// If a previous active request exists it is superseded: its done channel is
// closed (so prior waiters unblock with a nil outcome) before the new request
// takes its place.
func (m *CancelManager) Cancel(mode CancelMode, opts ...CancelOption) *CancelHandle {
	cfg := &cancelConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	req := &cancelRequest{
		mode:        mode,
		preemptMode: 0, // set by CancelWithMode when applicable
		recursive:   cfg.recursive,
		timeout:     cfg.timeout,
		reason:      cfg.reason,
		done:        make(chan struct{}),
	}
	h := &CancelHandle{done: req.done, req: req}

	m.mu.Lock()
	if m.activeReq != nil {
		// Supersede any prior request: unblock its waiters with a nil
		// outcome before installing the new request.
		close(m.activeReq.done)
	}
	m.activeReq = req
	if cfg.timeout > 0 {
		deadline := time.Now().Add(cfg.timeout)
		m.timeoutDeadline = &deadline
	} else {
		m.timeoutDeadline = nil
	}
	m.mu.Unlock()

	// Non-blocking notification to the executeLoop. If the buffer is full
	// the send is dropped — the executeLoop will observe the request via
	// HasPendingCancel polling at the next safe-point.
	select {
	case m.requestCh <- req:
	default:
	}

	// For immediate cancel, preempt the turn loop right away so an
	// in-flight stream select unblocks immediately.
	if mode == CancelImmediate && m.turnLoop != nil {
		_ = m.turnLoop.Preempt("cancel immediate")
	}

	return h
}

// CheckCancel reports whether the active cancel request should be honored at
// the given safe-point. The executeLoop calls this at each integration point
// with the mode corresponding to that safe-point.
//
// Semantics:
//   - If the active request mode is CancelImmediate, returns true at any
//     safe-point (immediate cancels do not wait).
//   - If the queried safe-point is CancelImmediate, returns false unless the
//     active request is also CancelImmediate (a safe-point request does not
//     fire at an immediate checkpoint).
//   - Otherwise returns true iff the active request's mode has any bit in
//     common with the queried safe-point (bitwise OR support).
func (m *CancelManager) CheckCancel(mode CancelMode) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeReq == nil {
		return false
	}
	reqMode := m.activeReq.mode
	if reqMode == CancelImmediate {
		return true
	}
	if mode == CancelImmediate {
		return false
	}
	return (reqMode & mode) != 0
}

// CompleteCancel is called by the executeLoop when a cancel is honored. It
// records the outcome error (preserving any ErrCancelTimeout already set by an
// escalation), closes the active request's done channel so CancelHandle.Wait
// unblocks, and clears the active request.
func (m *CancelManager) CompleteCancel(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeReq == nil {
		return
	}
	// Preserve an escalation error already set by CheckTimeoutEscalation.
	if m.activeReq.err == nil {
		m.activeReq.err = err
	}
	close(m.activeReq.done)
	m.activeReq = nil
	m.timeoutDeadline = nil
}

// HasPendingCancel reports whether there is an active (not yet completed)
// cancel request.
func (m *CancelManager) HasPendingCancel() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeReq != nil
}

// CancelWithMode maps a PreemptMode to the appropriate CancelMode and
// issues the cancel. PreemptQueue is a no-op here — the caller (InputQueue)
// handles enqueueing without cancelling.
func (m *CancelManager) CancelWithMode(mode PreemptMode, reason string) {
	switch mode {
	case PreemptQueue:
		// No cancel; the InputQueue drains at the turn boundary.
		return
	case PreemptSafePoint:
		h := m.Cancel(CancelAfterChatModel|CancelAfterToolCalls, WithReason(reason))
		if h.req != nil {
			h.req.preemptMode = PreemptSafePoint
		}
	case PreemptForce:
		h := m.Cancel(CancelImmediate, WithReason(reason))
		if h.req != nil {
			h.req.preemptMode = PreemptForce
		}
	}
}

// CheckTimeoutEscalation upgrades the active cancel to CancelImmediate if a
// timeout was configured (via WithCancelTimeout) and has now elapsed. On
// escalation it sets the active request's err to ErrCancelTimeout and preempts
// the TurnLoop so an in-flight stream unblocks. Returns true if escalation
// occurred.
//
// Calling this when there is no active request, when the active request is
// already CancelImmediate, or when no timeout is configured returns false.
func (m *CancelManager) CheckTimeoutEscalation() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeReq == nil {
		return false
	}
	if m.activeReq.mode == CancelImmediate {
		return false
	}
	if m.timeoutDeadline == nil {
		return false
	}
	if time.Now().Before(*m.timeoutDeadline) {
		return false
	}
	// Escalate: flip to immediate so the next CheckCancel(CancelImmediate)
	// (or any safe-point check, since CancelImmediate matches all) fires.
	m.activeReq.mode = CancelImmediate
	m.activeReq.err = ErrCancelTimeout
	if m.turnLoop != nil {
		_ = m.turnLoop.Preempt("cancel timeout escalation")
	}
	return true
}
