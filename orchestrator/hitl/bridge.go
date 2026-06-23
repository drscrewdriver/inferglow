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

// Package hitl provides a bridge between the flow engine and the approval
// framework, enabling human-in-the-loop (HITL) workflows.
//
// The bridge connects flow pause/resume with approval request/decision:
//   - Flow pauses at a checkpoint → approval request submitted → wait for decision → resume
//
// This package lives in the orchestrator layer because it depends on both
// flow/ and approval/, and flow/ must not import approval/ directly.
package hitl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/inferglow/approval"
	"github.com/inferglow/flow"
)

// Bridge connects flow pause/resume with the approval framework.
// It is safe for concurrent use.
type Bridge struct {
	manager *approval.PolicyApprovalManager

	// mu protects pendingCh.
	mu       sync.Mutex
	pendingCh map[string]chan *approval.Decision // recordID → decision channel
}

// NewBridge creates a Bridge backed by the given approval manager.
func NewBridge(mgr *approval.PolicyApprovalManager) *Bridge {
	return &Bridge{
		manager:   mgr,
		pendingCh: make(map[string]chan *approval.Decision),
	}
}

// PauseForApproval pauses the given execution, submits the approval request,
// and blocks until a decision is reached or the context is cancelled.
//
// If the approval is resolved immediately (e.g. auto-approve by policy),
// the decision is returned without pausing. Otherwise the flow is paused
// (with checkpoint if the flow has auto-checkpoint enabled) and the method
// blocks until ResolveApproval is called or ctx is done.
//
// Returns the decision and the PausePoint (non-nil only when pending).
func (b *Bridge) PauseForApproval(
	ctx context.Context,
	f *flow.Flow,
	exec *flow.Execution,
	req *approval.Request,
) (*approval.Decision, *flow.PausePoint, error) {
	// Submit the approval request.
	record, err := b.manager.Submit(req)
	if err != nil {
		return nil, nil, fmt.Errorf("hitl: submit approval: %w", err)
	}

	// Fast path: immediately resolved.
	if record.Status != approval.DecisionPending {
		return &approval.Decision{
			Status: record.Status,
			Reason: "resolved immediately",
		}, nil, nil
	}

	// Slow path: pause the flow and wait for manual resolution.
	pp := f.Pause(exec, fmt.Sprintf("awaiting approval: %s", req.RequestID))

	// Register a channel for this pending record.
	ch := make(chan *approval.Decision, 1)
	b.mu.Lock()
	b.pendingCh[record.ID] = ch
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pendingCh, record.ID)
		b.mu.Unlock()
	}()

	// Determine effective timeout.
	timeout := approval.AutoApproveTimeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case d := <-ch:
		return d, pp, nil
	case <-timer.C:
		// Timeout: auto-deny or escalate.
		return &approval.Decision{
			Status:  approval.DecisionDenied,
			Reason:  fmt.Sprintf("approval timeout (%s), escalation=%s", timeout, req.Escalation),
			Handler: "hitl-bridge-timeout",
		}, pp, nil
	case <-ctx.Done():
		return nil, pp, fmt.Errorf("hitl: context cancelled: %w", ctx.Err())
	}
}

// ResolveApproval resolves a pending approval record and notifies any
// goroutine waiting in PauseForApproval.
func (b *Bridge) ResolveApproval(recordID string, approved bool, approver string) error {
	record, err := b.manager.ResolveRecord(recordID, approved, approver)
	if err != nil {
		return fmt.Errorf("hitl: resolve record: %w", err)
	}

	b.mu.Lock()
	ch, ok := b.pendingCh[recordID]
	b.mu.Unlock()

	if ok {
		ch <- &approval.Decision{
			Status: record.Status,
			Reason: fmt.Sprintf("resolved by %s", approver),
		}
	}
	return nil
}

// Manager returns the underlying approval manager for direct access.
func (b *Bridge) Manager() *approval.PolicyApprovalManager {
	return b.manager
}
