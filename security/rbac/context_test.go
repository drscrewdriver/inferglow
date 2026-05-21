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
	"testing"
)

func TestWithRole_RoleFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithRole(ctx, RoleAdmin)

	role, ok := RoleFromContext(ctx)
	if !ok {
		t.Errorf("RoleFromContext ok = false, want true")
	}
	if role != RoleAdmin {
		t.Errorf("role = %q, want %q", role, RoleAdmin)
	}
}

func TestRoleFromContext_NoRole(t *testing.T) {
	ctx := context.Background()
	role, ok := RoleFromContext(ctx)
	if ok {
		t.Errorf("RoleFromContext ok = true, want false (no role set)")
	}
	if role != RoleViewer {
		t.Errorf("role = %q, want %q (default to viewer)", role, RoleViewer)
	}
}

func TestRoleFromContext_NilContext(t *testing.T) {
	role, ok := RoleFromContext(nil)
	if ok {
		t.Errorf("RoleFromContext(nil) ok = true, want false")
	}
	if role != RoleViewer {
		t.Errorf("role = %q, want %q", role, RoleViewer)
	}
}

func TestWithRole_NilContext(t *testing.T) {
	ctx := WithRole(nil, RoleEditor)
	role, ok := RoleFromContext(ctx)
	if !ok {
		t.Errorf("RoleFromContext ok = false after WithRole(nil, …), want true")
	}
	if role != RoleEditor {
		t.Errorf("role = %q, want %q", role, RoleEditor)
	}
}

func TestRoleFromContext_EmptyRole(t *testing.T) {
	ctx := context.Background()
	ctx = WithRole(ctx, Role(""))

	role, ok := RoleFromContext(ctx)
	if ok {
		t.Errorf("RoleFromContext ok = true for empty role, want false")
	}
	if role != RoleViewer {
		t.Errorf("role = %q, want %q (default to viewer)", role, RoleViewer)
	}
}

func TestRoleFromContext_AllRoles(t *testing.T) {
	for _, role := range []Role{RoleViewer, RoleEditor, RoleAdmin, RoleCustom} {
		ctx := WithRole(context.Background(), role)
		got, ok := RoleFromContext(ctx)
		if !ok {
			t.Errorf("RoleFromContext ok = false for %q, want true", role)
		}
		if got != role {
			t.Errorf("role = %q, want %q", got, role)
		}
	}
}

func TestWithRole_DoesNotMutateParent(t *testing.T) {
	parent := context.Background()
	child := WithRole(parent, RoleAdmin)

	parentRole, parentOK := RoleFromContext(parent)
	if parentOK {
		t.Errorf("parent context should not have a role, got %q", parentRole)
	}

	childRole, childOK := RoleFromContext(child)
	if !childOK || childRole != RoleAdmin {
		t.Errorf("child context role = %q (ok=%v), want %q", childRole, childOK, RoleAdmin)
	}
}
