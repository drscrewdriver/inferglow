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
	SideEffectNone    SideEffectLevel = "none"
	SideEffectRead    SideEffectLevel = "read"
	SideEffectWrite   SideEffectLevel = "write"
	SideEffectNetwork SideEffectLevel = "network"
	SideEffectExec    SideEffectLevel = "exec"
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
