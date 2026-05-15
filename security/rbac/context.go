package rbac

import "context"

// contextKey is an unexported key type so the rbac context value never
// collides with keys from other packages.
type contextKey struct{}

// roleKey is the concrete key instance used by WithRole / RoleFromContext.
var roleKey = contextKey{}

// WithRole returns a copy of ctx carrying role. Callers downstream can
// retrieve it via RoleFromContext. Passing an empty Role is allowed and
// results in RoleFromContext returning ("", false).
func WithRole(ctx context.Context, role Role) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, roleKey, role)
}

// RoleFromContext extracts the Role previously injected by WithRole.
// When no role is present (the context was never decorated) it returns
// RoleViewer and false, so callers that forget to set a role default to
// the least-privileged tier rather than being denied outright.
func RoleFromContext(ctx context.Context) (Role, bool) {
	if ctx == nil {
		return RoleViewer, false
	}
	v := ctx.Value(roleKey)
	if v == nil {
		return RoleViewer, false
	}
	role, ok := v.(Role)
	if !ok {
		return RoleViewer, false
	}
	if role == "" {
		return RoleViewer, false
	}
	return role, true
}
