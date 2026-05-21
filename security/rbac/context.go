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
