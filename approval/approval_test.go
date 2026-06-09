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

package approval

import (
	"context"
	"errors"
	"testing"
)

func TestRiskExceeds(t *testing.T) {
	tests := []struct {
		a, b RiskLevel
		want bool
	}{
		{RiskNone, RiskNone, false},
		{RiskLow, RiskNone, true},
		{RiskMedium, RiskLow, true},
		{RiskHigh, RiskMedium, true},
		{RiskLow, RiskHigh, false},
		{RiskNone, RiskHigh, false},
	}
	for _, tt := range tests {
		if got := RiskExceeds(tt.a, tt.b); got != tt.want {
			t.Errorf("RiskExceeds(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAutoApproveHandler(t *testing.T) {
	h := AutoApproveHandler{}
	if h.Name() != "auto_approve" {
		t.Fatalf("Name() = %q, want %q", h.Name(), "auto_approve")
	}
	req := &Request{RequestID: "r1", Source: "action", Capability: "bash", Risk: RiskHigh}
	d, err := h.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if d.Status != DecisionApproved || !d.Approved {
		t.Errorf("expected approved, got %v", d.Status)
	}
}

func TestFailClosedHandler(t *testing.T) {
	h := FailClosedHandler{}
	if h.Name() != "fail_closed" {
		t.Fatalf("Name() = %q, want %q", h.Name(), "fail_closed")
	}
	req := &Request{RequestID: "r2", Source: "resource", Capability: "network"}
	d, err := h.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if d.Status != DecisionDenied || d.Approved {
		t.Errorf("expected denied, got %v", d.Status)
	}
}

func TestInputTimeoutFailHandler(t *testing.T) {
	h := InputTimeoutFailHandler{}
	if h.Name() != "input_timeout_fail" {
		t.Fatalf("Name() = %q, want %q", h.Name(), "input_timeout_fail")
	}
	req := &Request{RequestID: "r3", Source: "task_node", Capability: "execute"}
	d, err := h.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if d.Status != DecisionPending {
		t.Errorf("expected pending, got %v", d.Status)
	}
	if d.Metadata["timeout"] == "" {
		t.Error("expected timeout metadata to be set")
	}
}

func TestManagerRegisterAndResolve(t *testing.T) {
	m := NewPolicyApprovalManager()
	if err := m.RegisterHandler(AutoApproveHandler{}, false); err != nil {
		t.Fatalf("RegisterHandler: %v", err)
	}
	if err := m.SetDefaultHandler("auto_approve"); err != nil {
		t.Fatalf("SetDefaultHandler: %v", err)
	}
	if m.DefaultHandler() != "auto_approve" {
		t.Errorf("DefaultHandler = %q, want %q", m.DefaultHandler(), "auto_approve")
	}

	req := &Request{RequestID: "r1", Source: "action", Capability: "bash"}
	d, err := m.Resolve(context.Background(), req, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !d.Approved {
		t.Error("expected approved")
	}
}

func TestManagerDuplicateHandler(t *testing.T) {
	m := NewPolicyApprovalManager()
	if err := m.RegisterHandler(AutoApproveHandler{}, false); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := m.RegisterHandler(AutoApproveHandler{}, false)
	if !errors.Is(err, ErrHandlerExists) {
		t.Fatalf("expected ErrHandlerExists, got %v", err)
	}
	// replace=true should succeed.
	if err := m.RegisterHandler(AutoApproveHandler{}, true); err != nil {
		t.Fatalf("replace register: %v", err)
	}
}

func TestManagerNoDefault(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{RequestID: "r1"}
	_, err := m.Resolve(context.Background(), req, "")
	if !errors.Is(err, ErrNoDefaultHandler) {
		t.Fatalf("expected ErrNoDefaultHandler, got %v", err)
	}
}

func TestManagerExplicitHandler(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(FailClosedHandler{}, false)
	_ = m.RegisterHandler(AutoApproveHandler{}, false)

	req := &Request{RequestID: "r1", Source: "action", Capability: "bash"}
	// Use fail_closed explicitly.
	d, err := m.Resolve(context.Background(), req, "fail_closed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Approved {
		t.Error("expected denied from fail_closed")
	}
}

func TestManagerPolicyDenied(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.SetDefaultHandler("auto_approve")

	req := &Request{
		RequestID:  "r1",
		Source:     "action",
		Capability: "bash_execute",
		Policy: &AccessPolicy{
			DeniedCapabilities: []string{"bash_execute"},
		},
	}
	d, err := m.Resolve(context.Background(), req, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Approved {
		t.Error("expected denied by policy")
	}
	if !d.PolicyOverride {
		t.Error("expected PolicyOverride=true")
	}
}

func TestManagerPolicyAllowed(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(FailClosedHandler{}, false)
	_ = m.SetDefaultHandler("fail_closed")

	req := &Request{
		RequestID:  "r1",
		Capability: "read_only",
		Policy: &AccessPolicy{
			AllowedCapabilities: []string{"read_only"},
		},
	}
	d, err := m.Resolve(context.Background(), req, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !d.Approved {
		t.Error("expected approved by policy")
	}
}

func TestManagerPolicyRiskExceeded(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.SetDefaultHandler("auto_approve")

	req := &Request{
		RequestID: "r1",
		Risk:      RiskHigh,
		Policy: &AccessPolicy{
			MaxRiskLevel: RiskLow,
		},
	}
	d, err := m.Resolve(context.Background(), req, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.Approved {
		t.Error("expected denied due to risk")
	}
}

func TestManagerUnregister(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.SetDefaultHandler("auto_approve")

	if !m.UnregisterHandler("auto_approve") {
		t.Fatal("expected true from UnregisterHandler")
	}
	if m.DefaultHandler() != "" {
		t.Error("default handler should be cleared after unregister")
	}
	if m.UnregisterHandler("nonexistent") {
		t.Error("expected false for nonexistent handler")
	}
}

func TestManagerListHandlers(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.RegisterHandler(FailClosedHandler{}, false)

	names := m.ListHandlers()
	if len(names) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(names))
	}
}

func TestAutoAllowHandler(t *testing.T) {
	h := AutoAllowHandler{}
	if h.Name() != "auto_allow" {
		t.Fatalf("Name() = %q, want %q", h.Name(), "auto_allow")
	}
	req := &Request{RequestID: "r1", Source: "action", Capability: "bash"}
	d, err := h.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if d.Status != DecisionAllowed || !d.Approved {
		t.Errorf("expected allowed/approved, got status=%q approved=%v", d.Status, d.Approved)
	}
}

func TestDecisionAllowedConstant(t *testing.T) {
	if string(DecisionAllowed) != "allowed" {
		t.Errorf("DecisionAllowed = %q, want %q", DecisionAllowed, "allowed")
	}
}

// --- Submit / record management tests ---

func TestSubmit_NoHandler_PendingRecord(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{RequestID: "r1", Source: "sandbox", Capability: "docker"}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionPending {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionPending)
	}
	if rec.ID == "" {
		t.Error("pending record should have a non-empty ID")
	}
	if rec.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSubmit_PolicyDenied(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{
		RequestID:  "r1",
		Capability: "docker",
		Policy: &AccessPolicy{
			DeniedCapabilities: []string{"docker"},
		},
	}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionDenied {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionDenied)
	}
	if rec.ID != "" {
		t.Errorf("auto-decided record should have empty ID, got %q", rec.ID)
	}
}

func TestSubmit_PolicyAllowed(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{
		RequestID:  "r1",
		Capability: "trusted_local",
		Policy: &AccessPolicy{
			AllowedCapabilities: []string{"trusted_local"},
		},
	}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionApproved {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionApproved)
	}
}

func TestSubmit_HandlerApproved(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.SetDefaultHandler("auto_approve")

	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionApproved {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionApproved)
	}
}

func TestSubmit_HandlerAllowed(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoAllowHandler{}, false)
	_ = m.SetDefaultHandler("auto_allow")

	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionAllowed {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionAllowed)
	}
}

