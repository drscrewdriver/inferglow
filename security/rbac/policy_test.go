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
	"testing"
)

func TestRoleConstants(t *testing.T) {
	cases := []struct {
		role Role
		want string
	}{
		{RoleViewer, "viewer"},
		{RoleEditor, "editor"},
		{RoleAdmin, "admin"},
		{RoleCustom, "custom"},
	}
	for _, c := range cases {
		if string(c.role) != c.want {
			t.Errorf("role %q = %q, want %q", c.role, string(c.role), c.want)
		}
	}
}

func TestSideEffectLevelConstants(t *testing.T) {
	cases := []struct {
		level SideEffectLevel
		want  string
	}{
		{SideEffectNone, "none"},
		{SideEffectRead, "read"},
		{SideEffectWrite, "write"},
		{SideEffectNetwork, "network"},
		{SideEffectExec, "exec"},
	}
	for _, c := range cases {
		if string(c.level) != c.want {
			t.Errorf("level %q = %q, want %q", c.level, string(c.level), c.want)
		}
	}
}

func TestImplies(t *testing.T) {
	cases := []struct {
		high, low Role
		want      bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleEditor, true},
		{RoleAdmin, RoleViewer, true},
		{RoleEditor, RoleEditor, true},
		{RoleEditor, RoleViewer, true},
		{RoleEditor, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleEditor, false},
		{RoleViewer, RoleAdmin, false},
		{RoleAdmin, RoleCustom, false},
		{RoleCustom, RoleViewer, false},
		{RoleCustom, RoleCustom, true},
	}
	for _, c := range cases {
		got := Implies(c.high, c.low)
		if got != c.want {
			t.Errorf("Implies(%q, %q) = %v, want %v", c.high, c.low, got, c.want)
		}
	}
}

func TestActionPolicy(t *testing.T) {
	p := ActionPolicy{
		AllowedRoles:    []Role{RoleViewer, RoleEditor},
		SideEffectLevel: SideEffectRead,
	}
	if len(p.AllowedRoles) != 2 {
		t.Errorf("AllowedRoles len = %d, want 2", len(p.AllowedRoles))
	}
	if p.SideEffectLevel != SideEffectRead {
		t.Errorf("SideEffectLevel = %q, want %q", p.SideEffectLevel, SideEffectRead)
	}
}
