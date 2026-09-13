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
	"strconv"
	"time"

	"github.com/inferglow/model"
)

// defaultMessagePageSize is the page size used when ?limit= is absent.
const defaultMessagePageSize = 50

// maxMessagePageSize caps a single history page.
const maxMessagePageSize = 200

// handleListSessionMessages handles GET /v1/sessions/{id}/messages — paginated
// chat history for a session, newest first.
//
// Query params:
//   - before: RFC3339 timestamp cursor; messages older than it are returned
//     (absent = start from the newest page)
//   - limit:  page size, default 50, max 200
//
// Response: {"messages": [...], "has_more": bool, "next_before": ts|null}.
// An empty result means the client reached the top of the history.
func (s *Server) handleListSessionMessages(w http.ResponseWriter, r *http.Request) {
	if s.msgStore == nil {
		writeError(w, http.StatusServiceUnavailable, "message store not configured")
		return
	}
	id := r.PathValue("id")
	if s.sessionStore != nil && s.sessionStore.Get(id) == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var before time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid before timestamp: "+err.Error())
			return
		}
		before = parsed
	}

	limit := defaultMessagePageSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = min(n, maxMessagePageSize)
	}

	msgs, hasMore := s.msgStore.ListBefore(id, before, limit)

	var nextBefore *string
	if hasMore && len(msgs) > 0 {
		ts := msgs[len(msgs)-1].CreatedAt.UTC().Format(time.RFC3339)
		nextBefore = &ts
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":    msgs,
		"has_more":    hasMore,
		"next_before": nextBefore,
	})
}

// handleGetSessionTrace handles GET /v1/sessions/{id}/trace — the session's
// persisted run summaries (trace-role records, newest first). The 轨迹/上下文
// panels rebuild from this after restarts and session restores; empty trace
// list = the session predates trace persistence (panels render "—").
//
// Each entry embeds the raw MessageRecord (content stays the original JSON
// string for old clients) and adds a structured `summary` (model.
// RunTraceSummary: spans / requests / turns / usage) whenever the content
// parses. Old snapshots without the schema:2 keys degrade to summary=null.
//
// Query params:
//   - limit:    max entries (default 100, max 500)
//   - detail:   "false" strips spans/requests from the summaries (light list)
//   - from_turn / to_turn: keep only runs whose turn range intersects
//     [from_turn, to_turn] (single-turn sessions have turn=1)
func (s *Server) handleGetSessionTrace(w http.ResponseWriter, r *http.Request) {
	if s.msgStore == nil {
		writeError(w, http.StatusServiceUnavailable, "message store not configured")
		return
	}
	id := r.PathValue("id")
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = min(n, 500)
		}
	}
	detail := r.URL.Query().Get("detail") != "false"
	fromTurn, toTurn := -1, -1
	if raw := r.URL.Query().Get("from_turn"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			fromTurn = n
		} else {
			writeError(w, http.StatusBadRequest, "invalid from_turn")
			return
		}
	}
	if raw := r.URL.Query().Get("to_turn"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			toTurn = n
		} else {
			writeError(w, http.StatusBadRequest, "invalid to_turn")
			return
		}
	}

	records := s.msgStore.ListTraces(id, limit)
	type traceEntry struct {
		MessageRecord
		Summary *model.RunTraceSummary `json:"summary,omitempty"`
	}
	out := make([]traceEntry, 0, len(records))
	for _, rec := range records {
		entry := traceEntry{MessageRecord: rec}
		if rec.Role == MessageRoleTrace {
			var summary model.RunTraceSummary
			if err := json.Unmarshal([]byte(rec.Content), &summary); err == nil {
				if !detail {
					summary.Spans = nil
					summary.Requests = nil
				}
				if !turnRangeIntersects(&summary, fromTurn, toTurn) {
					continue
				}
				entry.Summary = &summary
			}
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"traces": out})
}

// turnRangeIntersects reports whether the run's turn coverage intersects
// [fromTurn, toTurn] (either bound absent = open-ended; both absent = no
// filtering).
func turnRangeIntersects(summary *model.RunTraceSummary, fromTurn, toTurn int) bool {
	if fromTurn < 0 && toTurn < 0 {
		return true
	}
	lo, hi := 1<<30, -1
	for _, t := range summary.Turns {
		lo = min(lo, t.Turn)
		hi = max(hi, t.Turn)
	}
	for _, req := range summary.Requests {
		lo = min(lo, req.Turn)
		hi = max(hi, req.Turn)
	}
	if hi < 0 { // no turn dimensions persisted — keep the entry visible
		return true
	}
	if fromTurn >= 0 && hi < fromTurn {
		return false
	}
	if toTurn >= 0 && lo > toTurn {
		return false
	}
	return true
}
