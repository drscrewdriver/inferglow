package rbac

import (
	"reflect"
	"sort"
	"testing"
)

func TestNewPermissionMatrix_Empty(t *testing.T) {
	m := NewPermissionMatrix()
	if m == nil {
		t.Fatal("NewPermissionMatrix returned nil")
	}
	if actions := m.RegisteredActions(); len(actions) != 0 {
		t.Errorf("new matrix has registered actions: %v", actions)
	}
}

func TestPermissionMatrix_DefaultDeny_Unregistered(t *testing.T) {
	m := NewPermissionMatrix()
	for _, role := range []Role{RoleViewer, RoleEditor, RoleAdmin, RoleCustom} {
		if m.Allow(role, "nonexistent") {
			t.Errorf("Allow(%q, %q) = true, want false (default deny)", role, "nonexistent")
		}
	}
}

func TestPermissionMatrix_RegisterAndAllow(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("calculator", []Role{RoleViewer, RoleEditor, RoleAdmin})

	if !m.Allow(RoleViewer, "calculator") {
		t.Errorf("Allow(viewer, calculator) = false, want true")
	}
	if !m.Allow(RoleEditor, "calculator") {
		t.Errorf("Allow(editor, calculator) = false, want true")
	}
	if !m.Allow(RoleAdmin, "calculator") {
		t.Errorf("Allow(admin, calculator) = false, want true")
	}
}

func TestPermissionMatrix_Register_DeniesUnregisteredRole(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("file_write", []Role{RoleAdmin})

	if m.Allow(RoleViewer, "file_write") {
		t.Errorf("Allow(viewer, file_write) = true, want false")
	}
	if m.Allow(RoleEditor, "file_write") {
		t.Errorf("Allow(editor, file_write) = true, want false")
	}
	if !m.Allow(RoleAdmin, "file_write") {
		t.Errorf("Allow(admin, file_write) = false, want true")
	}
}

func TestPermissionMatrix_Register_Overwrites(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("action_a", []Role{RoleViewer})
	m.Register("action_a", []Role{RoleAdmin})

	if m.Allow(RoleViewer, "action_a") {
		t.Errorf("Allow(viewer, action_a) = true after re-register, want false")
	}
	if !m.Allow(RoleAdmin, "action_a") {
		t.Errorf("Allow(admin, action_a) = false after re-register, want true")
	}
}

func TestPermissionMatrix_Register_EmptyRolesDeniesAll(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("locked", nil)

	for _, role := range []Role{RoleViewer, RoleEditor, RoleAdmin, RoleCustom} {
		if m.Allow(role, "locked") {
			t.Errorf("Allow(%q, locked) = true, want false", role)
		}
	}
}

func TestPermissionMatrix_RegisterPolicy(t *testing.T) {
	m := NewPermissionMatrix()
	m.RegisterPolicy("url_fetch", ActionPolicy{
		AllowedRoles:    []Role{RoleEditor, RoleAdmin},
		SideEffectLevel: SideEffectNetwork,
	})

	if m.Allow(RoleViewer, "url_fetch") {
		t.Errorf("Allow(viewer, url_fetch) = true, want false")
	}
	if !m.Allow(RoleEditor, "url_fetch") {
		t.Errorf("Allow(editor, url_fetch) = false, want true")
	}
}

func TestPermissionMatrix_ActionsForRole(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("calculator", []Role{RoleViewer, RoleEditor, RoleAdmin})
	m.Register("web_search", []Role{RoleViewer, RoleEditor, RoleAdmin})
	m.Register("file_write", []Role{RoleAdmin})
	m.Register("url_fetch", []Role{RoleEditor, RoleAdmin})

	viewerActions := m.ActionsForRole(RoleViewer)
	wantViewer := []string{"calculator", "web_search"}
	if !reflect.DeepEqual(viewerActions, wantViewer) {
		t.Errorf("ActionsForRole(viewer) = %v, want %v", viewerActions, wantViewer)
	}

	editorActions := m.ActionsForRole(RoleEditor)
	wantEditor := []string{"calculator", "url_fetch", "web_search"}
	if !reflect.DeepEqual(editorActions, wantEditor) {
		t.Errorf("ActionsForRole(editor) = %v, want %v", editorActions, wantEditor)
	}

	adminActions := m.ActionsForRole(RoleAdmin)
	wantAdmin := []string{"calculator", "file_write", "url_fetch", "web_search"}
	if !reflect.DeepEqual(adminActions, wantAdmin) {
		t.Errorf("ActionsForRole(admin) = %v, want %v", adminActions, wantAdmin)
	}
}

