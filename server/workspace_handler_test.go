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

// Behavior tests for the C-7 workspace provider and handlers.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkspaceProviderOpenListDir verifies the adapter wraps workspace.New
// and exposes the read-only file-listing surface.
func TestWorkspaceProviderOpenListDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewWorkspaceProvider()
	info, err := p.Open("main", root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "main" || info.Root == "" {
		t.Fatalf("unexpected info: %+v", info)
	}

	files, err := p.ListDir("main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "hello.txt" {
		t.Fatalf("ListDir = %v, want [hello.txt]", files)
	}

	got, ok := p.Get("main")
	if !ok || got.Name != "main" {
		t.Fatal("Get should return the opened workspace")
	}
	if err := p.Close("main"); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Get("main"); ok {
		t.Fatal("workspace should be gone after Close")
	}
}

// TestWorkspaceOpenEmptyRootRejected verifies the adapter surfaces the
// workspace.New validation error for an empty root dir.
func TestWorkspaceOpenEmptyRootRejected(t *testing.T) {
	p := NewWorkspaceProvider()
	if _, err := p.Open("bad", ""); err == nil {
		t.Fatal("expected error opening a workspace with an empty root")
	}
}

// TestWorkspaceHandlerCRUD 驱动 HTTP 层走 create/list/get/files/delete。
func TestWorkspaceHandlerCRUD(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())

	body := `{"name":"main","root_dir":"` + strings.ReplaceAll(root, `\`, `\\`) + `"}`
	req := httptest.NewRequest("POST", "/v1/workspaces", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Name != "main" {
		t.Fatalf("name = %q, want main", created.Name)
	}

	req = httptest.NewRequest("GET", "/v1/workspaces/main", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/v1/workspaces/main/files", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("files: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a.txt") {
		t.Fatalf("files response missing a.txt: %s", w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/v1/workspaces/main", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", w.Code)
	}
	if _, ok := srv.wsProvider.Get("main"); ok {
		t.Fatal("workspace still present after delete")
	}
}

// TestWorkspaceUnconfigured503 asserts 503 when no provider is wired.
func TestWorkspaceUnconfigured503(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore()) // no SetWorkspaceProvider
	req := httptest.NewRequest("GET", "/v1/workspaces", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
