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
	"testing"
)

func TestNewRBACMiddleware_DefaultRoleExtractor(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())

	// No role in context → defaults to RoleViewer.
	err := m.Check(context.Background(), "calculator")
	if err != nil {
		t.Errorf("Check(calculator) with default viewer = %v, want nil", err)
	}

	err = m.Check(context.Background(), "file_write")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Check(file_write) with default viewer = %v, want ErrPermissionDenied", err)
	}
}

func TestRBACMiddleware_Check_Allowed(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())
	ctx := WithRole(context.Background(), RoleAdmin)

	for _, action := range []string{
		"calculator", "web_search", "url_fetch", "file_read",
		"file_write", "code_executor", "bash_executor", "json_processor",
	} {
		if err := m.Check(ctx, action); err != nil {
			t.Errorf("Check(admin, %s) = %v, want nil", action, err)
		}
	}
}

func TestRBACMiddleware_Check_Denied(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())

	viewerCtx := WithRole(context.Background(), RoleViewer)
	deniedForViewer := []string{
		"url_fetch", "file_read", "file_write", "code_executor", "bash_executor",
	}
	for _, action := range deniedForViewer {
		err := m.Check(viewerCtx, action)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("Check(viewer, %s) = %v, want ErrPermissionDenied", action, err)
		}
	}

	editorCtx := WithRole(context.Background(), RoleEditor)
	deniedForEditor := []string{"file_write", "code_executor", "bash_executor"}
	for _, action := range deniedForEditor {
		err := m.Check(editorCtx, action)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("Check(editor, %s) = %v, want ErrPermissionDenied", action, err)
		}
	}
}

func TestRBACMiddleware_Check_UnregisteredAction(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())
	ctx := WithRole(context.Background(), RoleAdmin)

	err := m.Check(ctx, "nonexistent_action")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Check(admin, nonexistent) = %v, want ErrPermissionDenied", err)
	}
}

func TestRBACMiddleware_Check_ViewerAllowed(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())
	ctx := WithRole(context.Background(), RoleViewer)

	for _, action := range []string{"calculator", "web_search", "json_processor"} {
		if err := m.Check(ctx, action); err != nil {
			t.Errorf("Check(viewer, %s) = %v, want nil", action, err)
		}
	}
}

func TestRBACMiddleware_WithRoleExtractor(t *testing.T) {
	m := NewRBACMiddleware(
		NewDefaultMatrix(),
		WithRoleExtractor(func(ctx context.Context) Role {
			return RoleAdmin
		}),
	)

	// Even without a role in context, the custom extractor returns admin.
	err := m.Check(context.Background(), "file_write")
	if err != nil {
		t.Errorf("Check(file_write) with admin extractor = %v, want nil", err)
	}
}

func TestRBACMiddleware_WithRoleExtractor_NilIgnored(t *testing.T) {
	m := NewRBACMiddleware(
		NewDefaultMatrix(),
		WithRoleExtractor(nil),
	)

	// nil extractor is ignored; falls back to default (viewer).
	err := m.Check(context.Background(), "file_write")
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Check(file_write) with nil extractor = %v, want ErrPermissionDenied", err)
	}
}

func TestRBACMiddleware_Wrap_Allowed(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())
	ctx := WithRole(context.Background(), RoleViewer)

	called := false
	wrapped := m.Wrap(func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		called = true
		return "ok", nil
	})

	result, err := wrapped(ctx, "calculator", nil)
	if err != nil {
		t.Fatalf("Wrap(calculator) err = %v, want nil", err)
	}
	if !called {
		t.Errorf("inner function not called")
	}
	if result != "ok" {
		t.Errorf("result = %v, want %q", result, "ok")
	}
}

func TestRBACMiddleware_Wrap_Denied(t *testing.T) {
	m := NewRBACMiddleware(NewDefaultMatrix())
	ctx := WithRole(context.Background(), RoleViewer)

	called := false
	wrapped := m.Wrap(func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		called = true
		return "ok", nil
	})

	result, err := wrapped(ctx, "file_write", nil)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Wrap(file_write) err = %v, want ErrPermissionDenied", err)
	}
	if called {
		t.Errorf("inner function should not be called on denial")
	}
	if result != nil {
		t.Errorf("result = %v, want nil on denial", result)
	}
}

// --- Approval integration tests ---

// mockApprover is a test double for the Approver interface.
type mockApprover struct {
	decision ApprovalDecision
	err      error
	called   bool
	lastReq  ApprovalRequest
}

func (m *mockApprover) RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error) {
	m.called = true
	m.lastReq = req
	return m.decision, m.err
}

func TestRBACApprovalAdapter_RbacDenies_ApproverNotCalled(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: true}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	ctx := WithRole(context.Background(), RoleViewer)
	err := adapter.Authorize(ctx, "file_write", ApprovalRequest{ActionName: "file_write"})

	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Authorize err = %v, want ErrPermissionDenied", err)
	}
	if approver.called {
		t.Errorf("approver should not be called when RBAC denies")
	}
}

func TestRBACApprovalAdapter_RbacAllows_ApproverApproves(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: true, Message: "ok"}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	ctx := WithRole(context.Background(), RoleAdmin)
	err := adapter.Authorize(ctx, "file_write", ApprovalRequest{ActionName: "file_write", Requester: "alice"})

	if err != nil {
		t.Errorf("Authorize err = %v, want nil", err)
	}
	if !approver.called {
		t.Errorf("approver should be called when RBAC allows")
	}
	if approver.lastReq.Requester != "alice" {
		t.Errorf("approver received requester = %q, want %q", approver.lastReq.Requester, "alice")
	}
}

