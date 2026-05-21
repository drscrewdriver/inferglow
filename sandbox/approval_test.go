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

package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestNewApprovalService(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	if s == nil {
		t.Fatal("NewApprovalService returned nil")
	}
	if s.policy != policy {
		t.Error("policy not stored")
	}
}

func TestApprovalServicePolicy(t *testing.T) {
	policy := &ApprovalPolicy{RequiredApprover: "admin"}
	s := NewApprovalService(policy)
	if s.Policy().RequiredApprover != "admin" {
		t.Errorf("RequiredApprover = %q, want %q", s.Policy().RequiredApprover, "admin")
	}
}

func TestApprovalServiceSubmit_AutoApproved(t *testing.T) {
	policy := &ApprovalPolicy{
		AutoApprovedModes: []SandboxMode{ModeTrustedLocal},
	}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{
		ProviderName: "trusted_local",
		Mode:         ModeTrustedLocal,
	}
	record, err := s.Submit(req)
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if record.Status != ApprovalApproved {
		t.Errorf("Status = %q, want %q", record.Status, ApprovalApproved)
	}
}

func TestApprovalServiceSubmit_Blocklisted(t *testing.T) {
	policy := &ApprovalPolicy{
		BlocklistedProviders: []string{"evil_provider"},
	}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{
		ProviderName: "evil_provider",
		Mode:         ModeDocker,
	}
	record, err := s.Submit(req)
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if record.Status != ApprovalRejected {
		t.Errorf("Status = %q, want %q", record.Status, ApprovalRejected)
	}
}

