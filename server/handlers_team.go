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

// handleCreateTeam creates a new team definition.
// POST /v1/teams
func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeError(w, http.StatusServiceUnavailable, "team coordinator not configured")
		return
	}
	var cfg TeamConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	id, err := s.teamStore.Create(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"id":     id,
		"name":   cfg.Name,
		"status": "created",
	})
}

// handleListTeams returns all team definitions.
// GET /v1/teams
func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"teams": []any{}, "count": 0})
		return
	}
	teams := s.teamStore.List()
	writeJSON(w, http.StatusOK, map[string]any{
		"teams": teams,
		"count": len(teams),
	})
}

// handleGetTeam returns a team definition by ID.
// GET /v1/teams/{id}
func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeError(w, http.StatusServiceUnavailable, "team coordinator not configured")
		return
	}
	id := r.PathValue("id")
	cfg := s.teamStore.Get(id)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// handleDeleteTeam removes a team definition.
// DELETE /v1/teams/{id}
func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeError(w, http.StatusServiceUnavailable, "team coordinator not configured")
		return
	}
	id := r.PathValue("id")
	if err := s.teamStore.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// TeamRunRequest is the request body for team execution.
type TeamRunRequest struct {
	Task string `json:"task"`
}

// handleTeamRun executes a team coordination round synchronously.
// POST /v1/teams/{id}/run
func (s *Server) handleTeamRun(w http.ResponseWriter, r *http.Request) {
	if s.teamRunner == nil || s.teamStore == nil {
		writeError(w, http.StatusServiceUnavailable, "team coordinator not configured")
		return
	}
	id := r.PathValue("id")
	cfg := s.teamStore.Get(id)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}

	var req TeamRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Task == "" {
		writeError(w, http.StatusBadRequest, "task is required")
		return
	}

	coord, err := s.teamRunner.BuildCoordinator(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := coord.Round(r.Context(), req.Task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "team execution error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"team_id":        id,
		"final_response": result.FinalResponse,
		"member_outputs": result.MemberOutputs,
		"rounds":         result.Rounds,
	})
}

// handleTeamStream executes a team coordination round with SSE streaming.
// POST /v1/teams/{id}/stream
func (s *Server) handleTeamStream(w http.ResponseWriter, r *http.Request) {
	if s.teamRunner == nil || s.teamStore == nil {
		writeError(w, http.StatusServiceUnavailable, "team coordinator not configured")
		return
	}
	id := r.PathValue("id")
	cfg := s.teamStore.Get(id)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}

	var req TeamRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	coord, err := s.teamRunner.BuildCoordinator(cfg)
	if err != nil {
		writeSSEEvent(w, "error", map[string]string{"error": err.Error()})
		flusher.Flush()
		return
	}

	// Execute the coordination round.
	result, err := coord.Round(r.Context(), req.Task)
	if err != nil {
		writeSSEEvent(w, "error", map[string]string{"error": err.Error()})
		flusher.Flush()
		return
	}

	// Emit a member_done event for each member's output.
	for role, output := range result.MemberOutputs {
		writeSSEEvent(w, "member_done", map[string]string{
			"role":     role,
			"response": output,
		})
		flusher.Flush()
	}

	// Emit the final done event.
	writeSSEEvent(w, "done", map[string]any{
		"team_id":        id,
		"final_response": result.FinalResponse,
		"rounds":         result.Rounds,
	})
	flusher.Flush()
}
