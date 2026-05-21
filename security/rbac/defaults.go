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
