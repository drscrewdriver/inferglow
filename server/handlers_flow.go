// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/inferglow/flow/flowdef"
	"gopkg.in/yaml.v3"
)

// --- Run handlers ---

// handleCreateRun handles POST /v1/runs — submit a flow execution.
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Flow   string         `json:"flow" validate:"required"`
		Inputs map[string]any `json:"inputs"`
		Owner  string         `json:"owner"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	handle, err := s.runMgr.Start(req.Flow, req.Inputs, req.Owner)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, runResponse(handle))
}

// handleListRuns handles GET /v1/runs — list all runs.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	statusFilter := RunStatus(r.URL.Query().Get("status"))
	runs := s.runMgr.List(statusFilter)
	resp := make([]map[string]any, 0, len(runs))
	for _, h := range runs {
		resp = append(resp, runResponse(h))
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetRun handles GET /v1/runs/{id} — get run status.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	handle, err := s.runMgr.Status(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runResponse(handle))
}

// handleCancelRun handles DELETE /v1/runs/{id} — cancel a run.
func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.runMgr.Cancel(id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled", "run_id": id})
}

// handlePauseRun handles POST /v1/runs/{id}/pause — pause a run.
func (s *Server) handlePauseRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.runMgr.Pause(id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "run_id": id})
}

// handleResumeRun handles POST /v1/runs/{id}/resume — resume a paused run.
func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ResumeInput map[string]any `json:"resume_input"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.runMgr.Resume(id, req.ResumeInput); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "run_id": id})
}

// handleRunEvents handles GET /v1/runs/{id}/events — SSE event stream.
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	eventCh, cleanup, err := s.runMgr.Subscribe(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer cleanup()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventCh:
			if !ok {
				fmt.Fprintf(w, "event: done\ndata: {\"run_id\":%q}\n\n", id)
				flusher.Flush()
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}

// --- Flow handlers ---

// handleListFlows handles GET /v1/flows — list all registered flows.
func (s *Server) handleListFlows(w http.ResponseWriter, r *http.Request) {
	names := s.flowStore.List()
	writeJSON(w, http.StatusOK, names)
}

// handleGetFlow handles GET /v1/flows/{name} — get flow definition.
func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	def, ok := s.flowStore.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("flow %q not found", name))
		return
	}
	writeJSON(w, http.StatusOK, def)
}

// handleRegisterFlow handles POST /v1/flows — register a new flow (hot-load).
func (s *Server) handleRegisterFlow(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	def := &flowdef.FlowDef{}
	if err := yaml.Unmarshal(body, def); err != nil {
		writeError(w, http.StatusBadRequest, "parse YAML: "+err.Error())
		return
	}

	// Auto-detect simple format.
	var probe map[string]any
	_ = yaml.Unmarshal(body, &probe)
	if _, hasStages := probe["stages"]; hasStages {
		if _, hasSpec := probe["spec"]; !hasSpec {
			sw := &flowdef.SimpleWorkflow{}
			if err := yaml.Unmarshal(body, sw); err == nil {
				if converted, cerr := flowdef.ConvertSimpleToFlowDef(sw); cerr == nil {
					def = converted
				} else {
					writeError(w, http.StatusUnprocessableEntity, cerr.Error())
					return
				}
			}
		}
	}

	if err := s.flowStore.Register(def); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":   def.Metadata.Name,
		"status": "registered",
	})
}

// handleValidateFlow handles POST /v1/flows/{name}/validate — validate without loading.
func (s *Server) handleValidateFlow(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	def := &flowdef.FlowDef{}
	if err := yaml.Unmarshal(body, def); err != nil {
		writeError(w, http.StatusBadRequest, "parse YAML: "+err.Error())
		return
	}

	if err := flowdef.Validate(def); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":  def.Metadata.Name,
		"valid": true,
		"steps": len(def.Spec.Steps),
	})
}

// handleListStages handles GET /v1/stages — list available stages.
func (s *Server) handleListStages(w http.ResponseWriter, r *http.Request) {
	names := s.flowStore.Stages().List()
	writeJSON(w, http.StatusOK, names)
}

// handleGetRunState handles GET /v1/runs/{id}/state — return execution state snapshot.
func (s *Server) handleGetRunState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	handle, err := s.runMgr.Status(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	handle.mu.Lock()
	state := handle.ExecState
	handle.mu.Unlock()

	if state == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"run_id":  id,
			"status":  string(handle.Status),
			"message": "execution not yet started or still in progress",
		})
		return
	}

	// Build a JSON-safe representation of the execution state.
	stepLog := make(map[string]any, len(state.StepLog))
	for name, entry := range state.StepLog {
		e := map[string]any{
			"step":     entry.StepName,
			"duration": entry.Duration.String(),
		}
		if entry.Error != nil {
			e["error"] = entry.Error.Error()
		}
		stepLog[name] = e
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":        id,
		"status":        string(state.Status),
		"step_log":      stepLog,
		"step_exec_log": state.StepExecLog,
		"error_count":   len(state.Errors),
	})
}

// handleGetRunSteps handles GET /v1/runs/{id}/steps — return step execution log.
func (s *Server) handleGetRunSteps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	handle, err := s.runMgr.Status(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	handle.mu.Lock()
	state := handle.ExecState
	handle.mu.Unlock()

	if state == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"run_id": id,
			"steps":  []any{},
		})
		return
	}

	steps := make([]map[string]any, 0, len(state.StepExecLog))
	for _, name := range state.StepExecLog {
		entry, ok := state.StepLog[name]
		if !ok {
			continue
		}
		s := map[string]any{
			"step":     entry.StepName,
			"duration": entry.Duration.String(),
		}
		if entry.Error != nil {
			s["error"] = entry.Error.Error()
		}
		steps = append(steps, s)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": id,
		"steps":  steps,
	})
}

// --- helpers ---

func runResponse(h *RunHandle) map[string]any {
	resp := map[string]any{
		"run_id":     h.ID,
		"flow":       h.FlowName,
		"status":     string(h.Status),
		"started_at": h.StartedAt.Format(time.RFC3339),
	}
	if h.Inputs != nil {
		resp["inputs"] = h.Inputs
	}
	if h.Output != nil {
		resp["output"] = h.Output
	}
	if h.Error != "" {
		resp["error"] = h.Error
	}
	if h.Owner != "" {
		resp["owner"] = h.Owner
	}
	if h.FinishedAt != nil {
		resp["finished_at"] = h.FinishedAt.Format(time.RFC3339)
	}
	return resp
}

// Ensure unused import is referenced.
var _ context.Context
