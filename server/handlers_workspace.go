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
	"encoding/json"
	"net/http"
)

// handleCreateWorkspace handles POST /v1/workspaces — open a workspace rooted
// at a given directory (spec C-7).
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	if !s.canAccess(r, "workspace", "", "create") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Name    string `json:"name"`
		RootDir string `json:"root_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" || req.RootDir == "" {
		writeError(w, http.StatusBadRequest, "name and root_dir are required")
		return
	}
	info, err := s.wsProvider.Open(req.Name, req.RootDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

// handleListWorkspaces handles GET /v1/workspaces — list all opened workspaces.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	if !s.canAccess(r, "workspace", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	writeJSON(w, http.StatusOK, s.wsProvider.List())
}

// handleGetWorkspace handles GET /v1/workspaces/{name} — return a single
// workspace's info.
func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	info, ok := s.wsProvider.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleDeleteWorkspace handles DELETE /v1/workspaces/{name} — remove a
// workspace binding.
func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.wsProvider.Close(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed", "name": name})
}

// handleListWorkspaceFiles handles GET /v1/workspaces/{name}/files — list the
// entries underneath an optional path within the named workspace (spec C-7,
// optional file-listing surface, routed via ListDir).
func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	if s.wsProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace provider not configured")
		return
	}
	name := r.PathValue("id")
	if !s.canAccess(r, "workspace", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	_, ok := s.wsProvider.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	rel := r.URL.Query().Get("path")
	files, err := s.wsProvider.ListDir(name, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":  name,
		"path":  rel,
		"files": files,
	})
}