func TestPermissionMatrix_ActionsForRole_EmptyForUnregisteredRole(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("calculator", []Role{RoleViewer})

	customActions := m.ActionsForRole(RoleCustom)
	if len(customActions) != 0 {
		t.Errorf("ActionsForRole(custom) = %v, want empty", customActions)
	}
}

func TestPermissionMatrix_RegisteredActions_Sorted(t *testing.T) {
	m := NewPermissionMatrix()
	m.Register("zebra", []Role{RoleAdmin})
	m.Register("alpha", []Role{RoleAdmin})
	m.Register("mango", []Role{RoleAdmin})

	got := m.RegisteredActions()
	want := []string{"alpha", "mango", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RegisteredActions() = %v, want %v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("RegisteredActions() not sorted: %v", got)
	}
}

func TestDefaultPolicies(t *testing.T) {
	wantCount := 8
	if len(DefaultPolicies) != wantCount {
		t.Errorf("DefaultPolicies has %d entries, want %d", len(DefaultPolicies), wantCount)
	}

	expectedNames := []string{
		"calculator", "web_search", "url_fetch", "file_read",
		"file_write", "code_executor", "bash_executor", "json_processor",
	}
	for _, name := range expectedNames {
		if _, ok := DefaultPolicies[name]; !ok {
			t.Errorf("DefaultPolicies missing %q", name)
		}
	}
}

func TestNewDefaultMatrix(t *testing.T) {
	m := NewDefaultMatrix()
	actions := m.RegisteredActions()
	if len(actions) != 8 {
		t.Errorf("NewDefaultMatrix registered %d actions, want 8", len(actions))
	}

	// viewer: calculator, web_search, json_processor
	if !m.Allow(RoleViewer, "calculator") {
		t.Errorf("viewer should access calculator")
	}
	if !m.Allow(RoleViewer, "web_search") {
		t.Errorf("viewer should access web_search")
	}
	if !m.Allow(RoleViewer, "json_processor") {
		t.Errorf("viewer should access json_processor")
	}
	if m.Allow(RoleViewer, "file_write") {
		t.Errorf("viewer should NOT access file_write")
	}
	if m.Allow(RoleViewer, "bash_executor") {
		t.Errorf("viewer should NOT access bash_executor")
	}

	// editor: viewer + url_fetch, file_read
	if !m.Allow(RoleEditor, "url_fetch") {
		t.Errorf("editor should access url_fetch")
	}
	if !m.Allow(RoleEditor, "file_read") {
		t.Errorf("editor should access file_read")
	}
	if m.Allow(RoleEditor, "file_write") {
		t.Errorf("editor should NOT access file_write")
	}

	// admin: everything
	allActions := []string{
		"calculator", "web_search", "url_fetch", "file_read",
		"file_write", "code_executor", "bash_executor", "json_processor",
	}
	for _, name := range allActions {
		if !m.Allow(RoleAdmin, name) {
			t.Errorf("admin should access %s", name)
		}
	}
}

func TestNewDefaultMatrix_Overridable(t *testing.T) {
	m := NewDefaultMatrix()
	// Override file_write to also allow editor.
	m.Register("file_write", []Role{RoleEditor, RoleAdmin})

	if !m.Allow(RoleEditor, "file_write") {
		t.Errorf("after override, editor should access file_write")
	}
	if !m.Allow(RoleAdmin, "file_write") {
		t.Errorf("after override, admin should still access file_write")
	}
}
