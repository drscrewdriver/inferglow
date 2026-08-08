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
	"time"

	"github.com/inferglow/session"
)

// handleUsageReport handles GET /v1/usage/report — aggregated usage statistics
// across sessions (tokens / cache hit rate / cost) backed by the session
// module's ReportGenerator.
//
// Query params:
//   - from/to: RFC3339 range bounds (absent = current month)
//   - model:   optional model filter
func (s *Server) handleUsageReport(w http.ResponseWriter, r *http.Request) {
	gen := s.reportGen
	if gen == nil {
		gen = session.NewReportGenerator(s.cfg.UsageDataDir)
	}

	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 1, 0)
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from timestamp: "+err.Error())
			return
		}
		from = parsed
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to timestamp: "+err.Error())
			return
		}
		to = parsed
	}

	model := r.URL.Query().Get("model")
	report, err := gen.Generate(r.Context(), from, to, model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate report: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
