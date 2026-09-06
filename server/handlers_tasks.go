// Copyright 2026 InferGlow Authors

package server

// Webui todo management (R8) — the model's task_tracker tools and the webui
// 待办 panel share ONE builtins actions.TaskStore instance (JSON file under
// UsageDataDir), so tasks the model adds mid-conversation appear in the
// panel and panel edits are visible to the model's next task_list call.

import (
	"encoding/json"
	"net/http"

	"github.com/inferglow/builtins/actions"
)

var taskStore *actions.TaskStore

// SetTaskStore wires the shared task store (called from main.go). Nil keeps
// the /v1/tasks endpoints returning 503.
func SetTaskStore(ts *actions.TaskStore) { taskStore = ts }

// handleListTasks handles GET /v1/tasks?status=pending|in_progress|done|cancelled.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if taskStore == nil {
		writeError(w, http.StatusServiceUnavailable, "task store not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tasks": taskStore.List(r.URL.Query().Get("status")),
	})
}

// handleCreateTask handles POST /v1/tasks {title, description?, priority?}.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if taskStore == nil {
		writeError(w, http.StatusServiceUnavailable, "task store not configured")
		return
	}
	var req struct {
		Title       string `json:"title" validate:"required"`
		Description string `json:"description,omitempty"`
		Priority    int    `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	writeJSON(w, http.StatusCreated, taskStore.Add(req.Title, req.Description, req.Priority))
}

// handlePatchTask handles PATCH /v1/tasks/{id} {status?, title?, priority?}.
func (s *Server) handlePatchTask(w http.ResponseWriter, r *http.Request) {
	if taskStore == nil {
		writeError(w, http.StatusServiceUnavailable, "task store not configured")
		return
	}
	var req struct {
		Status   *string `json:"status,omitempty"`
		Title    *string `json:"title,omitempty"`
		Priority *int    `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	changes := map[string]any{}
	if req.Status != nil {
		changes["status"] = *req.Status
	}
	if req.Title != nil {
		changes["title"] = *req.Title
	}
	if req.Priority != nil {
		changes["priority"] = *req.Priority
	}
	task, err := taskStore.Update(r.PathValue("id"), changes)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// handleDeleteTask handles DELETE /v1/tasks/{id}.
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if taskStore == nil {
		writeError(w, http.StatusServiceUnavailable, "task store not configured")
		return
	}
	if err := taskStore.Delete(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
