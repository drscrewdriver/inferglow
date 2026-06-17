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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// mockAgentStore is a simple in-memory agent store for testing.
type mockAgentStore struct {
	agents map[string]*mockAgent
	nextID int
}

type mockAgent struct {
	name string
}

func (m *mockAgent) Run(ctx context.Context, msg string) (string, error) {
	return "echo: " + msg, nil
}

func newMockStore() *mockAgentStore {
	return &mockAgentStore{agents: make(map[string]*mockAgent)}
}

func (s *mockAgentStore) Get(id string) AgentLike {
	a, ok := s.agents[id]
	if !ok {
		return nil
	}
	return a
}

func (s *mockAgentStore) List() []AgentLike {
	var out []AgentLike
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out
}

func (s *mockAgentStore) Create(cfg AgentConfig) (string, error) {
	s.nextID++
	id := "agent-" + strconv.Itoa(s.nextID)
	s.agents[id] = &mockAgent{name: cfg.Name}
	return id, nil
}

func (s *mockAgentStore) Delete(id string) error {
	delete(s.agents, id)
	return nil
}

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("want healthy, got %v", resp["status"])
	}
}

func TestListAgentsEmpty(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	req := httptest.NewRequest("GET", "/v1/agents", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestCreateAndGetAgent(t *testing.T) {
	store := newMockStore()
	srv := NewServer(DefaultConfig(), store)

	// Create
	body := `{"name":"test-agent","model":"gpt-4"}`
	req := httptest.NewRequest("POST", "/v1/agents", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", w.Code)
	}

	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	id := createResp["id"]
	if id == "" {
		t.Fatal("missing agent id")
	}

	// Get
	req = httptest.NewRequest("GET", "/v1/agents/"+id, nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("get: want 200, got %d", w.Code)
	}
}

func TestChatEndpoint(t *testing.T) {
	store := newMockStore()
	store.agents["a1"] = &mockAgent{name: "test"}

	srv := NewServer(DefaultConfig(), store)
	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/v1/agents/a1/chat", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var resp ChatResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Response != "echo: hello" {
		t.Errorf("want 'echo: hello', got %q", resp.Response)
	}
}

func TestChatNotFound(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/v1/agents/missing/chat", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestOpenAPISpec(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var spec map[string]any
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Errorf("want openapi 3.0.3, got %v", spec["openapi"])
	}
}
