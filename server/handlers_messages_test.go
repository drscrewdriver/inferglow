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
	"testing"
	"time"
)

// seedMessages appends n messages to a session via the store directly.
func seedMessages(t *testing.T, srv *Server, sessionID string, n int) {
	t.Helper()
	base := time.Now().Add(-24 * time.Hour)
	for i := 0; i < n; i++ {
		_, err := srv.msgStore.Append(sessionID, MessageRecord{
			Role:      MessageRoleUser,
			Content:   "m",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestListSessionMessages_Pagination exercises the HTTP endpoint end to end:
// first page (limit 3 of 5) then cursor page (2 remaining, has_more=false).
func TestListSessionMessages_Pagination(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)
	seedMessages(t, srv, id, 5)

	// Page 1: newest 3 first.
	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages?limit=3", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("page1: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var page1 struct {
		Messages   []MessageRecord `json:"messages"`
		HasMore    bool            `json:"has_more"`
		NextBefore *string         `json:"next_before"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Messages) != 3 || !page1.HasMore {
		t.Fatalf("page1 = %d msgs has_more=%v, want 3/true", len(page1.Messages), page1.HasMore)
	}
	if page1.NextBefore == nil {
		t.Fatal("page1: want next_before cursor")
	}
	// Newest first ordering.
	if !page1.Messages[0].CreatedAt.After(page1.Messages[1].CreatedAt) {
		t.Fatal("messages not newest-first")
	}

	// Page 2: cursor returns the remaining 2, has_more=false, next_before=null.
	req = httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages?limit=3&before="+*page1.NextBefore, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("page2: want 200, got %d", w.Code)
	}
	var page2 struct {
		Messages   []MessageRecord `json:"messages"`
		HasMore    bool            `json:"has_more"`
		NextBefore *string         `json:"next_before"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Messages) != 2 || page2.HasMore {
		t.Fatalf("page2 = %d msgs has_more=%v, want 2/false", len(page2.Messages), page2.HasMore)
	}
	if page2.NextBefore != nil {
		t.Fatal("page2: want null next_before")
	}
}

// TestListSessionMessages_EmptyResult verifies an empty result means top reached.
func TestListSessionMessages_EmptyResult(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp struct {
		Messages   []MessageRecord `json:"messages"`
		HasMore    bool            `json:"has_more"`
		NextBefore *string         `json:"next_before"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 0 || resp.HasMore || resp.NextBefore != nil {
		t.Fatalf("resp = %+v, want empty top", resp)
	}
}

// TestListSessionMessages_DefaultLimit verifies limit defaults to 50.
func TestListSessionMessages_DefaultLimit(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)
	seedMessages(t, srv, id, 60)

	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp struct {
		Messages []MessageRecord `json:"messages"`
		HasMore  bool            `json:"has_more"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 50 || !resp.HasMore {
		t.Fatalf("len = %d has_more=%v, want 50/true", len(resp.Messages), resp.HasMore)
	}
}

// TestListSessionMessages_SessionNotFound verifies 404 for a missing session
// when the session store is wired.
func TestListSessionMessages_SessionNotFound(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	req := httptest.NewRequest("GET", "/v1/sessions/nope/messages", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestListSessionMessages_StoreNotConfigured verifies 503 without a message store.
func TestListSessionMessages_StoreNotConfigured(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())

	req := httptest.NewRequest("GET", "/v1/sessions/x/messages", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// TestListSessionMessages_InvalidBefore verifies a malformed cursor yields 400.
func TestListSessionMessages_InvalidBefore(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)

	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages?before=not-a-time", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
