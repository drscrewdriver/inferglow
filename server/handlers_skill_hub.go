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

// toSkillInfo projects an action.Action into its JSON-safe SkillRecord view.
// The Go Executor is never serialized; it is only surfaced as a boolean flag.
func toSkillInfo(a *action.Action) SkillRecord {
	return SkillRecord{
		Name:        a.Name,
		Description: a.Description,
		Tags:        a.Tags,
		Executable:  a.Executor != nil,
	}
}

// handleListSkills handles GET /v1/skill-hub — list installed skills.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeError(w, http.StatusServiceUnavailable, "skill store not configured")
		return
	}
	if !s.canAccess(r, "skill", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	out := make([]SkillRecord, 0, len(s.skillStore.List()))
	for _, name := range s.skillStore.List() {
		if a, err := s.skillStore.Get(name); err == nil {
			out = append(out, toSkillInfo(a))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetSkill handles GET /v1/skill-hub/{name} — return a single skill.
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeError(w, http.StatusServiceUnavailable, "skill store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "skill", name, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	a, err := s.skillStore.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	writeJSON(w, http.StatusOK, toSkillInfo(a))
}

// handleDeleteSkill handles DELETE /v1/skill-hub/{name} — remove a skill.
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeError(w, http.StatusServiceUnavailable, "skill store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "skill", name, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.skillStore.Remove(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": name})
}

// handleExecuteSkill handles POST /v1/skill-hub/{name}/execute — run a skill
// with a JSON input. The response is the structured action.ActionResult.
func (s *Server) handleExecuteSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeError(w, http.StatusServiceUnavailable, "skill store not configured")
		return
	}
	name := r.PathValue("name")
	if !s.canAccess(r, "skill", name, "execute") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Input map[string]any `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	result, err := s.skillStore.Execute(r.Context(), name, req.Input)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}