func TestRBACApprovalAdapter_RbacAllows_ApproverDenies(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: false, Message: "policy violation"}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	ctx := WithRole(context.Background(), RoleAdmin)
	err := adapter.Authorize(ctx, "file_write", ApprovalRequest{ActionName: "file_write"})

	if !errors.Is(err, ErrApprovalDenied) {
		t.Errorf("Authorize err = %v, want ErrApprovalDenied", err)
	}
}

func TestRBACApprovalAdapter_RbacAllows_ApproverErrors(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approverErr := errors.New("approver unavailable")
	approver := &mockApprover{err: approverErr}
	adapter := NewRBACApprovalAdapter(mw, approver)

	ctx := WithRole(context.Background(), RoleAdmin)
	err := adapter.Authorize(ctx, "file_write", ApprovalRequest{ActionName: "file_write"})

	if !errors.Is(err, approverErr) {
		t.Errorf("Authorize err = %v, want approverErr", err)
	}
}

func TestRBACApprovalAdapter_NilApprover_OnlyRbac(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	adapter := NewRBACApprovalAdapter(mw, nil)

	ctx := WithRole(context.Background(), RoleAdmin)
	// RBAC allows admin → nil approver skipped → no error.
	err := adapter.Authorize(ctx, "file_write", ApprovalRequest{ActionName: "file_write"})
	if err != nil {
		t.Errorf("Authorize err = %v, want nil (nil approver)", err)
	}

	viewerCtx := WithRole(context.Background(), RoleViewer)
	// RBAC denies viewer → ErrPermissionDenied.
	err = adapter.Authorize(viewerCtx, "file_write", ApprovalRequest{ActionName: "file_write"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Authorize err = %v, want ErrPermissionDenied", err)
	}
}

func TestRBACApprovalAdapter_WrapWithApproval_Allowed(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: true}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	called := false
	wrapped := adapter.WrapWithApproval(func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		called = true
		return "executed", nil
	})

	ctx := WithRole(context.Background(), RoleAdmin)
	result, err := wrapped(ctx, "bash_executor", nil)
	if err != nil {
		t.Fatalf("WrapWithApproval err = %v, want nil", err)
	}
	if !called {
		t.Errorf("inner function not called")
	}
	if !approver.called {
		t.Errorf("approver not called")
	}
	if result != "executed" {
		t.Errorf("result = %v, want %q", result, "executed")
	}
}

func TestRBACApprovalAdapter_WrapWithApproval_RbacDenied(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: true}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	called := false
	wrapped := adapter.WrapWithApproval(func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		called = true
		return nil, nil
	})

	ctx := WithRole(context.Background(), RoleViewer)
	_, err := wrapped(ctx, "bash_executor", nil)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("WrapWithApproval err = %v, want ErrPermissionDenied", err)
	}
	if called {
		t.Errorf("inner function should not be called")
	}
	if approver.called {
		t.Errorf("approver should not be called when RBAC denies")
	}
}

func TestRBACApprovalAdapter_WrapWithApproval_ApproverDenied(t *testing.T) {
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: false, Message: "denied"}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	called := false
	wrapped := adapter.WrapWithApproval(func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		called = true
		return nil, nil
	})

	ctx := WithRole(context.Background(), RoleAdmin)
	_, err := wrapped(ctx, "bash_executor", nil)
	if !errors.Is(err, ErrApprovalDenied) {
		t.Errorf("WrapWithApproval err = %v, want ErrApprovalDenied", err)
	}
	if called {
		t.Errorf("inner function should not be called when approver denies")
	}
}

func TestRBACApprovalAdapter_TwoLayer_Order(t *testing.T) {
	// Verify RBAC runs before approval: viewer denied at RBAC layer
	// even though approver would approve.
	mw := NewRBACMiddleware(NewDefaultMatrix())
	approver := &mockApprover{decision: ApprovalDecision{Approved: true}}
	adapter := NewRBACApprovalAdapter(mw, approver)

	ctx := WithRole(context.Background(), RoleViewer)
	err := adapter.Authorize(ctx, "code_executor", ApprovalRequest{ActionName: "code_executor"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Authorize err = %v, want ErrPermissionDenied", err)
	}
	if approver.called {
		t.Errorf("approver must not be called when RBAC denies")
	}

	// Now admin: RBAC passes, approver is consulted.
	adminCtx := WithRole(context.Background(), RoleAdmin)
	err = adapter.Authorize(adminCtx, "code_executor", ApprovalRequest{ActionName: "code_executor"})
	if err != nil {
		t.Errorf("Authorize(admin) err = %v, want nil", err)
	}
	if !approver.called {
		t.Errorf("approver must be called when RBAC allows")
	}
}

func TestErrPermissionDenied(t *testing.T) {
	if !errors.Is(ErrPermissionDenied, ErrPermissionDenied) {
		t.Errorf("ErrPermissionDenied should be itself")
	}
}

func TestErrApprovalDenied(t *testing.T) {
	if !errors.Is(ErrApprovalDenied, ErrApprovalDenied) {
		t.Errorf("ErrApprovalDenied should be itself")
	}
}
