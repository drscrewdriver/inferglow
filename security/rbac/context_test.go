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
