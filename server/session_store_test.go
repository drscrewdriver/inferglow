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

// Behavior tests for the C-4 session store and its CRUD + SSE surface.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inferglow/messagebus"
)

// TestSessionStoreConcurrentCreateUniqueIDs mirrors the TeamStore template:
// concurrent Create calls must yield unique IDs under the Map-backed store.
func TestSessionStoreConcurrentCreateUniqueIDs(t *testing.T) {
	ss := NewSessionStore()
	const workers, perWorker = 8, 20
	results := make(chan string, workers*perWorker)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id, err := ss.Create(SessionRecord{Owner: "o", AgentID: "a"})
				if err != nil {
					t.Errorf("Create: %v", err)
					return
				}
				results <- id
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, workers*perWorker)
	for id := range results {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate session ID: %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(ss.List()) != workers*perWorker {
		t.Fatalf("List len = %d, want %d", len(ss.List()), workers*perWorker)
	}
}

// TestSessionCRUD exercises create/get/delete end-to-end over HTTP.
func TestSessionCRUD(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	srv.SetMessageBus(messagebus.NewInMemoryMessageBus())

	body := `{"owner":"alice","agent_id":"a1"}`
	req := httptest.NewRequest("POST", "/v1/sessions", strings.NewReader(body))
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
	if created.ID == "" {
		t.Fatal("missing session id")
	}

	req = httptest.NewRequest("GET", "/v1/sessions/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}

	req = httptest.NewRequest("DELETE", "/v1/sessions/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}

	// After delete, get returns 404.
	req = httptest.NewRequest("GET", "/v1/sessions/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404, got %d", w.Code)
	}
}

// TestSessionDeleteBroadcastsTermination verifies the extensible cascade-clean
// linkage: deleting a session publishes a session.terminated event on the bus.
func TestSessionDeleteBroadcastsTermination(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	bus := messagebus.NewInMemoryMessageBus()
	srv.SetMessageBus(bus)

	id, err := srv.sessionStore.Create(SessionRecord{Owner: "o", AgentID: "a"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := bus.DrainSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/v1/sessions/"+id, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}

	select {
	case msg := <-ch:
		if msg.Kind != "session.terminated" {
			t.Fatalf("want session.terminated, got %q", msg.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session.terminated event")
	}
}

// TestSessionStreamDeliversAndCloses verifies the SSE stream delivers a session
// event and then terminates cleanly when the request context is cancelled.
func TestSessionStreamDeliversAndCloses(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore())
	bus := messagebus.NewInMemoryMessageBus()
	srv.SetMessageBus(bus)

	id, err := srv.sessionStore.Create(SessionRecord{Owner: "o", AgentID: "a"})
	if err != nil {
		t.Fatal(err)
	}

	// Publish a session-bound event the stream should relay.
	if err := bus.Publish(context.Background(), "session", messagebus.Message{
		ID:        id,
		SessionID: id,
		Topic:     "session",
		Kind:      "session.updated",
		Payload:   map[string]any{"session_id": id},
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/stream", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(w, req)
		close(done)
	}()

	// Give the drainLoop a chance to deliver, then cancel to end the stream.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not terminate after context cancel")
	}
	if !strings.Contains(w.Body.String(), "session.updated") {
		t.Fatalf("expected session.updated SSE event, got %q", w.Body.String())
	}
}

// TestSessionUnconfigured503 asserts the handlers degrade to 503 when no
// session store is wired.
func TestSessionUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetSessionStore
	req := httptest.NewRequest("GET", "/v1/sessions", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}

// TestSessionStreamUnconfigured503 verifies the SSE route 503s when the bus is
// absent (even if the store is present).
func TestSessionStreamUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetSessionStore(NewSessionStore()) // no bus
	id, err := srv.sessionStore.Create(SessionRecord{Owner: "o", AgentID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/sessions/"+id+"/stream", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
