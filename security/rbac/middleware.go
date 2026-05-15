package rbac

import (
	"context"
	"errors"
)

// ErrPermissionDenied is returned by Check (and propagated by Wrap)
// when the caller's role is not allowed to invoke the requested
// Action.
var ErrPermissionDenied = errors.New("rbac: permission denied")

// ExecuteFunc is the signature of a single-action execution function
// that RBACMiddleware can wrap. The orchestrator's ActionDispatcher
// executes Actions in batches; callers adapt each per-Action call to
// this signature so the middleware can gate it without the rbac
// package importing the orchestrator or action types (which would
// create a circular dependency).
type ExecuteFunc func(ctx context.Context, actionName string, params map[string]any) (any, error)

// MiddlewareOption configures an RBACMiddleware at construction time.
type MiddlewareOption func(*RBACMiddleware)

// WithRoleExtractor overrides the default role extractor (which reads
// the role from the context via RoleFromContext). This is useful when
// roles are carried in a non-standard location (e.g. a JWT claim, a
// gRPC metadata header) that the caller has already decoded.
func WithRoleExtractor(fn func(context.Context) Role) MiddlewareOption {
	return func(m *RBACMiddleware) {
		if fn != nil {
			m.roleExtractor = fn
		}
	}
}

// RBACMiddleware gates Action invocation by consulting a
// PermissionMatrix. It is the integration point between the rbac
// identity layer and the action runtime: callers either call Check
// directly before executing an Action, or use Wrap to decorate an
// ExecuteFunc so the check happens transparently.
type RBACMiddleware struct {
	matrix        *PermissionMatrix
	roleExtractor func(context.Context) Role
}

// NewRBACMiddleware builds a middleware backed by matrix. When no
// options are supplied the middleware reads the caller's role from the
// context via RoleFromContext (defaulting to RoleViewer when absent).
func NewRBACMiddleware(matrix *PermissionMatrix, opts ...MiddlewareOption) *RBACMiddleware {
	m := &RBACMiddleware{
		matrix:        matrix,
		roleExtractor: defaultRoleExtractor,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// defaultRoleExtractor reads the role from the context, falling back to
// RoleViewer when no role was injected.
func defaultRoleExtractor(ctx context.Context) Role {
	role, _ := RoleFromContext(ctx)
	return role
}

// Check reports whether the caller identified by ctx is allowed to
// invoke actionName. It returns ErrPermissionDenied when the role is
// not permitted (including when the Action was never registered).
func (m *RBACMiddleware) Check(ctx context.Context, actionName string) error {
	role := m.roleExtractor(ctx)
	if !m.matrix.Allow(role, actionName) {
		return ErrPermissionDenied
	}
	return nil
}

// Wrap returns an ExecuteFunc that runs the RBAC check before
// delegating to next. When the check fails the wrapped function
// returns nil and ErrPermissionDenied without invoking next.
func (m *RBACMiddleware) Wrap(next ExecuteFunc) ExecuteFunc {
	return func(ctx context.Context, actionName string, params map[string]any) (any, error) {
		if err := m.Check(ctx, actionName); err != nil {
			return nil, err
		}
		return next(ctx, actionName, params)
	}
}
