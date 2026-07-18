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
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/inferglow/observability"
)

//go:embed dashboard.html
var dashboardHTML []byte

// SetSpanCollector injects the span collector for observability endpoints (OT-13).
func (s *Server) SetSpanCollector(c *observability.SpanCollector) {
	s.spanCollector = c
}

// handleObservabilitySpans returns recent spans as JSON.
// GET /v1/observability/spans?limit=N
func (s *Server) handleObservabilitySpans(w http.ResponseWriter, r *http.Request) {
	if s.spanCollector == nil {
		http.Error(w, "observability not enabled", http.StatusServiceUnavailable)
		return
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	spans := s.spanCollector.Recent(limit)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spans)
}

// handleObservabilityStats returns aggregated span statistics.
// GET /v1/observability/stats
func (s *Server) handleObservabilityStats(w http.ResponseWriter, r *http.Request) {
	if s.spanCollector == nil {
		http.Error(w, "observability not enabled", http.StatusServiceUnavailable)
		return
	}

	stats := s.spanCollector.Aggregate()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

// handleDashboard serves the embedded observability dashboard HTML.
// GET /dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
}
