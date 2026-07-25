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

	"github.com/inferglow/server/trigger"
)

// handleCreateSchedule handles POST /v1/schedules — register a new periodic
// schedule. The schedule is stored and reflected into the trigger registry, so
// it can be started/stopped via the cron lifecycle endpoints.
func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	var req struct {
		Name     string `json:"name"`
		Flow     string `json:"flow"`
		Interval int64  `json:"interval_ms"` // milliseconds
		Stateful bool   `json:"stateful"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Interval <= 0 {
		writeError(w, http.StatusBadRequest, "interval_ms must be positive")
		return
	}
	rec := ScheduleRecord{
		Name:     req.Name,
		Flow:     req.Flow,
		Interval: time.Duration(req.Interval) * time.Millisecond,
		Stateful: req.Stateful,
		Enabled:  req.Enabled,
	}
	id, err := s.scheduleStore.Create(rec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := trigger.Config{
		ID:      id,
		Type:    "cron",
		Flow:    rec.Flow,
		Enabled: rec.Enabled,
		Cron:    &trigger.CronConfig{Interval: rec.Interval},
	}
	if _, err := s.triggerReg.Register(cfg); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, scheduleResponse(s.scheduleStore.Get(id)))
}

// handleListSchedules handles GET /v1/schedules — list all schedules.
func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	resp := make([]map[string]any, 0)
	for _, rec := range s.scheduleStore.List() {
		resp = append(resp, scheduleResponse(rec))
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetSchedule handles GET /v1/schedules/{id} — return a single schedule.
func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	id := r.PathValue("id")
	rec := s.scheduleStore.Get(id)
	if rec == nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponse(rec))
}

// handleDeleteSchedule handles DELETE /v1/schedules/{id} — remove a schedule
// and unregister its cron trigger.
func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	id := r.PathValue("id")
	if err := s.scheduleStore.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = s.triggerReg.Unregister(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// ensureScheduleTrigger rebuilds and registers the cron trigger for a schedule
// if it is not currently present (e.g. after the registry was rebuilt). It
// returns the trigger.
func (s *Server) ensureScheduleTrigger(rec *ScheduleRecord) (trigger.Trigger, bool) {
	if t, ok := s.triggerReg.Get(rec.ID); ok {
		return t, true
	}
	cfg := trigger.Config{
		ID:      rec.ID,
		Type:    "cron",
		Flow:    rec.Flow,
		Enabled: rec.Enabled,
		Cron:    &trigger.CronConfig{Interval: rec.Interval},
	}
	t, err := s.triggerReg.Register(cfg)
	if err != nil {
		return nil, false
	}
	return t, true
}

// handleStartSchedule handles POST /v1/schedules/{id}/start — activate the
// schedule's cron trigger. Stateful schedules are rebuilt+registered here so
// they survive a registry teardown; stateless ones, only if still present.
func (s *Server) handleStartSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	id := r.PathValue("id")
	rec := s.scheduleStore.Get(id)
	if rec == nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	t, ok := s.ensureScheduleTrigger(rec)
	if !ok {
		writeError(w, http.StatusInternalServerError, "failed to (re)register schedule trigger")
		return
	}
	if err := t.Start(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rec.Enabled = true
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "id": id})
}

// handleStopSchedule handles POST /v1/schedules/{id}/stop — deactivate the
// schedule's cron trigger.
func (s *Server) handleStopSchedule(w http.ResponseWriter, r *http.Request) {
	if s.scheduleStore == nil {
		writeError(w, http.StatusServiceUnavailable, "schedule store not configured")
		return
	}
	id := r.PathValue("id")
	if s.scheduleStore.Get(id) == nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}
	t, ok := s.triggerReg.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "schedule trigger not registered")
		return
	}
	if err := t.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rem := s.scheduleStore.Get(id); rem != nil {
		rem.Enabled = false
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "id": id})
}

// scheduleResponse builds a JSON-safe map for a schedule, exposing the
// interval in milliseconds for API consumers.
func scheduleResponse(rec *ScheduleRecord) map[string]any {
	return map[string]any{
		"id":          rec.ID,
		"name":        rec.Name,
		"flow":        rec.Flow,
		"interval_ms": rec.Interval.Milliseconds(),
		"stateful":    rec.Stateful,
		"enabled":     rec.Enabled,
		"created_at":  rec.CreatedAt,
	}
}
