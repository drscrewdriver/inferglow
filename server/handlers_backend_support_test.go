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

// Behavior tests for the Spec B support endpoints (workspace fs, sandbox,
// jobs, produced-files).

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

// testServerWithWorkspace builds a server whose workspace root points at a
// fresh temp directory.
func testServerWithWorkspace(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	if _, err := srv.wsProvider.Open("main", root); err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	return srv, root
}

func doJSON(t *testing.T, srv *Server, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// TestWorkspaceFSWriteReadTreeDelete exercises the /v1/workspace file endpoints.
func TestWorkspaceFSWriteReadTreeDelete(t *testing.T) {
	srv, root := testServerWithWorkspace(t)

	if w := doJSON(t, srv, "POST", "/v1/workspace/write", `{"path":"src/a.txt","content":"hello"}`); w.Code != http.StatusCreated {
		t.Fatalf("write: want 201, got %d (%s)", w.Code, w.Body.String())
	}
	// file must be on disk
	if _, err := os.Stat(filepath.Join(root, "src", "a.txt")); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	w := doJSON(t, srv, "GET", "/v1/workspace/read?path=src/a.txt", "")
	if w.Code != http.StatusOK {
		t.Fatalf("read: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"content":"hello"`) {
		t.Fatalf("read content missing: %s", w.Body.String())
	}

	// list the src dir via /v1/fs alias
	w = doJSON(t, srv, "GET", "/v1/fs/tree?path=src", "")
	if w.Code != http.StatusOK {
		t.Fatalf("tree: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a.txt") {
		t.Fatalf("tree missing a.txt: %s", w.Body.String())
	}

	// trailing traversal read must be refused
	w = doJSON(t, srv, "GET", "/v1/workspace/read?path="+urlEscape("../../etc"), "")
	if w.Code == http.StatusOK {
		t.Fatalf("traversal read should be refused, got 200")
	}
}

// TestSandboxEndpoints exercises runtimes, presets and rejection recording.
func TestSandboxEndpoints(t *testing.T) {
	srv, _ := testServerWithWorkspace(t)

	w := doJSON(t, srv, "GET", "/v1/sandbox/runtimes", "")
	if w.Code != http.StatusOK {
		t.Fatalf("runtimes: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "trusted_local") || !strings.Contains(w.Body.String(), "docker") {
		t.Fatalf("runtimes missing expected backends: %s", w.Body.String())
	}

	w = doJSON(t, srv, "GET", "/v1/sandbox/presets", "")
	if w.Code != http.StatusOK {
		t.Fatalf("presets: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "workspace_write") {
		t.Fatalf("presets missing default: %s", w.Body.String())
	}

	w = doJSON(t, srv, "POST", "/v1/sandbox/preset", `{"preset":"full_access"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("set preset: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "POST", "/v1/sandbox/rejections", `{"capability":"bash_execute","reason":"boundary"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("record rejection: want 201, got %d (%s)", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, "GET", "/v1/sandbox/rejections", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "bash_execute") {
		t.Fatalf("rejections missing record: %d %s", w.Code, w.Body.String())
	}
}

// TestProducedFiles lists workspace files as produced output.
func TestProducedFiles(t *testing.T) {
	srv, root := testServerWithWorkspace(t)
	if w := doJSON(t, srv, "POST", "/v1/workspace/write", `{"path":"out/report.md","content":"x"}`); w.Code != http.StatusCreated {
		t.Fatalf("write: %d", w.Code)
	}
	// sanity: file must be on disk under root
	if _, err := os.Stat(filepath.Join(root, "out", "report.md")); err != nil {
		t.Fatalf("file missing on disk: %v", err)
	}
	w := doJSON(t, srv, "GET", "/v1/produced-files", "")
	if w.Code != http.StatusOK {
		t.Fatalf("produced-files: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "out/report.md") {
		t.Fatalf("produced-files missing report.md: %s", w.Body.String())
	}
}

// TestJobsList finds a tracked job via the RunManager.
func TestJobsList(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	runID := "run-test"
	srv.runMgr.mu.Lock()
	srv.runMgr.runs[runID] = &RunHandle{ID: runID, Jobs: []*RunJob{}}
	srv.runMgr.mu.Unlock()
	if _, err := srv.runMgr.TrackJob(runID, "bash_execute", "demo job"); err != nil {
		t.Fatalf("track job: %v", err)
	}

	w := doJSON(t, srv, "GET", "/v1/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("jobs: want 200, got %d", w.Code)
	}
	var resp struct {
		Jobs []RunJob `json:"jobs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	found := false
	for _, j := range resp.Jobs {
		if j.Kind == "bash_execute" && j.RunID == runID {
			found = true
		}
	}
	if !found {
		t.Fatalf("tracked job not listed: %s", w.Body.String())
	}
}

func urlEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", "%2F"), "..", "%2E%2E")
}