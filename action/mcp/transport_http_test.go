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

package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testMCPServer is a minimal HTTP MCP server stub for transport
// tests. GET opens a long-lived SSE stream; POST accepts JSON-RPC
// frames. The test driver pushes SSE events via pushSSE (formatted)
// or pushRaw (verbatim) and inspects received POSTs via waitForPost.
type testMCPServer struct {
	*httptest.Server
	sseCh   chan []byte // each msg sent as "data: <msg>\n\n"
	rawCh   chan string // raw SSE bytes sent verbatim
	postCh  chan []byte
	closeCh chan struct{} // closed to terminate SSE handlers promptly
}

func newTestMCPServer(t *testing.T) *testMCPServer {
	t.Helper()
	s := &testMCPServer{
		sseCh:   make(chan []byte, 16),
		rawCh:   make(chan string, 16),
		postCh:  make(chan []byte, 16),
		closeCh: make(chan struct{}),
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *testMCPServer) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		for {
			select {
			case msg := <-s.sseCh:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			case raw := <-s.rawCh:
				fmt.Fprint(w, raw)
				flusher.Flush()
			case <-s.closeCh:
				return
			case <-r.Context().Done():
				return
			}
		}
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.postCh <- body
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// pushSSE enqueues a JSON-RPC message as a single SSE data event.
func (s *testMCPServer) pushSSE(msg []byte) { s.sseCh <- msg }

// pushRaw enqueues verbatim SSE bytes (for multi-line / comment tests).
func (s *testMCPServer) pushRaw(raw string) { s.rawCh <- raw }

// closeSSE terminates all active SSE handlers so server.Close does not
// block waiting for long-lived connections to drain.
func (s *testMCPServer) closeSSE() { close(s.closeCh) }

// waitForPost returns the next POST body received by the server.
func (s *testMCPServer) waitForPost(t *testing.T) []byte {
	t.Helper()
	select {
	case body := <-s.postCh:
		return body
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for POST to test server")
		return nil
	}
}

func TestHTTPTransport_StartHandshake(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	// A successful Start means the SSE stream is open: push a
	// message and verify Recv delivers it.
	msg := []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	server.pushSSE(msg)

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("Recv mismatch:\n want: %s\n got:  %s", msg, got)
	}
}

func TestHTTPTransport_ToolsListRoundTrip(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if err := tr.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify the POST body the server received.
	if got := server.waitForPost(t); !bytes.Equal(got, req) {
		t.Errorf("POST body mismatch:\n want: %s\n got:  %s", req, got)
	}

	// Push the tools/list response via SSE.
	resp := []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`)
	server.pushSSE(resp)

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if !bytes.Equal(got, resp) {
		t.Errorf("Recv mismatch:\n want: %s\n got:  %s", resp, got)
	}
}

func TestHTTPTransport_ToolsCallRoundTrip(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	req := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"msg":"hi"}}}`)
	if err := tr.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := server.waitForPost(t); !bytes.Equal(got, req) {
		t.Errorf("POST body mismatch:\n want: %s\n got:  %s", req, got)
	}

	resp := []byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"hi"}]}}`)
	server.pushSSE(resp)

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if !bytes.Equal(got, resp) {
		t.Errorf("Recv mismatch:\n want: %s\n got:  %s", resp, got)
	}
}

func TestHTTPTransport_StartErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	if err := tr.Start(context.Background()); err == nil {
		t.Fatal("expected error on non-200 SSE status")
	}
}

func TestHTTPTransport_SendErrorStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	err := tr.Send(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err == nil {
		t.Fatal("expected error on 400 POST response")
	}
}

func TestHTTPTransport_ConnectionDrops(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Close the SSE handler from the server side, simulating a
	// connection drop. The read loop should exit and Recv should
	// surface an error.
	server.closeSSE()

	_, err := tr.Recv(ctx)
	if err == nil {
		t.Fatal("expected error after connection drop")
	}
	_ = tr.Stop(context.Background())
}

func TestHTTPTransport_SSEMultiLineData(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	// Multi-line data: the SSE spec concatenates the values of
	// consecutive `data:` lines with "\n".
	server.pushRaw("data: line1\ndata: line2\n\n")

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if want := "line1\nline2"; string(got) != want {
		t.Errorf("multi-line data = %q, want %q", got, want)
	}
}

func TestHTTPTransport_SSECommentAndEventLines(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	// Comment lines (:) and event: lines must be ignored; only the
	// data: field should be delivered.
	server.pushRaw(": heartbeat comment\nevent: message\ndata: {\"ok\":true}\n\n")

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if want := `{"ok":true}`; string(got) != want {
		t.Errorf("data = %q, want %q", got, want)
	}
}

func TestHTTPTransport_SSEMultipleEvents(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	// Two events separated by empty lines produce two Recv calls.
	server.pushRaw("data: {\"id\":1}\n\ndata: {\"id\":2}\n\n")

	got1, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv #1: %v", err)
	}
	if want := `{"id":1}`; string(got1) != want {
		t.Errorf("event 1 = %q, want %q", got1, want)
	}

	got2, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv #2: %v", err)
	}
	if want := `{"id":2}`; string(got2) != want {
		t.Errorf("event 2 = %q, want %q", got2, want)
	}
}

func TestHTTPTransport_SendBeforeStartFails(t *testing.T) {
	tr := &HTTPTransport{baseURL: "http://example.invalid"}
	if err := tr.Send(context.Background(), []byte("x")); err == nil {
		t.Error("Send before Start should fail")
	}
	if _, err := tr.Recv(context.Background()); err == nil {
		t.Error("Recv before Start should fail")
	}
}

func TestHTTPTransport_StartTwiceFails(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx := context.Background()
	if err := tr.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() { _ = tr.Stop(ctx) }()
	if err := tr.Start(ctx); err == nil {
		t.Error("second Start should fail")
	}
}

func TestHTTPTransport_StopWithoutStartNoop(t *testing.T) {
	tr := &HTTPTransport{baseURL: "http://example.invalid"}
	if err := tr.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start should be no-op, got %v", err)
	}
}

func TestHTTPTransport_RecvCancelRespectsContext(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	tr := &HTTPTransport{baseURL: server.URL}
	ctx := context.Background()
	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(ctx) }()

	// Don't push anything — Recv should block until ctx is canceled.
	recvCtx, recvCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer recvCancel()
	if _, err := tr.Recv(recvCtx); err == nil {
		t.Error("Recv with canceled ctx should return error")
	}
}
