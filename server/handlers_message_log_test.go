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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestChatRecordsMessages verifies a chat with session_id persists a user and
// assistant message that the history endpoint then serves.
func TestChatRecordsMessages(t *testing.T) {
	store := newMockStore()
	store.agents["a1"] = &mockAgent{name: "test"}

	srv := NewServer(DefaultConfig(), store)
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)

	body := `{"message":"hello","session_id":"` + id + `"}`
	req := httptest.NewRequest("POST", "/v1/agents/a1/chat", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chat: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("messages: want 200, got %d", w.Code)
	}
	var resp struct {
		Messages []MessageRecord `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(resp.Messages))
	}
	// Newest first: assistant then user.
	if resp.Messages[0].Role != MessageRoleAssistant || resp.Messages[0].Content != "echo: hello" {
		t.Fatalf("msgs[0] = %+v, want assistant reply", resp.Messages[0])
	}
	if resp.Messages[1].Role != MessageRoleUser || resp.Messages[1].Content != "hello" {
		t.Fatalf("msgs[1] = %+v, want user message", resp.Messages[1])
	}
}

// TestChatResponseByteCompatibility guards the chat response contract: the
// body bytes must be identical with and without a message store attached.
func TestChatResponseByteCompatibility(t *testing.T) {
	mk := func(withStore bool) string {
		store := newMockStore()
		store.agents["a1"] = &mockAgent{name: "test"}
		srv := NewServer(DefaultConfig(), store)
		if withStore {
			srv.SetMessageStore(NewMessageStore())
		}
		body := `{"message":"hello","session_id":"sess-1"}`
		req := httptest.NewRequest("POST", "/v1/agents/a1/chat", strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("chat: want 200, got %d", w.Code)
		}
		return w.Body.String()
	}

	without := mk(false)
	with := mk(true)
	if without != with {
		t.Fatalf("chat response bytes differ:\nwithout store: %s\nwith store:    %s", without, with)
	}
}

// TestChatNoSessionID_NoRecording verifies messages are not recorded when the
// request carries no session_id.
func TestChatNoSessionID_NoRecording(t *testing.T) {
	store := newMockStore()
	store.agents["a1"] = &mockAgent{name: "test"}

	srv := NewServer(DefaultConfig(), store)
	srv.SetMessageStore(NewMessageStore())

	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/v1/agents/a1/chat", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chat: want 200, got %d", w.Code)
	}
	if got := srv.msgStore.Count("sess-1"); got != 0 {
		t.Fatalf("msgStore.Count = %d, want 0 (no session attached)", got)
	}
}

// TestStreamRunRecordsMessages verifies stream-run persists the user message
// and the assistant reply carried by the run_end event.
func TestStreamRunRecordsMessages(t *testing.T) {
	store := newMockStore()
	store.agents["a1"] = &mockAgent{name: "test"}

	srv := NewServer(DefaultConfig(), store)
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageStore(NewMessageStore())

	id := createSessionViaAPI(t, srv)

	body := `{"message":"hi","session_id":"` + id + `"}`
	req := httptest.NewRequest("POST", "/v1/agents/a1/stream-run", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stream-run: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	// The stream must have produced at least run_start/run_end/done.
	if !bytes.Contains(w.Body.Bytes(), []byte(`"run_end"`)) {
		t.Fatalf("stream body missing run_end: %s", w.Body.String())
	}

	req = httptest.NewRequest("GET", "/v1/sessions/"+id+"/messages", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("messages: want 200, got %d", w.Code)
	}
	var resp struct {
		Messages []MessageRecord `json:"messages"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2 (user + assistant)", len(resp.Messages))
	}
	// run_end.tool_name carries the assistant reply; the echo mock returns
	// "echo: hi" which must have been persisted as the assistant content.
	if resp.Messages[0].Role != MessageRoleAssistant || resp.Messages[0].Content != "echo: hi" {
		t.Fatalf("msgs[0] = %+v, want assistant 'echo: hi'", resp.Messages[0])
	}
	if resp.Messages[1].Role != MessageRoleUser || resp.Messages[1].Content != "hi" {
		t.Fatalf("msgs[1] = %+v, want user 'hi'", resp.Messages[1])
	}
}
