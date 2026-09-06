// Copyright 2026 InferGlow Authors

package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ptyFrame is one server→client wire message.
type ptyFrame struct {
	Type string `json:"type"`
	D    string `json:"d,omitempty"`
	Code *int   `json:"code,omitempty"`
}

// dialPty connects to the PTY endpoint with the query-token auth path.
func dialPty(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// collectUntil reads frames until pred matches decoded output or timeout.
func collectUntil(t *testing.T, conn *websocket.Conn, timeout time.Duration, pred func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read frames: %v (got %q)", err, acc.String())
		}
		var f ptyFrame
		if err := json.Unmarshal(raw, &f); err != nil {
			continue
		}
		switch f.Type {
		case "o", "replay":
			if data, derr := base64.StdEncoding.DecodeString(f.D); derr == nil {
				acc.Write(data)
			}
			if pred(acc.String()) {
				return acc.String()
			}
		case "exit":
			t.Fatalf("shell exited before marker (code %v): %q", f.Code, acc.String())
		}
	}
	t.Fatalf("timeout waiting for marker; got %q", acc.String())
	return ""
}

// TestPtyFailClosed — without the -pty switch (or an API key) the endpoint
// must not exist at all, mirroring /v1/exec's fail-closed contract.
func TestPtyFailClosed(t *testing.T) {
	srv := NewServer(DefaultConfig(), newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/pty")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("no -pty ⇒ want 404, got %d", resp.StatusCode)
	}
}

// TestPtySessionPersistAndReplay — the E2E lifecycle: spawn on first
// connect, echo a marker, disconnect, reconnect and find the marker in the
// replayed transcript (persistent shell + bounded ring).
func TestPtySessionPersistAndReplay(t *testing.T) {
	dir := t.TempDir()

	cfg := DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.PTYEnabled = true
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skip("no bash on this host")
		}
		cfg.PTYShell = "bash -i"
	}
	srv := NewServer(cfg, newMockStore())
	srv.SetWorkspaceProvider(NewWorkspaceProvider())
	srv.SeedWorkspaces([]WorkspaceSeed{{Name: "tws", Root: dir}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/v1/pty?token=test-key&workspace=tws"

	// 1) Bad token rejected.
	badURL := strings.Replace(wsURL, "token=test-key", "token=wrong", 1)
	if _, _, err := websocket.DefaultDialer.Dial(badURL, nil); err == nil {
		t.Fatalf("bad token must be rejected")
	}

	// 2) First connect: spawn + echo a marker.
	marker := "pty-marker-e5f2"
	conn := dialPty(t, wsURL)
	conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","text":"echo `+marker+`\r"}`))
	collectUntil(t, conn, 15*time.Second, func(out string) bool {
		return strings.Contains(out, marker)
	})
	// Resize is accepted without breaking the stream.
	conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":100,"rows":30}`))
	time.Sleep(300 * time.Millisecond)

	// 3) Disconnect: the shell must survive.
	conn.Close()

	// 4) Reconnect: the transcript replay contains the marker.
	conn2 := dialPty(t, wsURL)
	collectUntil(t, conn2, 15*time.Second, func(out string) bool {
		return strings.Contains(out, marker)
	})

	// 5) Explicit kill closes the session; the next connect spawns fresh.
	conn2.WriteMessage(websocket.TextMessage, []byte(`{"type":"kill"}`))
	time.Sleep(300 * time.Millisecond)
	conn3 := dialPty(t, wsURL)
	// A fresh cmd.exe echoes its prompt; just prove the session is alive by
	// echoing a new marker.
	marker2 := "pty-marker-2b91"
	conn3.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","text":"echo `+marker2+`\r"}`))
	collectUntil(t, conn3, 15*time.Second, func(out string) bool {
		return strings.Contains(out, marker2)
	})
	srv.ShutdownTerminals()
}
