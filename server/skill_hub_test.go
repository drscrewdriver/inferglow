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

// Behavior tests for the C-10 Skill Hub store and HTTP handlers.

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

// skillExecFunc adapts a plain function to the action.ActionExecutor contract.
type skillExecFunc func(ctx context.Context, input map[string]any) (*action.ActionResult, error)

func (f skillExecFunc) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	return f(ctx, input)
}

// installEchoSkill registers a tiny echo skill used by the tests below.
func installEchoSkill(t *testing.T, ss *SkillStore) {
	t.Helper()
	if err := ss.Install(&action.Action{
		Name:        "echo",
		Description: "Echo the input back",
		Tags:        []string{"builtin", "demo"},
		Executor: skillExecFunc(func(_ context.Context, input map[string]any) (*action.ActionResult, error) {
			return &action.ActionResult{OK: true, Status: "success", Result: input}, nil
		}),
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSkillStoreInstallListExecuteDelete exercises the store layer: install,
// list, get, execute, soft-delete and the post-delete 404 behaviour.
func TestSkillStoreInstallListExecuteDelete(t *testing.T) {
	ss := NewSkillStore()
	installEchoSkill(t, ss)

	if got := ss.List(); len(got) != 1 || got[0] != "echo" {
		t.Fatalf("List = %v, want [echo]", got)
	}

	a, err := ss.Get("echo")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "echo" || a.Executor == nil {
		t.Fatalf("Get returned unexpected action: %+v", a)
	}

	res, err := ss.Execute(context.Background(), "echo", map[string]any{"in": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("Execute hypern OK=false: %+v", res)
	}

	if err := ss.Remove("echo"); err != nil {
		t.Fatal(err)
	}
	if got := ss.List(); len(got) != 0 {
		t.Fatalf("List after remove = %v, want empty", got)
	}
	if _, err := ss.Get("echo"); err == nil {
		t.Fatal("expected error after soft delete")
	}
	if _, err := ss.Execute(context.Background(), "echo", nil); err == nil {
		t.Fatal("expected error executing a removed skill")
	}
}

// TestSkillHandlerEndToEnd drives the HTTP layer end-to-end.
func TestSkillHandlerEndToEnd(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	ss := NewSkillStore()
	installEchoSkill(t, ss)
	srv.SetSkillStore(ss)

	// List.
	req := httptest.NewRequest("GET", "/v1/skill-hub", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"name":"echo"`) {
		t.Fatalf("list missing echo skill: %s", w.Body.String())
	}

	// Get.
	req = httptest.NewRequest("GET", "/v1/skill-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}
	var got SkillRecord
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "echo" || !got.Executable {
		t.Fatalf("get returned unexpected record: %+v", got)
	}

	// Execute.
	req = httptest.NewRequest("POST", "/v1/skill-hub/echo/execute",
		strings.NewReader(`{"input":{"text":"hi"}}`))
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("execute: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"text":"hi"`) {
		t.Fatalf("execute response missing echoed input: %s", w.Body.String())
	}

	// Delete.
	req = httptest.NewRequest("DELETE", "/v1/skill-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}

	// Get after delete -> 404.
	req = httptest.NewRequest("GET", "/v1/skill-hub/echo", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404, got %d", w.Code)
	}
}

// TestSkillUnconfigured503 asserts handlers return 503 when the store is not
// wired, so unassembled servers degrade gracefully.
func TestSkillUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetSkillStore
	req := httptest.NewRequest("GET", "/v1/skill-hub", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}