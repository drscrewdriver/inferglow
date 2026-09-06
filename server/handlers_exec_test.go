// Copyright 2026 InferGlow Authors

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecRouteFailClosed — without APIKey or without ExecEnabled the exec
// endpoint must be unreachable: 404 (route absent) or, when an API key is
// configured, 401 from the auth middleware that runs ahead of the mux. Both
// are fail-closed — the command can never execute.
func TestExecRouteFailClosed(t *testing.T) {
	cases := []struct {
		name     string
		cfg      Config
		wantCode []int
	}{
		{"no-key-no-flag", Config{}, []int{404}},
		{"key-only", Config{APIKey: "k"}, []int{401, 404}},
		{"flag-only", Config{ExecEnabled: true}, []int{404}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewServer(tc.cfg, newMockStore())
			w := doJSON(t, srv, "POST", "/v1/exec", `{"argv":["git","status"]}`)
			ok := false
			for _, want := range tc.wantCode {
				if w.Code == want {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("want %v (fail-closed), got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

func newExecTestServer(t *testing.T) *Server {
	t.Helper()
	srv := NewServer(Config{
		APIKey:       "test-key",
		ExecEnabled:  true,
		UsageDataDir: t.TempDir(),
	}, newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	return srv
}

func doExec(t *testing.T, srv *Server, key, body string) (int, string) {
	t.Helper()
	req := postExecRequest(t, body)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func postExecRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/exec", strings.NewReader(body))
	return req
}

// TestExecGates — auth, allowlist, traversal and a real allowlisted run.
func TestExecGates(t *testing.T) {
	srv := newExecTestServer(t)

	// Missing / wrong key → 401 (middleware).
	if code, _ := doExec(t, srv, "", `{"argv":["git","status"]}`); code != 401 {
		t.Fatalf("no key: want 401, got %d", code)
	}
	if code, _ := doExec(t, srv, "wrong", `{"argv":["git","status"]}`); code != 401 {
		t.Fatalf("wrong key: want 401, got %d", code)
	}

	// Non-allowlisted command → 403.
	if code, body := doExec(t, srv, "test-key", `{"argv":["powershell","-c","echo hi"]}`); code != 403 {
		t.Fatalf("non-allowlisted: want 403, got %d (%s)", code, body)
	}

	// Path-ish argv[0] → 403.
	if code, _ := doExec(t, srv, "test-key", `{"argv":["./git","status"]}`); code != 403 {
		t.Fatalf("path argv[0]: want 403, got %d", code)
	}

	// workdir traversal → 403.
	if code, _ := doExec(t, srv, "test-key", `{"argv":["go","version"],"workdir":"../../"}`); code != 403 {
		t.Fatalf("traversal workdir: want 403, got %d", code)
	}

	// Empty argv → 400.
	if code, _ := doExec(t, srv, "test-key", `{"argv":[]}`); code != 400 {
		t.Fatalf("empty argv: want 400, got %d", code)
	}

	// Allowlisted command really runs (go is in the test toolchain).
	code, body := doExec(t, srv, "test-key", `{"argv":["go","version"]}`)
	if code != 200 {
		t.Fatalf("go version: want 200, got %d (%s)", code, body)
	}
	if !strings.Contains(body, `"exit_code":0`) || !strings.Contains(body, "go version") {
		t.Fatalf("go version output unexpected: %s", body)
	}

	// Audit trail landed on disk.
	auditPath := filepath.Join(srv.cfg.UsageDataDir, "exec-audit.jsonl")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit file missing: %v", err)
	}
	if !strings.Contains(string(data), `"argv":["go","version"]`) {
		t.Fatalf("audit missing allowed run: %s", data)
	}
	if !strings.Contains(string(data), "command not in allowlist") {
		t.Fatalf("audit missing rejected attempt: %s", data)
	}
}
