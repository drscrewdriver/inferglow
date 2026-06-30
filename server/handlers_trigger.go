// Copyright 2026 InferGlow Authors

package server

import (
	"encoding/json"
	"net/http"

	"github.com/inferglow/server/trigger"
)

// handleCreateTrigger handles POST /v1/triggers — register a new trigger.
func (s *Server) handleCreateTrigger(w http.ResponseWriter, r *http.Request) {
	var cfg trigger.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if cfg.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if cfg.Flow == "" {
		writeError(w, http.StatusBadRequest, "flow is required")
		return
	}
	if cfg.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}

	t, err := s.triggerReg.Register(cfg)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, triggerResponse(t))
}

// handleListTriggers handles GET /v1/triggers — list all triggers.
func (s *Server) handleListTriggers(w http.ResponseWriter, r *http.Request) {
	triggers := s.triggerReg.List()
	resp := make([]map[string]any, 0, len(triggers))
	for _, t := range triggers {
		resp = append(resp, triggerResponse(t))
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetTrigger handles GET /v1/triggers/{id} — get trigger details.
func (s *Server) handleGetTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.triggerReg.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}
	writeJSON(w, http.StatusOK, triggerResponse(t))
}

// handleDeleteTrigger handles DELETE /v1/triggers/{id} — remove a trigger.
func (s *Server) handleDeleteTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.triggerReg.Unregister(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// handleStartTrigger handles POST /v1/triggers/{id}/start — activate a trigger.
func (s *Server) handleStartTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.triggerReg.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}
	if err := t.Start(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "id": id})
}

// handleStopTrigger handles POST /v1/triggers/{id}/stop — deactivate a trigger.
func (s *Server) handleStopTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.triggerReg.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}
	if err := t.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": id})
}

// handleWebhook handles POST /v1/webhooks/{id} — webhook HTTP entry point.
// It looks up the webhook trigger and delegates to its ServeHTTP method.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.triggerReg.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "webhook trigger not found")
		return
	}

	wt, ok := t.(*trigger.WebhookTrigger)
	if !ok {
		writeError(w, http.StatusBadRequest, "trigger is not a webhook")
		return
	}

	wt.ServeHTTP(w, r)
}

// triggerResponse builds a JSON-safe map for a trigger.
func triggerResponse(t trigger.Trigger) map[string]any {
	return map[string]any{
		"id":      t.ID(),
		"type":    t.Type(),
		"flow":    t.FlowName(),
		"enabled": t.Enabled(),
	}
}
