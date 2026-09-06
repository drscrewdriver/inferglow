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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/server/config"
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

// TestTracePersistence — a stream run persists one trace-role record; it is
// served by /v1/sessions/{id}/trace and EXCLUDED from the chat history
// listing, so restoring a session rebuilds the 轨迹 panel without polluting
// the transcript.
func TestTracePersistence(t *testing.T) {
	chunks := []string{"ok"}
	llmSrv := fakeOpenAIStreamLLM(chunks)
	defer llmSrv.Close()

	store, err := NewConfigAgentStore(config.MultiLLMConfig{
		Providers: map[string]config.LLMConfig{
			"fake": {Provider: "openai", BaseURL: llmSrv.URL, Model: "mock-1", APIKey: "test-key"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(DefaultConfig(), store)
	srv.SetMessageStore(NewMessageStore())

	req := httptest.NewRequest("POST", "/v1/agents/fake/stream-run", strings.NewReader(`{"message":"hi","session_id":"sess-trace"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Chat listing must NOT contain the trace record.
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, httptest.NewRequest("GET", "/v1/sessions/sess-trace/messages?limit=50", nil))
	if strings.Contains(w2.Body.String(), `"role":"trace"`) {
		t.Fatalf("trace leaked into chat listing: %s", w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), `"role":"assistant"`) {
		t.Fatalf("assistant record missing: %s", w2.Body.String())
	}

	// Trace endpoint returns the run summary with spans + usage.
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, httptest.NewRequest("GET", "/v1/sessions/sess-trace/trace", nil))
	if w3.Code != 200 {
		t.Fatalf("trace endpoint: %d %s", w3.Code, w3.Body.String())
	}
	var payload struct {
		Traces []struct {
			Content string `json:"content"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &payload); err != nil || len(payload.Traces) != 1 {
		t.Fatalf("trace endpoint: err=%v traces=%d body=%s", err, len(payload.Traces), w3.Body.String())
	}
	summary := payload.Traces[0].Content
	for _, want := range []string{`"agent_id":"fake"`, `"kind":"llm"`, `"kind":"agent"`} {
		if !strings.Contains(summary, want) {
			t.Fatalf("trace summary missing %s: %s", want, summary)
		}
	}
}

// TestPersistenceRoundTrip — sessions, chat history (user/assistant/tool)
// and run traces survive store reconstruction: create + stream → snapshot →
// fresh stores + Load → everything back, trace excluded from chat listing.
func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()

	llmSrv := fakeOpenAIStreamLLM([]string{"持久化回复"})
	defer llmSrv.Close()
	store, err := NewConfigAgentStore(config.MultiLLMConfig{
		Providers: map[string]config.LLMConfig{
			"fake": {Provider: "openai", BaseURL: llmSrv.URL, Model: "m", APIKey: "k"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(DefaultConfig(), store)
	ss := NewSessionStore()
	ss.SetPersistence(filepath.Join(dir, "sessions.json"))
	srv.SetSessionStore(ss)
	ms := NewMessageStore()
	ms.SetPersistence(filepath.Join(dir, "messages.json"))
	srv.SetMessageStore(ms)

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/sessions", strings.NewReader(`{"agent_id":"fake","title":"roundtrip","workspace":"rewrite-agently"}`)))
	if w.Code != 201 {
		t.Fatalf("create session: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/agents/fake/stream-run", strings.NewReader(`{"message":"hi","session_id":"sess-1"}`)))

	// ── Fresh stores, same snapshot files ──
	ss2 := NewSessionStore()
	if !ss2.SetPersistence(filepath.Join(dir, "sessions.json")) {
		t.Fatalf("session snapshot not restored")
	}
	ms2 := NewMessageStore()
	ms2.SetPersistence(filepath.Join(dir, "messages.json"))
	if got := ss2.Get("sess-1"); got == nil || got.Title != "roundtrip" || got.Workspace != "rewrite-agently" {
		t.Fatalf("session not restored: %+v", got)
	}
	srv2 := NewServer(DefaultConfig(), store)
	srv2.SetSessionStore(ss2)
	srv2.SetMessageStore(ms2)

	w = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/sessions/sess-1/messages?limit=50", nil))
	body := w.Body.String()
	if !strings.Contains(body, "持久化回复") {
		t.Fatalf("assistant history lost: %s", body)
	}
	if strings.Contains(body, `"role":"trace"`) {
		t.Fatalf("trace leaked into chat after restore: %s", body)
	}
	w = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/sessions/sess-1/trace", nil))
	var tp struct {
		Traces []struct {
			Content string `json:"content"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &tp); err != nil || len(tp.Traces) == 0 {
		t.Fatalf("trace not restored: err=%v n=%d body=%s", err, len(tp.Traces), w.Body.String())
	}
	if !strings.Contains(tp.Traces[0].Content, `"agent_id":"fake"`) {
		t.Fatalf("restored trace content wrong: %s", tp.Traces[0].Content)
	}
	// New session continues the ID sequence (no sess-1 collision).
	w = httptest.NewRecorder()
	srv2.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/sessions", strings.NewReader(`{"agent_id":"fake"}`)))
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"id":"sess-2"`) {
		t.Fatalf("ID sequence not restored: %d %s", w.Code, w.Body.String())
	}
}

// TestTasksSharedStore — the /v1/tasks CRUD surface and (via the same store
// instance) the model's task_tracker tools operate on one persisted file:
// webui creates → model-side List sees it; restart reloads it.
func TestTasksSharedStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	SetTaskStore(actions.NewTaskStore(path))
	t.Cleanup(func() { SetTaskStore(nil) })

	srv := NewServer(DefaultConfig(), newMockStore())

	// Create via the webui API.
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", strings.NewReader(`{"title":"面板新建"}`)))
	if w.Code != 201 {
		t.Fatalf("create task: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// The model-side store instance (same file, fresh handle) sees it.
	modelView := actions.NewTaskStore(path).List("")
	if len(modelView) != 1 || modelView[0].Title != "面板新建" {
		t.Fatalf("model task_list misses webui task: %+v", modelView)
	}

	// Patch status; list endpoint reflects it.
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("PATCH", "/v1/tasks/"+created.ID, strings.NewReader(`{"status":"done"}`)))
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/tasks?status=done", nil))
	if !strings.Contains(w.Body.String(), "面板新建") {
		t.Fatalf("status filter broken: %s", w.Body.String())
	}

	// Persistence: a fresh server-side view restores the done state.
	after := actions.NewTaskStore(path).List("done")
	if len(after) != 1 {
		t.Fatalf("task snapshot lost: %+v", after)
	}
}
