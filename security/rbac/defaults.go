package rbac

// DefaultPolicies maps each built-in Action name to its default
// ActionPolicy. The matrix follows the principle of least privilege:
// read-only Actions are available to every role, write Actions require
// editor or above, and Actions with exec/network side effects require
// admin.
//
// The Action names here match the *ActionID constants declared in the
// builtins/actions package (e.g. CalculatorActionID == "calculator").
// They are duplicated as string literals so this file does not import
// the builtins/actions package (which would pull in the action and
// sandbox modules transitively).
var DefaultPolicies = map[string]ActionPolicy{
	"calculator": {
		AllowedRoles:    []Role{RoleViewer, RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectNone,
	},
	"web_search": {
		AllowedRoles:    []Role{RoleViewer, RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectNetwork,
	},
	"url_fetch": {
		AllowedRoles:    []Role{RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectNetwork,
	},
	"file_read": {
		AllowedRoles:    []Role{RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectRead,
	},
	"file_write": {
		AllowedRoles:    []Role{RoleAdmin},
		SideEffectLevel: SideEffectWrite,
	},
	"code_executor": {
		AllowedRoles:    []Role{RoleAdmin},
		SideEffectLevel: SideEffectExec,
	},
	"bash_executor": {
		AllowedRoles:    []Role{RoleAdmin},
		SideEffectLevel: SideEffectExec,
	},
	"json_processor": {
		AllowedRoles:    []Role{RoleViewer, RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectNone,
	},
}

// NewDefaultMatrix returns a PermissionMatrix pre-populated with every
// entry from DefaultPolicies. Callers can further Register or override
// entries on the returned matrix.
func NewDefaultMatrix() *PermissionMatrix {
	m := NewPermissionMatrix()
	for name, policy := range DefaultPolicies {
		m.RegisterPolicy(name, policy)
	}
	return m
}
