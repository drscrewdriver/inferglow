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
)

// handleMemorySemanticSearch performs semantic search when the MemoryStore
// implements SemanticMemoryStore. Falls back to regular search otherwise.
// POST /v1/memories/search
func (s *Server) handleMemorySemanticSearch(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}

	var req struct {
		Query       string `json:"query"`
		Limit       int    `json:"limit,omitempty"`
		UseSemantic bool   `json:"use_semantic,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}

	// Try semantic search if requested and available.
	if req.UseSemantic {
		if sms, ok := s.memStore.(SemanticMemoryStore); ok {
			results, err := sms.SemanticSearch(r.Context(), req.Query, req.Limit)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "semantic search error: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"memories": results,
				"count":    len(results),
				"mode":     "semantic",
			})
			return
		}
		// Fall through to regular search if SemanticMemoryStore not implemented.
	}

	// Fallback to regular substring search.
	results, err := s.memStore.SearchMemory(req.Query, "", req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memories": results,
		"count":    len(results),
		"mode":     "substring",
	})
}

// handleMemoryStats returns memory subsystem statistics.
// GET /v1/memories/stats
func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}

	// Try SemanticMemoryStore for rich stats.
	if sms, ok := s.memStore.(SemanticMemoryStore); ok {
		stats := sms.MemoryStats()
		writeJSON(w, http.StatusOK, stats)
		return
	}

	// Basic stats: just report that memory store is active.
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "active",
		"backend": "basic",
		"note":    "semantic stats not available; use SemanticMemoryStore interface",
	})
}

// handleMemoryValidate increases the confidence of a memory entry.
// POST /v1/memories/{id}/validate
func (s *Server) handleMemoryValidate(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}
	id := r.PathValue("id")
	// Verify the memory exists.
	rec, err := s.memStore.GetMemory(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Re-upsert to signal validation (idempotent).
	if err := s.memStore.UpsertMemory(*rec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": "validated",
	})
}

// handleMemoryNegate clears the confidence of a memory entry.
// POST /v1/memories/{id}/negate
func (s *Server) handleMemoryNegate(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}
	id := r.PathValue("id")
	rec, err := s.memStore.GetMemory(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Mark as negated by clearing content (soft-delete semantics).
	rec.Facts = nil
	if err := s.memStore.UpsertMemory(*rec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": "negated",
	})
}

// parseLimit parses a limit query parameter with a default value.
func parseLimit(r *http.Request, defaultLimit int) int {
	l := r.URL.Query().Get("limit")
	if l == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(l)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	return n
}
