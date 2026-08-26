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

	"github.com/inferglow/approval"
)

// --- Run queue management (Phase 1, Task 1) ---

// handleRunQueue handles PATCH /v1/runs/{id}/queue — manage a run's planning
// queue (aligning with dsh-input-traffic's later/next/now tiers).
//
// Request body: {"kind": "edit|remove|steer|clear", "item_id": "...",
//   "tier": "later|next|now", "text": "...", "to_front": bool}
// For "push", use kind=push with tier/text.
//
// Response: {"queue": [...], "count": n}.
func (s *Server) handleRunQueue(w http.ResponseWriter, r *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Kind    string       `json:"kind"`
		ItemID  string       `json:"item_id"`
		Tier    RunQueueTier `json:"tier"`
		Text    string       `json:"text"`
		ToFront bool         `json:"to_front"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	switch req.Kind {
	case "push":
		item, err := s.runMgr.QueuePush(id, req.Tier, req.Text, "")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = item
	case "edit":
		if _, err := s.runMgr.QueueEdit(id, req.ItemID, req.Tier, req.Text); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	case "remove":
		if err := s.runMgr.QueueRemove(id, req.ItemID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	case "steer":
		if _, err := s.runMgr.QueueSteer(id, req.ItemID, req.Tier, req.ToFront); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	case "clear":
		if err := s.runMgr.QueueClear(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, `kind must be one of "push|edit|remove|steer|clear"`)
		return
	}

	queue, err := s.runMgr.QueueList(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": queue, "count": len(queue)})
}

// --- Run background jobs (Phase 1, Task 2) ---

// handleRunJobs handles GET /v1/runs/{id}/jobs — return a run's background
// jobs. Realtime updates flow over the existing /v1/runs/{id}/events SSE
// stream (job_started / job_done events).
func (s *Server) handleRunJobs(w http.ResponseWriter, r *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	id := r.PathValue("id")
	jobs, err := s.runMgr.Jobs(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "count": len(jobs)})
}

// --- Approval decision bridge (Phase 1, Task 5) ---

// approvalDecisionCarrier wraps the approval payload accepted by
// POST /v1/runs/{id}/input and /v1/approvals/{id}/decision.
type approvalDecisionCarrier struct {
	RecordID          string         `json:"record_id"`
	Approve           *bool          `json:"approve"`
	Approver          string         `json:"approver"`
	Justification     string         `json:"justification"`
	SandboxPermissions map[string]any `json:"sandbox_permissions"`
}

// handleRunInput handles POST /v1/runs/{id}/input — route an input to a run.
// It doubles as the approval decision bridge: when the body carries an
// approval decision (allow/reject) plus an optional justification and sandbox
// permissions, it resolves the pending record and (if approved) applies a
// sandbox-permission approval for the run.
//
// Request body (either form):
//   - {"message": "...", "preempt_mode": "queue|safe_point|force", "session_id": "..."}
//   - approval decision carried in fields above.
//
// Response: {"status": "...", "queue": [...], "record": {...}|nil}.
func (s *Server) handleRunInput(w http.ResponseWriter, r *http.Request) {
	if s.runMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "run manager not configured")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Message         string              `json:"message"`
		PreemptMode     string              `json:"preempt_mode"`
		SessionID       string              `json:"session_id"`
		approvalDecisionCarrier
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Approval decision path.
	if req.RecordID != "" {
		if req.Approve == nil {
			writeError(w, http.StatusBadRequest, "approve must be true/false when record_id is set")
			return
		}
		if s.approvalMgr == nil {
			writeError(w, http.StatusServiceUnavailable, "approval manager not configured")
			return
		}
		approver := req.Approver
		if approver == "" {
			approver = "webui"
		}
		rec, err := s.approvalMgr.ResolveRecord(req.RecordID, *req.Approve, approver)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		// Record the approving intent as a run-level background job.
		kind := "approval.sandbox_permissions"
		status := "completed"
		if !*req.Approve {
			kind = "approval.denied"
		}
		job, _ := s.runMgr.TrackJob(id, kind, req.Justification)
		if job != nil {
			_, _ = s.runMgr.UpdateJob(id, job.ID, status, "")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "resolved",
			"record": rec,
			"note":   req.SandboxPermissions,
		})
		return
	}

	// Plain input path: enqueue into the run's planning queue at the
	// requested tier (default later).
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message or record_id required")
		return
	}
	tier := RunQueueTier(req.PreemptMode)
	switch tier {
	case RunQueueTierNext, RunQueueTierNow:
	default:
		tier = RunQueueTierLater
	}
	item, err := s.runMgr.QueuePush(id, tier, req.Message, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	queue, _ := s.runMgr.QueueList(id)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "queued",
		"item":   item,
		"queue":  queue,
		"count":  len(queue),
	})
}

// --- Approval record endpoints (Phase 1, Task 5) ---

// handleListApprovals handles GET /v1/approvals — list pending/resolved
// approval records as surfaced by the approval manager.
func (s *Server) handleListApprovals(w http.ResponseWriter, _ *http.Request) {
	if s.approvalMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"approvals": []any{}, "count": 0})
		return
	}
	recs := s.approvalMgr.ListRecords()
	writeJSON(w, http.StatusOK, map[string]any{"approvals": recs, "count": len(recs)})
}

// handleSubmitApproval handles POST /v1/approvals — submit a new approval
// request and return its record (which is stored when it remains pending).
func (s *Server) handleSubmitApproval(w http.ResponseWriter, r *http.Request) {
	if s.approvalMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "approval manager not configured")
		return
	}
	var req approval.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Capability == "" || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "capability and subject are required")
		return
	}
	rec, err := s.approvalMgr.Submit(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

// handleApprovalDecision handles POST /v1/approvals/{id}/decision — resolve a
// pending approval record (allow/reject) via the approval manager.
func (s *Server) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	if s.approvalMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "approval manager not configured")
		return
	}
	id := r.PathValue("id")
	var req struct {
		Approve  *bool  `json:"approve"`
		Approver string `json:"approver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Approve == nil {
		writeError(w, http.StatusBadRequest, "approve must be true/false")
		return
	}
	approver := req.Approver
	if approver == "" {
		approver = "webui"
	}
	rec, err := s.approvalMgr.ResolveRecord(id, *req.Approve, approver)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}