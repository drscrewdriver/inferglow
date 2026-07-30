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

	"github.com/inferglow/action"
)

// handleListMCPTools handles GET /v1/mcp-hub — list installed MCP tools.
func (s *Server) handleListMCPTools(w http.ResponseWriter, r *http.Request) {
	if s.mcpHubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp hub store not configured")
		return
	}
	if !s.canAccess(r, "mcp-tool", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	writeJSON(w, http.StatusOK, s.mcpHubStore.List())
}

// handleGetMCPTool handles GET /v1/mcp-hub/{name} — return a single tool.
func (s *Server) handleGetMCPTool(w http.ResponseWriter, r *http.Request) {
	if s.mcpHubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp hub store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "mcp-tool", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	rec, err := s.mcpHubStore.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleInstallMCPTool handles POST /v1/mcp-hub — install a tool from a
// marketplace-style action definition.
func (s *Server) handleInstallMCPTool(w http.ResponseWriter, r *http.Request) {
	if s.mcpHubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp hub store not configured")
		return
	}
	if !s.canAccess(r, "mcp-tool", "", "install") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Schema      map[string]any `json:"input_schema,omitempty"`
		Executor    json.RawMessage `json:"executor,omitempty"` // reserved for future marketplace payloads
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	// For now the HTTP layer only installs metadata-only tools; executable
	// registration is wired through the store's Install from Go code.
	a := &action.Action{Name: req.Name, Description: req.Description, Schema: req.Schema}
	if err := s.mcpHubStore.Install(a); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rec, err := s.mcpHubStore.Get(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

// handleDeleteMCPTool handles DELETE /v1/mcp-hub/{name} — remove a tool.
func (s *Server) handleDeleteMCPTool(w http.ResponseWriter, r *http.Request) {
	if s.mcpHubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp hub store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "mcp-tool", name, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.mcpHubStore.Remove(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// handleCallMCPTool handles POST /v1/mcp-hub/{name}/call — invoke a tool.
func (s *Server) handleCallMCPTool(w http.ResponseWriter, r *http.Request) {
	if s.mcpHubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp hub store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "mcp-tool", name, "call") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Arguments map[string]any `json:"arguments,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	result, err := s.mcpHubStore.Call(r.Context(), name, req.Arguments)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}