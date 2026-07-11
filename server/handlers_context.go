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
)

// handleContextSearch performs a semantic or keyword search via ContextProvider.
// GET /v1/context/search?q=...&limit=10&scope=session|task_group|global
func (s *Server) handleContextSearch(w http.ResponseWriter, r *http.Request) {
	if s.ctxProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "context provider not configured")
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}

	hits, err := s.ctxProvider.Search(r.Context(), q, limit, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hits":  hits,
		"count": len(hits),
		"scope": scope,
	})
}

// handleContextStats returns context subsystem statistics.
// GET /v1/context/stats
func (s *Server) handleContextStats(w http.ResponseWriter, r *http.Request) {
	if s.ctxProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "context provider not configured")
		return
	}
	stats := s.ctxProvider.Stats()
	writeJSON(w, http.StatusOK, stats)
}