func TestApprovalServiceSubmit_Pending(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{
		ProviderName: "docker",
		Mode:         ModeDocker,
	}
	record, err := s.Submit(req)
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if record.Status != ApprovalPending {
		t.Errorf("Status = %q, want %q", record.Status, ApprovalPending)
	}
	if record.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestApprovalServiceResolve_Approved(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{ProviderName: "docker", Mode: ModeDocker}
	record, _ := s.Submit(req)

	resolved, err := s.Resolve(record.ID, true, "admin")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Status != ApprovalApproved {
		t.Errorf("Status = %q, want %q", resolved.Status, ApprovalApproved)
	}
	if resolved.Approver != "admin" {
		t.Errorf("Approver = %q, want %q", resolved.Approver, "admin")
	}
}

func TestApprovalServiceResolve_Rejected(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{ProviderName: "docker", Mode: ModeDocker}
	record, _ := s.Submit(req)

	resolved, err := s.Resolve(record.ID, false, "admin")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Status != ApprovalRejected {
		t.Errorf("Status = %q, want %q", resolved.Status, ApprovalRejected)
	}
}

func TestApprovalServiceResolve_NotFound(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	_, err := s.Resolve("nonexistent", true, "admin")
	if err == nil {
		t.Fatal("expected error for not found record")
	}
}

func TestApprovalServiceResolve_AlreadyResolved(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{ProviderName: "docker", Mode: ModeDocker}
	record, _ := s.Submit(req)

	_, err := s.Resolve(record.ID, true, "admin")
	if err != nil {
		t.Fatalf("first Resolve returned error: %v", err)
	}
	_, err = s.Resolve(record.ID, false, "admin")
	if err == nil {
		t.Fatal("expected error for already resolved record")
	}
}

func TestApprovalServiceGetRecord(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{ProviderName: "docker", Mode: ModeDocker}
	record, _ := s.Submit(req)

	got, err := s.GetRecord(record.ID)
	if err != nil {
		t.Fatalf("GetRecord returned error: %v", err)
	}
	if got.ID != record.ID {
		t.Errorf("ID = %q, want %q", got.ID, record.ID)
	}
}

func TestApprovalServiceListRecords(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	for i := 0; i < 3; i++ {
		req := &ApprovalRequest{ProviderName: "docker", Mode: SandboxMode("mode_" + string(rune('a'+i))), Reason: "test"}
		s.Submit(req)
	}
	records := s.ListRecords()
	if len(records) != 3 {
		t.Errorf("ListRecords returned %d records, want 3", len(records))
	}
}

func TestInputTimeoutFailHandler_TimesOut(t *testing.T) {
	handler := &InputTimeoutFailHandler{Timeout: 100 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := handler.Resolve(ctx, &ApprovalRequest{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if decision.Action != DecisionDenied {
		t.Errorf("Action = %q, want %q", decision.Action, DecisionDenied)
	}
}

func TestInputTimeoutFailHandler_ContextDone(t *testing.T) {
	handler := &InputTimeoutFailHandler{Timeout: 30 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	decision, err := handler.Resolve(ctx, &ApprovalRequest{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if decision.Action != DecisionDenied {
		t.Errorf("Action = %q, want %q", decision.Action, DecisionDenied)
	}
}

func TestFailClosedHandler(t *testing.T) {
	handler := &FailClosedHandler{}
	decision, err := handler.Resolve(context.Background(), &ApprovalRequest{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if decision.Action != DecisionPending {
		t.Errorf("Action = %q, want %q", decision.Action, DecisionPending)
	}
}

func TestAutoApproveHandler(t *testing.T) {
	handler := &AutoApproveHandler{}
	decision, err := handler.Resolve(context.Background(), &ApprovalRequest{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if decision.Action != DecisionApproved {
		t.Errorf("Action = %q, want %q", decision.Action, DecisionApproved)
	}
	if decision.Message != "auto-approved" {
		t.Errorf("Message = %q, want %q", decision.Message, "auto-approved")
	}
}

func TestAutoAllowHandler(t *testing.T) {
	handler := &AutoAllowHandler{}
	decision, err := handler.Resolve(context.Background(), &ApprovalRequest{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if decision.Action != DecisionAllowed {
		t.Errorf("Action = %q, want %q", decision.Action, DecisionAllowed)
	}
	if decision.Message != "auto-allowed" {
		t.Errorf("Message = %q, want %q", decision.Message, "auto-allowed")
	}
}

func TestApprovalHandlerInterface(t *testing.T) {
	var _ ApprovalHandler = (*InputTimeoutFailHandler)(nil)
	var _ ApprovalHandler = (*FailClosedHandler)(nil)
	var _ ApprovalHandler = (*AutoApproveHandler)(nil)
	var _ ApprovalHandler = (*AutoAllowHandler)(nil)
}

func TestApprovalStatusConstants(t *testing.T) {
	cases := []struct {
		got  ApprovalStatus
		want string
	}{
		{ApprovalPending, "pending"},
		{ApprovalApproved, "approved"},
		{ApprovalRejected, "rejected"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("%s = %q, want %q", c.got, c.got, c.want)
		}
	}
}

func TestDecisionActionConstants(t *testing.T) {
	cases := []struct {
		got  DecisionAction
		want string
	}{
		{DecisionApproved, "approved"},
		{DecisionDenied, "denied"},
		{DecisionPending, "pending"},
		{DecisionAllowed, "allowed"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("%s = %q, want %q", c.got, c.got, c.want)
		}
	}
}

func TestApprovalRecordTimestamps(t *testing.T) {
	policy := &ApprovalPolicy{}
	s := NewApprovalService(policy)
	req := &ApprovalRequest{ProviderName: "docker", Mode: ModeDocker}
	record, _ := s.Submit(req)

	if record.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestApprovalPolicyStruct(t *testing.T) {
	rl := &ResourceLimit{CPUShares: 2, MemoryBytes: 1024 * 1024}
	policy := &ApprovalPolicy{
		RequiredApprover:     "admin",
		AutoApprovedModes:    []SandboxMode{ModeTrustedLocal},
		BlocklistedProviders: []string{"bad"},
		MaxResourceLimit:     rl,
	}
	if policy.RequiredApprover != "admin" {
		t.Errorf("RequiredApprover = %q, want %q", policy.RequiredApprover, "admin")
	}
	if len(policy.AutoApprovedModes) != 1 {
		t.Error("AutoApprovedModes should have 1 element")
	}
	if policy.MaxResourceLimit.CPUShares != 2 {
		t.Errorf("MaxResourceLimit.CPUShares = %d, want 2", policy.MaxResourceLimit.CPUShares)
	}
}
