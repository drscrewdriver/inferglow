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

// Package approval provides a pluggable policy approval framework for
// the inferglow orchestrator. It is the Go equivalent of Agently's
// PolicyApproval layer: a centralized manager that resolves approval
// requests through configurable handler strategies.
//
// The framework supports multiple approval modes (auto_approve,
// fail_closed, input_timeout_fail) and can be extended with custom
// handlers for interactive approval, time-based approval, and more.
package approval

import "time"

// DecisionStatus represents the outcome of an approval resolution.
type DecisionStatus string

const (
	// DecisionApproved means the request is approved and may proceed.
	DecisionApproved DecisionStatus = "approved"

	// DecisionDenied means the request is denied and must not proceed.
	DecisionDenied DecisionStatus = "denied"

	// DecisionPending means the request requires further input (e.g.
	// human approval) and cannot be resolved immediately.
	DecisionPending DecisionStatus = "pending"
)

// RiskLevel classifies the risk of an approval request.
type RiskLevel string

const (
	RiskNone   RiskLevel = "none"
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Request represents an approval request submitted to the manager.
type Request struct {
	// RequestID is a unique identifier for this request.
	RequestID string `json:"request_id"`

	// Source identifies who initiated the request (e.g. "action",
	// "resource", "task_node").
	Source string `json:"source"`

	// Capability is the specific capability being requested (e.g.
	// "bash_execute", "network_access").
	Capability string `json:"capability"`

	// Subject is the target of the request (e.g. action name, resource
	// type).
	Subject string `json:"subject"`

	// Risk classifies the risk level.
	Risk RiskLevel `json:"risk"`

	// Payload carries request-specific data.
	Payload map[string]any `json:"payload,omitempty"`

	// Policy is the optional access policy to evaluate.
	Policy *AccessPolicy `json:"policy,omitempty"`
}

// Decision is the outcome of an approval resolution.
type Decision struct {
	// Status is the approval outcome.
	Status DecisionStatus `json:"status"`

	// Approved is a convenience flag mirroring Status == DecisionApproved.
	Approved bool `json:"approved"`

	// Reason explains why the decision was made.
	Reason string `json:"reason"`

	// Handler is the name of the handler that produced this decision.
	Handler string `json:"handler"`

	// PolicyOverride is set when the decision was influenced by an
	// access policy override.
	PolicyOverride bool `json:"policy_override,omitempty"`

	// Metadata carries handler-specific key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AccessPolicy defines access control constraints.
type AccessPolicy struct {
	// AllowedCapabilities lists capabilities that are auto-approved.
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`

	// DeniedCapabilities lists capabilities that are always denied.
	DeniedCapabilities []string `json:"denied_capabilities,omitempty"`

	// MaxRiskLevel is the maximum risk level that can be auto-approved.
	// Requests above this level require explicit approval.
	MaxRiskLevel RiskLevel `json:"max_risk_level,omitempty"`

	// RequireApproval forces all requests through the approval handler
	// regardless of other policy settings.
	RequireApproval bool `json:"require_approval,omitempty"`
}

// ApprovalHandler is the interface for pluggable approval strategies.
type ApprovalHandler interface {
	// Name returns the handler's unique name (e.g. "auto_approve",
	// "fail_closed").
	Name() string

	// Resolve evaluates the request and returns a decision.
	Resolve(req *Request) (*Decision, error)
}

// riskOrder maps risk levels to numeric order for comparison.
var riskOrder = map[RiskLevel]int{
	RiskNone:   0,
	RiskLow:    1,
	RiskMedium: 2,
	RiskHigh:   3,
}

// RiskExceeds returns true if a's risk level exceeds b's.
func RiskExceeds(a, b RiskLevel) bool {
	return riskOrder[a] > riskOrder[b]
}

// AutoApproveTimeout is a convenience timeout for input_timeout_fail
// handler. If approval is not received within this duration, the request
// is denied.
const AutoApproveTimeout = 30 * time.Second
