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
	"net/http/httptest"
	"strings"
	"testing"
)

// createSessionViaAPI creates a session over HTTP and returns its ID.
func createSessionViaAPI(t *testing.T, srv *Server) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/sessions", strings.NewReader(`{"agent_id":"a1"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// TestUpdateSessionMeta_PatchGroupPinRename verifies PATCH updates title/group/
// pinned in one request and returns the refreshed record.
func TestUpdateSessionMeta_PatchGroupPinRename(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	id := createSessionViaAPI(t, srv)

	body := `{"title":"GUI 会话","group":"工作","pinned":true}`
	req := httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var rec SessionRecord
	if err := json.NewDecoder(w.Body).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if rec.Title != "GUI 会话" || rec.Group != "工作" || !rec.Pinned {
		t.Fatalf("record = %+v, want title/group/pinned applied", rec)
	}
}

// TestUpdateSessionMeta_ClearField verifies an explicit empty value clears a field.
func TestUpdateSessionMeta_ClearField(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(`{"title":"x"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d", w.Code)
	}

	req = httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(`{"title":""}`))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch clear: want 200, got %d", w.Code)
	}
	var rec SessionRecord
	if err := json.NewDecoder(w.Body).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if rec.Title != "" {
		t.Fatalf("title = %q, want cleared", rec.Title)
	}
}

// TestUpdateSessionMeta_UnprovidedFieldLeftUntouched verifies absent fields
// are not overwritten.
func TestUpdateSessionMeta_UnprovidedFieldLeftUntouched(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(`{"group":"g1"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d", w.Code)
	}
	var rec SessionRecord
	if err := json.NewDecoder(w.Body).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if rec.Group != "g1" || rec.Title != "" || rec.Pinned {
		t.Fatalf("record = %+v, want only group applied", rec)
	}
}

// TestUpdateSessionMeta_ArchiveStatus verifies status can be flipped to archived.
func TestUpdateSessionMeta_ArchiveStatus(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(`{"status":"archived"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d", w.Code)
	}
	var rec SessionRecord
	if err := json.NewDecoder(w.Body).Decode(&rec); err != nil {
		t.Fatal(err)
	}
	if rec.Status != SessionStatusArchived {
		t.Fatalf("status = %q, want archived", rec.Status)
	}
}

// TestUpdateSessionMeta_NotFound verifies PATCH on a missing session returns 404.
func TestUpdateSessionMeta_NotFound(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	req := httptest.NewRequest("PATCH", "/v1/sessions/nope", strings.NewReader(`{"title":"x"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestUpdateSessionMeta_StoreNotConfigured verifies PATCH returns 503 when the
// session store is not wired up.
func TestUpdateSessionMeta_StoreNotConfigured(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())

	req := httptest.NewRequest("PATCH", "/v1/sessions/x", strings.NewReader(`{"title":"x"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// TestUpdateSessionMeta_InvalidBody verifies malformed JSON yields 400.
func TestUpdateSessionMeta_InvalidBody(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("PATCH", "/v1/sessions/"+id, strings.NewReader(`{bad json`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
