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

// Behavior tests for the C-9 MCP Hub store and HTTP handlers.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inferglow/action"
)

// echoExecutor satisfies action.ActionExecutor by echoing its input back.
type echoExecutor struct{}

func (echoExecutor) Execute(_ context.Context, input map[string]any) (*action.ActionResult, error) {
	return &action.ActionResult{OK: true, Status: "success", Result: input}, nil
}

// newEchoAction builds a real, executable action named "echo".
func newEchoAction() *action.Action {
	return &action.Action{
		Name:        "echo",
		Description: "echoes input back",
		Schema:      map[string]any{"type": "object"},
		Executor:    echoExecutor{},
	}
}

// installEchoMCPTool registers a tiny echo tool into the MCP Hub store.
func installEchoMCPTool(t *testing.T, m *MCPHubStore) {
	t.Helper()
	if err := m.Install(newEchoAction()); err != nil {
		t.Fatal(err)
	}
}

// TestMCPHubStoreInstallListCallDelete exercises the store layer.
func TestMCPHubStoreInstallListCallDelete(t *testing.T) {
	m := NewMCPHubStore()
	installEchoMCPTool(t, m)

	if got := m.List(); len(got) != 1 || got[0].Name != "echo" {
		t.Fatalf("List = %v, want [echo]", got)
	}

	rec, err := m.Get("echo")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "echo" {
		t.Fatalf("Get returned unexpected record: %+v", rec)
	}

	// Call the echo tool.
	result, err := m.Call(context.Background(), "echo", map[string]any{"in": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("Call returned unexpected result: %+v", result)
	}

	if err := m.Remove("echo"); err != nil {
		t.Fatal(err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("List after remove = %v, want empty", got)
	}
	if _, err := m.Get("echo"); err == nil {
		t.Fatal("expected error after remove")
	}
	if _, err := m.Call(context.Background(), "echo", nil); err == nil {
		t.Fatal("expected error calling a removed tool")
	}
}

// TestMCPHubHandlerEndToEnd drives the HTTP layer end-to-end.
func TestMCPHubHandlerEndToEnd(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	m := NewMCPHubStore()
	installEchoMCPTool(t, m)
	srv.SetMCPHubStore(m)

	// List.
	req := httptest.NewRequest("GET", "/v1/mcp-hub", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"echo"`) {
		t.Fatalf("list: want 200 with echo, got %d (%s)", w.Code, w.Body.String())
	}

	// Get.
	req = httptest.NewRequest("GET", "/v1/mcp-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}
	var got MCPToolRecord
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "echo" {
		t.Fatalf("get returned unexpected record: %+v", got)
	}

	// Call.
	req = httptest.NewRequest("POST", "/v1/mcp-hub/echo/call",
		strings.NewReader(`{"arguments":{"text":"hi"}}`))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("call: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	// Delete.
	req = httptest.NewRequest("DELETE", "/v1/mcp-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}

	// Get after delete -> 404.
	req = httptest.NewRequest("GET", "/v1/mcp-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404, got %d", w.Code)
	}
}

// TestMCPHubUnconfigured503 asserts handlers return 503 when the store is not wired.
func TestMCPHubUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetMCPHubStore
	req := httptest.NewRequest("GET", "/v1/mcp-hub", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}