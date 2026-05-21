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

// Package rbac implements role-based access control for InferGlow
// Actions.
//
// The rbac package is the identity layer: it decides whether a given
// Role is allowed to invoke a given Action. It is intentionally kept
// separate from the sandbox approval flow (the business layer), which
// decides whether a specific invocation is approved given runtime
// context. See approval_integration.go for how the two layers compose.
package rbac

// Role identifies the privilege level of a caller. The four built-in
// roles cover the common cases: viewer (read-only), editor (read +
// write), admin (full access). RoleCustom allows callers to attach an
// arbitrary permission set to a caller that does not fit the built-in
// tiers. The Implies helper captures the implicit hierarchy among the
// built-in roles.
type Role string

const (
	// RoleViewer can invoke read-only Actions only.
	RoleViewer Role = "viewer"
	// RoleEditor can invoke read and write Actions.
	RoleEditor Role = "editor"
	// RoleAdmin has full access to every registered Action.
	RoleAdmin Role = "admin"
	// RoleCustom is a caller-defined role whose permissions are
	// configured entirely through the permission matrix.
	RoleCustom Role = "custom"
)

// SideEffectLevel mirrors action.SideEffectLevel from the
// github.com/inferglow/action module. It is re-declared here so the
// security module stays self-contained (no dependency on action, which
// would transitively pull in sandbox and its docker/gvisor backends).
// The string values are identical to action.SideEffectLevel so callers
// can convert freely: rbac.SideEffectLevel(spec.SideEffectLevel).
type SideEffectLevel string

const (
	// SideEffectNone indicates the Action has no side effects.
	SideEffectNone SideEffectLevel = "none"
	// SideEffectRead indicates the Action only reads state.
	SideEffectRead SideEffectLevel = "read"
	// SideEffectWrite indicates the Action mutates local state.
	SideEffectWrite SideEffectLevel = "write"
	// SideEffectNetwork indicates the Action performs network I/O.
	SideEffectNetwork SideEffectLevel = "network"
	// SideEffectExec indicates the Action spawns subprocesses.
	SideEffectExec SideEffectLevel = "exec"
)

// ActionPolicy describes the permission requirements of a single
// Action. AllowedRoles lists every role that may invoke the Action;
// SideEffectLevel records the Action's side-effect tier (mirrors
// action.ActionSpec.SideEffectLevel) so callers can derive policies
// from ActionSpecs without importing the action package here.
type ActionPolicy struct {
	// AllowedRoles is the set of roles permitted to invoke the
	// Action. A role not in this list is denied.
	AllowedRoles []Role `json:"allowed_roles"`
	// SideEffectLevel is the Action's side-effect tier, used by
	// integration code to map side-effect severity onto approval
	// requirements.
	SideEffectLevel SideEffectLevel `json:"side_effect_level"`
}

// roleRank is a total ordering of the built-in roles used by Implies
// to capture the privilege hierarchy. RoleCustom is intentionally
// absent: a custom role has no implicit hierarchy and must be
// registered explicitly in the permission matrix.
var roleRank = map[Role]int{
	RoleViewer: 0,
	RoleEditor: 1,
	RoleAdmin:  2,
}

// Implies reports whether high carries all permissions of low. Admin
// implies editor and viewer; editor implies viewer; viewer implies only
// itself. RoleCustom never implies and is never implied by a built-in
// role.
func Implies(high, low Role) bool {
	if high == low {
		return true
	}
	hr, ok1 := roleRank[high]
	lr, ok2 := roleRank[low]
	if !ok1 || !ok2 {
		return false
	}
	return hr >= lr
}
