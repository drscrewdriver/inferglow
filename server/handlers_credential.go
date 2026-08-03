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
)

// maskSecret returns a partial-mask preview of a secret: the first and last 4
// characters are preserved and the middle is replaced with mask, matching the
// spec's "keep first/last 4 + mask middle" rule. Short secrets collapse to a
// fully masked placeholder so the original length is never leaked.
func maskSecret(s string) string {
	const mask = "****"
	const edge = 4
	n := len(s)
	if n <= edge*2 {
		if n == 0 {
			return ""
		}
		return mask
	}
	return s[:edge] + mask + s[n-edge:]
}

// handleCreateCredential handles POST /v1/credentials — create a credential.
// The raw secret is validated, masked at write time (credential_store.Create)
// and never serialized in the response; only SecretMasked is exposed.
func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	if s.credStore == nil {
		writeError(w, http.StatusServiceUnavailable, "credential store not configured")
		return
	}
	if !s.canAccess(r, "credential", "", "create") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	var req struct {
		Name     string `json:"name" validate:"required"`
		Provider string `json:"provider" validate:"required"`
		Username string `json:"username,omitempty"`
		Secret   string `json:"secret" validate:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	rec := CredentialRecord{
		Name:        req.Name,
		Provider:    req.Provider,
		Username:    req.Username,
		SecretValue: req.Secret,
	}
	id, err := s.credStore.Create(rec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.credStore.Get(id))
}

// handleListCredentials handles GET /v1/credentials — list all credentials
// (masked only; the raw secret is never serialized).
func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	if s.credStore == nil {
		writeError(w, http.StatusServiceUnavailable, "credential store not configured")
		return
	}
	if !s.canAccess(r, "credential", "", "list") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	writeJSON(w, http.StatusOK, s.credStore.List())
}

// handleGetCredential handles GET /v1/credentials/{id} — return a single
// credential with its masked preview only.
func (s *Server) handleGetCredential(w http.ResponseWriter, r *http.Request) {
	if s.credStore == nil {
		writeError(w, http.StatusServiceUnavailable, "credential store not configured")
		return
	}
	id := r.PathValue("id")
	if !s.canAccess(r, "credential", id, "read") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	rec := s.credStore.Get(id)
	if rec == nil {
		writeError(w, http.StatusNotFound, "credential not found")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleDeleteCredential handles DELETE /v1/credentials/{id} — remove a
// credential.
func (s *Server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	if s.credStore == nil {
		writeError(w, http.StatusServiceUnavailable, "credential store not configured")
		return
	}
	id := r.PathValue("id")
	if !s.canAccess(r, "credential", id, "delete") {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := s.credStore.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}