func TestSubmit_HandlerPending_StoresRecord(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(InputTimeoutFailHandler{}, false)
	_ = m.SetDefaultHandler("input_timeout_fail")

	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionPending {
		t.Errorf("Status = %q, want %q", rec.Status, DecisionPending)
	}
	if rec.ID == "" {
		t.Error("pending record should have a non-empty ID")
	}
	// Verify the record is stored.
	got, err := m.GetRecord(rec.ID)
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("GetRecord ID = %q, want %q", got.ID, rec.ID)
	}
}

func TestSubmit_GlobalPolicy(t *testing.T) {
	m := NewPolicyApprovalManager()
	m.SetPolicy(&AccessPolicy{
		DeniedCapabilities: []string{"evil_provider"},
	})

	req := &Request{RequestID: "r1", Capability: "evil_provider"}
	rec, err := m.Submit(req)
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if rec.Status != DecisionDenied {
		t.Errorf("Status = %q, want %q (global policy denied)", rec.Status, DecisionDenied)
	}
}

func TestResolveRecord_Approved(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, _ := m.Submit(req)

	resolved, err := m.ResolveRecord(rec.ID, true, "admin")
	if err != nil {
		t.Fatalf("ResolveRecord error: %v", err)
	}
	if resolved.Status != DecisionApproved {
		t.Errorf("Status = %q, want %q", resolved.Status, DecisionApproved)
	}
	if resolved.Approver != "admin" {
		t.Errorf("Approver = %q, want %q", resolved.Approver, "admin")
	}
}

