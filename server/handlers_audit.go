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

	"github.com/inferglow/audit"
)

// auditChain holds the optional audit chain instance. Nil when audit is disabled.
// Set via SetAuditChain during server initialization.
var auditChain *audit.AuditChain

// SetAuditChain configures the audit chain for request/response logging.
// Pass nil to disable auditing (zero overhead).
//
// Typical usage in server startup:
//
//	if cfg.Audit.Enabled {
//	    chain, _ := audit.NewAuditChain(audit.AuditConfig{...})
//	    srv.SetAuditChain(chain)
//	}
func (s *Server) SetAuditChain(chain *audit.AuditChain) {
	auditChain = chain
}

// auditRequest records an HTTP request/response pair to the audit chain.
// No-op when audit is disabled (auditChain == nil).
func auditRequest(method, path string, statusCode int, duration time.Duration, userAgent string) {
	if auditChain == nil {
		return
	}
	entry := &audit.AuditEntry{
		Timestamp: time.Now(),
		Source:    "server",
		Action:    method + " " + path,
		Metadata: map[string]string{
			"status_code": http.StatusText(statusCode),
			"duration_ms": time.Duration(duration.Milliseconds()).String(),
			"user_agent":  userAgent,
		},
	}
	_, _ = auditChain.Append(entry)
}

// handleAuditVerify verifies the integrity of the audit chain.
// GET /v1/audit/verify
//
// Returns 200 with verification result when audit is enabled,
// or 501 when audit is disabled.
func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	if auditChain == nil {
		writeError(w, http.StatusNotImplemented, "audit chain is disabled")
		return
	}

	if err := auditChain.VerifyChain(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":   false,
			"error":   err.Error(),
			"entries": auditChain.Len(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":   true,
		"entries": auditChain.Len(),
	})
}

// handleAuditEntries returns recent audit entries.
// GET /v1/audit/entries?limit=100
func (s *Server) handleAuditEntries(w http.ResponseWriter, r *http.Request) {
	if auditChain == nil {
		writeError(w, http.StatusNotImplemented, "audit chain is disabled")
		return
	}

	entries := auditChain.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}
