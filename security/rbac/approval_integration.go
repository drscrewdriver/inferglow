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

package rbac

import (
	"context"
	"errors"
	"fmt"
)

// ErrApprovalDenied is returned by RBACApprovalAdapter.Authorize when
// the RBAC identity check passes but the business-layer approver
// rejects the invocation.
var ErrApprovalDenied = errors.New("rbac: approval denied")

// ApprovalRequest is the rbac-local request type passed to an Approver.
// It intentionally mirrors only the fields the identity layer needs to
// forward; callers adapt it to sandbox.ApprovalRequest (or any other
// approval backend) at the integration boundary. Keeping the type
// local avoids importing the sandbox package, which would create a
// circular dependency (sandbox ← action ← … while security must stay
// dependency-free).
type ApprovalRequest struct {
	// ActionName is the name of the Action requesting approval.
	ActionName string
	// Requester is a human-readable label for the caller (e.g. a
	// user id or agent id) passed through to the approver.
	Requester string
	// Reason is an optional free-form justification.
	Reason string
}

// ApprovalDecision is the outcome of an approval request.
type ApprovalDecision struct {
	// Approved is true when the approver permits the invocation.
	Approved bool
	// Message is an optional human-readable explanation.
	Message string
}

// Approver is the business-layer abstraction that the rbac package
// relies on. The concrete implementation is supplied by the caller
// (typically a thin adapter around sandbox.ApprovalService /
// ApprovalHandler), which keeps the security module free of any
// sandbox import.
//
// Relationship between the two layers:
//
//	RBAC (identity layer): "Is this caller's role allowed to request
//	execution of this Action at all?" — answered by PermissionMatrix.
//	A denial here is fast and final: the request never reaches the
//	approval flow.
//
//	Approval (business layer): "Given the runtime context, should this
//	specific invocation be approved?" — answered by the Approver.
//	This gate may consider policies, resource limits, human review,
//	rate limits, etc.
//
// The adapter runs RBAC first (cheap, in-memory) and only consults
// the Approver when RBAC allows the role. This ordering means a
// misconfigured approver can never widen access beyond what the
// matrix grants.
type Approver interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}

// RBACApprovalAdapter composes the RBAC identity check with a
// business-layer Approver. Use Authorize to gate a single Action, or
// WrapWithApproval to obtain an ExecuteFunc that enforces both layers.
type RBACApprovalAdapter struct { //nolint:revive
	rbac     *RBACMiddleware
	approver Approver
}

// NewRBACApprovalAdapter builds an adapter that first checks RBAC via
// mw, then forwards to approver when RBAC allows the role. A nil
// approver disables the business layer (only RBAC is enforced).
func NewRBACApprovalAdapter(mw *RBACMiddleware, approver Approver) *RBACApprovalAdapter {
	return &RBACApprovalAdapter{
		rbac:     mw,
		approver: approver,
	}
}

// Authorize runs the two-layer gate for a single Action invocation.
// It returns:
//   - ErrPermissionDenied when RBAC rejects the caller's role,
//   - ErrApprovalDenied when RBAC allows but the approver rejects,
//   - the approver's error verbatim when the approver itself errors,
//   - nil when both layers permit the invocation.
func (a *RBACApprovalAdapter) Authorize(ctx context.Context, actionName string, req ApprovalRequest) error {
	if err := a.rbac.Check(ctx, actionName); err != nil {
		return err
	}
	if a.approver == nil {
		return nil
	}
	decision, err := a.approver.RequestApproval(ctx, req)
	if err != nil {
		return err
	}
	if !decision.Approved {
		return fmt.Errorf("%w: %s", ErrApprovalDenied, decision.Message)
	}
	return nil
}

// WrapWithApproval returns an ExecuteFunc that enforces both the RBAC
// identity layer and the approval business layer before delegating to
// next. The ApprovalRequest forwarded to the approver is built from
// actionName and the optional requester/reason supplied via
// WithRequester / WithReason options on the adapter (empty by default).
func (a *RBACApprovalAdapter) WrapWithApproval(next ExecuteFunc) ExecuteFunc {
	return func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		req := ApprovalRequest{ActionName: actionName}
		if err := a.Authorize(ctx, actionName, req); err != nil {
			return nil, err
		}
		return next(ctx, actionName, params)
	}
}