func TestResolveRecord_Denied(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, _ := m.Submit(req)

	resolved, err := m.ResolveRecord(rec.ID, false, "admin")
	if err != nil {
		t.Fatalf("ResolveRecord error: %v", err)
	}
	if resolved.Status != DecisionDenied {
		t.Errorf("Status = %q, want %q", resolved.Status, DecisionDenied)
	}
}

func TestResolveRecord_NotFound(t *testing.T) {
	m := NewPolicyApprovalManager()
	_, err := m.ResolveRecord("nonexistent", true, "admin")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestResolveRecord_AlreadyResolved(t *testing.T) {
	m := NewPolicyApprovalManager()
	req := &Request{RequestID: "r1", Capability: "docker"}
	rec, _ := m.Submit(req)

	_, err := m.ResolveRecord(rec.ID, true, "admin")
	if err != nil {
		t.Fatalf("first ResolveRecord error: %v", err)
	}
	_, err = m.ResolveRecord(rec.ID, false, "admin")
	if !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved, got %v", err)
	}
}

func TestGetRecord_NotFound(t *testing.T) {
	m := NewPolicyApprovalManager()
	_, err := m.GetRecord("nonexistent")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestListRecords(t *testing.T) {
	m := NewPolicyApprovalManager()
	for i := 0; i < 3; i++ {
		req := &Request{RequestID: "r" + string(rune('1'+i)), Capability: "docker"}
		m.Submit(req)
	}
	records := m.ListRecords()
	if len(records) != 3 {
		t.Errorf("ListRecords returned %d records, want 3", len(records))
	}
}

func TestResolve_UsesGlobalPolicy(t *testing.T) {
	m := NewPolicyApprovalManager()
	_ = m.RegisterHandler(AutoApproveHandler{}, false)
	_ = m.SetDefaultHandler("auto_approve")
	m.SetPolicy(&AccessPolicy{
		DeniedCapabilities: []string{"bash"},
	})

	req := &Request{RequestID: "r1", Capability: "bash"}
	d, err := m.Resolve(context.Background(), req, "")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if d.Approved {
		t.Error("expected denied by global policy, got approved")
	}
	if !d.PolicyOverride {
		t.Error("expected PolicyOverride=true")
	}
}
