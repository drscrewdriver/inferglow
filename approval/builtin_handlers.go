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

import "fmt"

// Compile-time interface checks.
var (
	_ ApprovalHandler = (*AutoApproveHandler)(nil)
	_ ApprovalHandler = (*FailClosedHandler)(nil)
	_ ApprovalHandler = (*InputTimeoutFailHandler)(nil)
)

// AutoApproveHandler approves every request unconditionally.
// This is useful for development and testing.
type AutoApproveHandler struct{}

// Name returns "auto_approve".
func (AutoApproveHandler) Name() string { return "auto_approve" }

// Resolve always returns an approved decision.
func (AutoApproveHandler) Resolve(req *Request) (*Decision, error) {
	return &Decision{
		Status:   DecisionApproved,
		Approved: true,
		Reason:   fmt.Sprintf("auto-approved by auto_approve handler (source=%s, capability=%s)", req.Source, req.Capability),
		Handler:  "auto_approve",
	}, nil
}

// FailClosedHandler denies every request unconditionally.
// This is the safest default for production environments.
type FailClosedHandler struct{}

// Name returns "fail_closed".
func (FailClosedHandler) Name() string { return "fail_closed" }

// Resolve always returns a denied decision.
func (FailClosedHandler) Resolve(req *Request) (*Decision, error) {
	return &Decision{
		Status:   DecisionDenied,
		Approved: false,
		Reason:   fmt.Sprintf("denied by fail_closed handler (source=%s, capability=%s)", req.Source, req.Capability),
		Handler:  "fail_closed",
	}, nil
}

// InputTimeoutFailHandler denies requests that are not explicitly
// approved within a timeout window. In this synchronous implementation
// it behaves like fail_closed — the timeout semantics are relevant
// when used with an interactive approval channel.
type InputTimeoutFailHandler struct{}

// Name returns "input_timeout_fail".
func (InputTimeoutFailHandler) Name() string { return "input_timeout_fail" }

// Resolve returns a pending decision indicating that interactive input
// is required but not available in synchronous mode.
func (InputTimeoutFailHandler) Resolve(req *Request) (*Decision, error) {
	return &Decision{
		Status:   DecisionPending,
		Approved: false,
		Reason:   fmt.Sprintf("requires interactive approval (source=%s, capability=%s); no channel available", req.Source, req.Capability),
		Handler:  "input_timeout_fail",
		Metadata: map[string]string{
			"timeout": AutoApproveTimeout.String(),
		},
	}, nil
}
