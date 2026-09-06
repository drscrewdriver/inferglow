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
	"net/http"
	"strconv"
	"time"
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
	traces := s.msgStore.ListTraces(id, limit)
	writeJSON(w, http.StatusOK, map[string]any{"traces": traces})
}
