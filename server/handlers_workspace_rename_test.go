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

package server

import (
	"strings"
	"testing"
)

// TestRenameWorkspace — PATCH /v1/workspaces/{id} renames the registry
// binding, keeps the root, rewrites bound session records, and refuses
// unknown targets / name collisions / no-op renames (R9).
func TestRenameWorkspace(t *testing.T) {
	dirA := t.TempDir()
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	srv.SeedWorkspaces([]WorkspaceSeed{{Name: "ws-a", Root: dirA}})
	st := NewSessionStore()
	srv.SetSessionStore(st)
	st.Create(SessionRecord{AgentID: "demo", Title: "in ws-a", Workspace: "ws-a"})
	st.Create(SessionRecord{AgentID: "demo", Title: "unbound"})

	// Unknown source → 404.
	if w := doJSON(t, srv, "PATCH", "/v1/workspaces/nope", `{"new_name":"ws-z"}`); w.Code != 404 {
		t.Fatalf("unknown workspace: want 404, got %d (%s)", w.Code, w.Body.String())
	}

	// No-op rename → 400.
	if w := doJSON(t, srv, "PATCH", "/v1/workspaces/ws-a", `{"new_name":"ws-a"}`); w.Code != 400 {
		t.Fatalf("no-op rename: want 400, got %d (%s)", w.Code, w.Body.String())
	}

	// Empty name → 400.
	if w := doJSON(t, srv, "PATCH", "/v1/workspaces/ws-a", `{"new_name":"  "}`); w.Code != 400 {
		t.Fatalf("empty new_name: want 400, got %d (%s)", w.Code, w.Body.String())
	}

	// Successful rename: root preserved, sessions follow.
	if w := doJSON(t, srv, "PATCH", "/v1/workspaces/ws-a", `{"new_name":"ws-renamed"}`); w.Code != 200 {
		t.Fatalf("rename: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	list := doJSON(t, srv, "GET", "/v1/workspaces", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"name":"ws-renamed"`) || strings.Contains(list.Body.String(), `"name":"ws-a"`) {
		t.Fatalf("registry after rename: %d %s", list.Code, list.Body.String())
	}
	// The renamed binding still resolves (no "workspace not found").
	tree := doJSON(t, srv, "GET", "/v1/fs/tree?workspace=ws-renamed", "")
	if tree.Code == 404 || strings.Contains(tree.Body.String(), "workspace not found") {
		t.Fatalf("renamed workspace unreachable: %d %s", tree.Code, tree.Body.String())
	}
	sessions := doJSON(t, srv, "GET", "/v1/sessions", "")
	if !strings.Contains(sessions.Body.String(), `"workspace":"ws-renamed"`) || strings.Contains(sessions.Body.String(), `"workspace":"ws-a"`) {
		t.Fatalf("session records did not follow rename: %s", sessions.Body.String())
	}

	// Collision → 409.
	srv.SeedWorkspaces([]WorkspaceSeed{{Name: "ws-other", Root: t.TempDir()}})
	if w := doJSON(t, srv, "PATCH", "/v1/workspaces/ws-renamed", `{"new_name":"ws-other"}`); w.Code != 409 {
		t.Fatalf("collision: want 409, got %d (%s)", w.Code, w.Body.String())
	}
}
