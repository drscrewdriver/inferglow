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
	"github.com/inferglow/orchestrator/agent/internal/cancel"
)

// This file re-exports the cancel state machine (now living in the
// internal/cancel subpackage) so the public agent API remains stable. The type
// aliases make agent.CancelManager and cancel.CancelManager the identical type,
// so existing call sites (including Engine) compile unchanged. The TurnLoop
// argument accepted by NewCancelManager is itself an alias for
// turnloop.TurnLoop, so callers passing an agent.TurnLoop compile unchanged.

// CancelMode specifies when an agent should be canceled. Modes can be
// combined with bitwise OR so that whichever safe-point is reached first
// triggers the cancel. CancelImmediate (0) cancels at any point without
// waiting for a safe-point.
type CancelMode = cancel.CancelMode

const (
	// CancelImmediate cancels the agent as soon as the signal is received,
	// without waiting for a ChatModel or ToolCalls safe-point.
	CancelImmediate = cancel.CancelImmediate
	// CancelAfterChatModel cancels after the current LLM call completes.
	CancelAfterChatModel = cancel.CancelAfterChatModel
	// CancelAfterToolCalls cancels after the current tool batch completes.
	CancelAfterToolCalls = cancel.CancelAfterToolCalls
)

// ErrCancelTimeout is returned by CancelHandle.Wait when a safe-point cancel
// timed out and was escalated to CancelImmediate.
var ErrCancelTimeout = cancel.ErrCancelTimeout

// ErrTurnInterrupted reports that the current turn was interrupted by a
// user input (PreemptSafePoint or PreemptForce). It is not a failure;
// callers may check it with errors.Is to distinguish steering from errors.
var ErrTurnInterrupted = cancel.ErrTurnInterrupted

// CancelHandle represents a cancel operation that can be waited on.
type CancelHandle = cancel.CancelHandle

// CancelManager coordinates cancel requests with the executeLoop.
type CancelManager = cancel.CancelManager

// NewCancelManager creates a CancelManager associated with the given TurnLoop.
var NewCancelManager = cancel.NewCancelManager

// CancelOption configures a cancel request.
type CancelOption = cancel.CancelOption

// WithRecursive opts into propagating the cancel to child agents.
var WithRecursive = cancel.WithRecursive

// WithCancelTimeout sets a grace period before a safe-point cancel escalates
// to CancelImmediate.
var WithCancelTimeout = cancel.WithCancelTimeout

// WithReason attaches a human-readable reason to a cancel request.
var WithReason = cancel.WithReason

// PreemptMode specifies how user input should interrupt a running agent.
type PreemptMode = cancel.PreemptMode

const (
	// PreemptQueue waits for the current turn to complete.
	PreemptQueue = cancel.PreemptQueue
	// PreemptSafePoint interrupts at the next planning-phase boundary.
	PreemptSafePoint = cancel.PreemptSafePoint
	// PreemptForce terminates immediately.
	PreemptForce = cancel.PreemptForce
)
