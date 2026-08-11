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
	"time"

	"github.com/inferglow/messagebus"
)

// handleCreateSession handles POST /v1/sessions — create a new management
// session bound to an agent.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not configured")
		return
	}
	var req struct {
		Owner   string `json:"owner,omitempty"`
		AgentID string `json:"agent_id" validate:"required"`
		Title   string `json:"title,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	rec := SessionRecord{Owner: req.Owner, AgentID: req.AgentID, Title: req.Title}
	id, err := s.sessionStore.Create(rec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.bus != nil {
		_ = s.bus.Publish(r.Context(), "session", messagebus.Message{
			ID:        id,
			SessionID: id,
			Topic:     "session",
			Kind:      "session.created",
			Payload:   map[string]any{"session_id": id, "agent_id": req.AgentID},
		})
	}
	writeJSON(w, http.StatusCreated, s.sessionStore.Get(id))
}

// handleListSessions handles GET /v1/sessions — list all sessions.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not configured")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionStore.List())
}

// handleGetSession handles GET /v1/sessions/{id} — return a single session.
// When no session store is configured it preserves the original stub response
// for backward compatibility (existing tests depend on it).
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.sessionStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": id,
			"status":     "active",
		})
		return
	}
	rec := s.sessionStore.Get(id)
	if rec == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleDeleteSession handles DELETE /v1/sessions/{id} — remove a session
// and broadcast a termination event so listeners can cascade-clean any
// attached child resources (spec C-4). There is no enforced foreign key, so
// the bus acts as the out-of-band linkage: consumers of the "session"
// topic observe the event and tear down their own records.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not configured")
		return
	}
	id := r.PathValue("id")
	rec := s.sessionStore.Get(id)
	if rec == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if s.bus != nil {
		_ = s.bus.Publish(r.Context(), "session", messagebus.Message{
			ID:        id,
			SessionID: id,
			Topic:     "session",
			Kind:      "session.terminated",
			Payload:   map[string]any{"session_id": id, "status": "terminated"},
		})
	}
	if err := s.sessionStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// handleUpdateSession handles PATCH /v1/sessions/{id} — update session
// metadata (title/group/pinned/status) with pointer semantics: absent fields
// are left untouched, empty values explicitly clear the field.
func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not configured")
		return
	}
	id := r.PathValue("id")
	if s.sessionStore.Get(id) == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var patch SessionPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if !s.sessionStore.UpdateMeta(id, patch) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionStore.Get(id))
}

// handleSessionStream handles GET /v1/sessions/{id}/stream — a long-lived SSE
// stream of the chosen session's bus events (spec C-4).
//
// The stream clears the per-request write deadline via http.ResponseController
// so the server's global WriteTimeout (60s) does not sever the connection.
func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not configured")
		return
	}
	id := r.PathValue("id")
	if s.sessionStore.Get(id) == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "message bus not configured")
		return
	}

	// SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// SSE timeout fix: lift the absolute write deadline so a long-lived
	// stream is not cut off by WriteTimeout after 60s (Go 1.20+).
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ch, err := s.bus.DrainSession(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				writeSSEEvent(w, "done", map[string]string{"session_id": id})
				flusher.Flush()
				return
			}
			writeSSEEvent(w, msg.Kind, msg.Payload)
			flusher.Flush()
		}
	}
}
