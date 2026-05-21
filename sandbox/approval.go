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
	"fmt"
	"sync"
	"time"
)

// ApprovalStatus represents the lifecycle state of an approval request.
type ApprovalStatus string

// Approval lifecycle states for an approval request.
const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// DecisionAction represents the outcome of an approval decision.
type DecisionAction string

// DecisionAction values describing the outcome of an approval decision.
const (
	DecisionApproved DecisionAction = "approved"
	DecisionDenied   DecisionAction = "denied"
	DecisionPending  DecisionAction = "pending"
	DecisionAllowed  DecisionAction = "allowed"
)

// ApprovalPolicy controls how approval requests are handled.
type ApprovalPolicy struct {
	RequiredApprover     string         `json:"required_approver"`
	AutoApprovedModes    []SandboxMode  `json:"auto_approved_modes"`
	BlocklistedProviders []string       `json:"blocklisted_providers"`
	MaxResourceLimit     *ResourceLimit `json:"max_resource_limit"`
}

// ApprovalService manages approval requests and records.
type ApprovalService struct {
	policy  *ApprovalPolicy
	records map[string]*ApprovalRecord
	mu      sync.RWMutex
	seq     int64
}

// NewApprovalService creates a new ApprovalService.
func NewApprovalService(policy *ApprovalPolicy) *ApprovalService {
	return &ApprovalService{
		policy:  policy,
		records: make(map[string]*ApprovalRecord),
	}
}

// ApprovalRequest represents a request for permission to execute in a sandbox.
type ApprovalRequest struct {
	ProviderName string           `json:"provider_name"`
	Policy       *ExecutionPolicy `json:"policy"`
	Mode         SandboxMode      `json:"mode"`
	Requester    string           `json:"requester"`
	Reason       string           `json:"reason"`
}

// ApprovalRecord stores the outcome of an approval request.
type ApprovalRecord struct {
	ID        string           `json:"id"`
	Request   *ApprovalRequest `json:"request"`
	Status    ApprovalStatus   `json:"status"`
	Approver  string           `json:"approver,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// Submit creates a new approval request and checks auto-approve / blocklist rules.
func (s *ApprovalService) Submit(req *ApprovalRequest) (*ApprovalRecord, error) {
	// Check auto-approved modes first
	for _, mode := range s.policy.AutoApprovedModes {
		if mode == req.Mode {
			return s.autoApprove(req), nil
		}
	}
	// Check blocklisted providers
	for _, provider := range s.policy.BlocklistedProviders {
		if provider == req.ProviderName {
			return s.reject(req, "provider is blocklisted"), nil
		}
	}
	// Create pending record with unique ID
	s.mu.Lock()
	s.seq++
	record := &ApprovalRecord{
		ID:        fmt.Sprintf("approval_%d", s.seq),
		Request:   req,
		Status:    ApprovalPending,
		CreatedAt: time.Now(),
	}
	s.records[record.ID] = record
	s.mu.Unlock()
	return record, nil
}

// Resolve updates an approval record with a decision.
func (s *ApprovalService) Resolve(recordID string, decision bool, approver string) (*ApprovalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[recordID]
	if !ok {
		return nil, fmt.Errorf("approval record not found: %s", recordID)
	}
	if record.Status != ApprovalPending {
		return nil, fmt.Errorf("approval already resolved: %s", record.Status)
	}
	if decision {
		record.Status = ApprovalApproved
	} else {
		record.Status = ApprovalRejected
	}
	record.Approver = approver
	record.UpdatedAt = time.Now()
	return record, nil
}

// GetRecord returns an approval record by ID.
func (s *ApprovalService) GetRecord(recordID string) (*ApprovalRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[recordID]
	if !ok {
		return nil, fmt.Errorf("approval record not found: %s", recordID)
	}
	return record, nil
}

// ListRecords returns all approval records.
func (s *ApprovalService) ListRecords() []*ApprovalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ApprovalRecord, 0, len(s.records))
	for _, r := range s.records {
		result = append(result, r)
	}
	return result
}

// Policy returns the approval policy.
func (s *ApprovalService) Policy() *ApprovalPolicy {
	return s.policy
}

func (s *ApprovalService) autoApprove(req *ApprovalRequest) *ApprovalRecord {
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("approval_%d", s.seq)
	s.mu.Unlock()
	return &ApprovalRecord{
		ID:        id,
		Request:   req,
		Status:    ApprovalApproved,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (s *ApprovalService) reject(req *ApprovalRequest, reason string) *ApprovalRecord {
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("approval_%d", s.seq)
	s.mu.Unlock()
	return &ApprovalRecord{
		ID:        id,
		Request:   req,
		Status:    ApprovalRejected,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ApprovalHandler is the interface for processing approval requests.
type ApprovalHandler interface {
	Resolve(ctx context.Context, req *ApprovalRequest) (*ApprovalDecision, error)
}

// ApprovalDecision represents the result of an approval handler decision.
type ApprovalDecision struct {
	Action  DecisionAction `json:"action"`
	Message string         `json:"message"`
}

// InputTimeoutFailHandler waits for user input with a timeout; denies on timeout.
type InputTimeoutFailHandler struct {
	Timeout time.Duration // default 30 seconds
}

// Resolve waits for context completion or timeout.
func (h *InputTimeoutFailHandler) Resolve(ctx context.Context, req *ApprovalRequest) (*ApprovalDecision, error) {
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		if ctx.Err() == nil {
			return &ApprovalDecision{Action: DecisionApproved, Message: "user approved"}, nil
		}
		return &ApprovalDecision{Action: DecisionDenied, Message: "context canceled"}, nil
	case <-timer.C:
		return &ApprovalDecision{Action: DecisionDenied, Message: "timeout exceeded"}, nil
	}
}

// FailClosedHandler keeps the request pending; does not auto-approve.
type FailClosedHandler struct{}

// Resolve returns a pending decision.
func (h *FailClosedHandler) Resolve(ctx context.Context, req *ApprovalRequest) (*ApprovalDecision, error) {
	return &ApprovalDecision{Action: DecisionPending, Message: "waiting for manual approval"}, nil
}

// AutoApproveHandler automatically approves all requests.
type AutoApproveHandler struct{}

// Resolve returns an approved decision.
func (h *AutoApproveHandler) Resolve(ctx context.Context, req *ApprovalRequest) (*ApprovalDecision, error) {
	return &ApprovalDecision{Action: DecisionApproved, Message: "auto-approved"}, nil
}

// AutoAllowHandler completely bypasses the approval flow.
type AutoAllowHandler struct{}

// Resolve returns an allowed decision.
func (h *AutoAllowHandler) Resolve(ctx context.Context, req *ApprovalRequest) (*ApprovalDecision, error) {
	return &ApprovalDecision{Action: DecisionAllowed, Message: "auto-allowed"}, nil
}
