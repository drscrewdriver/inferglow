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

package hitl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/inferglow/approval"
	"github.com/inferglow/flow"
)

// simpleFlow builds a two-step flow for testing pause/resume.
func simpleFlow() *flow.Flow {
	stepA := flow.NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return "A", nil
	}).Build()
	stepB := flow.NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return "B", nil
	}).Build()
	return flow.NewFlow().AddStep(stepA).To(stepB).Build()
}

func TestPauseForApproval_AutoApprove(t *testing.T) {
	mgr := approval.NewPolicyApprovalManager()
	_ = mgr.RegisterHandler(&approval.AutoApproveHandler{}, true)
	_ = mgr.SetDefaultHandler("auto_approve")

	bridge := NewBridge(mgr)
	f := simpleFlow()
	exec := f.Execute(context.Background(), "start")

	req := &approval.Request{
		RequestID:  "req-1",
		Source:     "test",
		Capability: "bash_execute",
	}

	decision, pp, err := bridge.PauseForApproval(context.Background(), f, exec, req)
	if err != nil {
		t.Fatalf("PauseForApproval error: %v", err)
	}
	if pp != nil {
		t.Error("expected nil PausePoint for auto-approve")
	}
	if decision.Status != approval.DecisionAllowed && decision.Status != approval.DecisionApproved {
		t.Errorf("expected approved/allowed, got %s", decision.Status)
	}
}

func TestPauseForApproval_PendingThenResolve(t *testing.T) {
	mgr := approval.NewPolicyApprovalManager()
	// No default handler → Submit returns pending record.
	bridge := NewBridge(mgr)
	f := simpleFlow()
	exec := f.Execute(context.Background(), "start")

	req := &approval.Request{
		RequestID: "req-2",
		Source:    "test",
		Timeout:   5 * time.Second,
	}

	var decision *approval.Decision
	var pp *flow.PausePoint
	var pauseErr error

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		decision, pp, pauseErr = bridge.PauseForApproval(context.Background(), f, exec, req)
	}()

	// Give the goroutine time to register the pending channel.
	time.Sleep(50 * time.Millisecond)

	// Resolve the pending record.
	records := mgr.ListRecords()
	if len(records) == 0 {
		t.Fatal("expected at least one pending record")
	}
	recordID := records[0].ID

	if err := bridge.ResolveApproval(recordID, true, "tester"); err != nil {
		t.Fatalf("ResolveApproval error: %v", err)
	}

	wg.Wait()

	if pauseErr != nil {
		t.Fatalf("PauseForApproval error: %v", pauseErr)
	}
	if pp == nil {
		t.Error("expected non-nil PausePoint for pending approval")
	}
	if decision.Status != approval.DecisionApproved {
		t.Errorf("expected approved, got %s", decision.Status)
	}
}

func TestPauseForApproval_Timeout(t *testing.T) {
	mgr := approval.NewPolicyApprovalManager()
	bridge := NewBridge(mgr)
	f := simpleFlow()
	exec := f.Execute(context.Background(), "start")

	req := &approval.Request{
		RequestID:  "req-3",
		Source:     "test",
		Timeout:    100 * time.Millisecond,
		Escalation: "auto_deny",
	}

	decision, pp, err := bridge.PauseForApproval(context.Background(), f, exec, req)
	if err != nil {
		t.Fatalf("PauseForApproval error: %v", err)
	}
	if pp == nil {
		t.Error("expected non-nil PausePoint after timeout")
	}
	if decision.Status != approval.DecisionDenied {
		t.Errorf("expected denied after timeout, got %s", decision.Status)
	}
}
